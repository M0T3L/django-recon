package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"django/internal/config"
	"django/internal/logger"
	"django/internal/models"
	"django/internal/notifier"
	"django/internal/runner"
)

// StepInputSource defines how the output of a previous step is passed to the next step.
type StepInputSource string

const (
	InputSourceNone  StepInputSource = "none"
	InputSourceArgs  StepInputSource = "args"
	InputSourceStdin StepInputSource = "stdin"
)

// Step represents an individual command execution stage within a pipeline.
type Step struct {
	Name          string
	Command       string
	Args          []string
	InputFrom     string
	ParserName    string
	Timeout       time.Duration
	PassOutputAs  StepInputSource
	StdoutHandler runner.LineHandler
	StderrHandler runner.LineHandler
}

// Pipeline represents a named sequence of steps executed in order.
type Pipeline struct {
	Name  string
	Steps []Step
}

// Registry holds registered strategy pipelines.
type Registry struct {
	mu        sync.RWMutex
	pipelines map[string]*Pipeline
}

// NewRegistry returns an initialized Pipeline Registry pre-configured with defaults or loaded from tools.yaml.
func NewRegistry() *Registry {
	r := &Registry{
		pipelines: make(map[string]*Pipeline),
	}
	_ = r.LoadFromToolsConfig("configs/tools.yaml")
	if len(r.pipelines) == 0 {
		r.registerDefaults()
	}
	return r
}

// LoadFromToolsConfig initializes pipelines dynamically from a loaded tools.yaml file.
func (r *Registry) LoadFromToolsConfig(path string) error {
	cfg, err := config.LoadToolsConfig(path)
	if err != nil {
		logger.Error("Pipeline", "Failed to load tools configuration file", logger.Err(err))
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for name, pCfg := range cfg.Pipelines {
		p := &Pipeline{
			Name:  pCfg.Name,
			Steps: make([]Step, 0, len(pCfg.Steps)),
		}

		for _, sCfg := range pCfg.Steps {
			if sCfg.Tool == "" {
				continue // skip internal modules without CLI binary execution
			}

			passInput := InputSourceNone
			if sCfg.InputFrom != "" {
				passInput = InputSourceStdin
			}

			stepTimeout := 10 * time.Minute
			if sCfg.Timeout != "" {
				if parsedTimeout, parseErr := time.ParseDuration(sCfg.Timeout); parseErr == nil && parsedTimeout > 0 {
					stepTimeout = parsedTimeout
				}
			}

			p.Steps = append(p.Steps, Step{
				Name:         sCfg.Name,
				Command:      sCfg.Tool,
				Args:         sCfg.Args,
				InputFrom:    sCfg.InputFrom,
				ParserName:   sCfg.Parser,
				Timeout:      stepTimeout,
				PassOutputAs: passInput,
			})
		}

		r.pipelines[name] = p
	}

	logger.Info("Pipeline", fmt.Sprintf("Successfully loaded %d pipeline strategies from %s", len(r.pipelines), path))
	return nil
}

// Register adds or updates a pipeline strategy in the registry.
func (r *Registry) Register(name string, p *Pipeline) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pipelines[name] = p
}

// Get retrieves a pipeline strategy by name.
func (r *Registry) Get(name string) (*Pipeline, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.pipelines[name]
	return p, ok
}

// registerDefaults sets up fallback pipeline strategies if YAML is missing.
func (r *Registry) registerDefaults() {
	r.Register("pipeline_1", &Pipeline{
		Name: "Standard Fast Pipeline",
		Steps: []Step{
			{
				Name:         "Subdomain Enumeration",
				Command:      "echo",
				Args:         []string{"[pipeline_1] Enumerating subdomains for {target}"},
				Timeout:      30 * time.Second,
				PassOutputAs: InputSourceNone,
			},
			{
				Name:         "Port & Service Scan",
				Command:      "echo",
				Args:         []string{"[pipeline_1] Scanning discovered targets for {target}"},
				Timeout:      60 * time.Second,
				PassOutputAs: InputSourceStdin,
			},
		},
	})

	r.Register("pipeline_2", &Pipeline{
		Name: "Deep Anomaly Pipeline",
		Steps: []Step{
			{
				Name:         "Deep Subdomain Recon",
				Command:      "echo",
				Args:         []string{"[pipeline_2] Deep recon on {target}"},
				Timeout:      60 * time.Second,
				PassOutputAs: InputSourceNone,
			},
			{
				Name:         "HTTP Endpoint Crawl",
				Command:      "echo",
				Args:         []string{"[pipeline_2] Crawling endpoints for {target}"},
				Timeout:      120 * time.Second,
				PassOutputAs: InputSourceStdin,
			},
		},
	})
}

// ScanDelta tracks newly discovered subdomains and findings during a single scan job.
type ScanDelta struct {
	NewSubdomains int
	NewFindings   int
}

// ExecuteContext runs the pipeline with target domain, target ID, and database persistence.
func (p *Pipeline) ExecuteContext(ctx context.Context, target string, targetID uint, db *gorm.DB, stepCallback func(stepName string, stepIndex int, totalSteps int)) error {
	_, err := p.ExecuteContextWithNotifier(ctx, target, targetID, db, nil, stepCallback)
	return err
}

// ExecuteContextWithNotifier runs the pipeline with target domain, target ID, DB persistence, and optional event notifier.
func (p *Pipeline) ExecuteContextWithNotifier(ctx context.Context, target string, targetID uint, db *gorm.DB, notif *notifier.Notifier, stepCallback func(stepName string, stepIndex int, totalSteps int)) (ScanDelta, error) {
	cmdRunner := runner.New()
	stepOutputs := make(map[string][]string)
	var delta ScanDelta
	var deltaMu sync.Mutex

	totalSteps := len(p.Steps)

	for idx, step := range p.Steps {
		select {
		case <-ctx.Done():
			logger.Warn("Pipeline", "Pipeline execution cancelled by context", logger.Target(target))
			return delta, fmt.Errorf("pipeline execution cancelled: %w", ctx.Err())
		default:
		}

		if stepCallback != nil {
			stepCallback(step.Name, idx+1, totalSteps)
		}

		// Substitute {{target}} parameter in arguments
		args := make([]string, len(step.Args))
		for i, arg := range step.Args {
			replaced := strings.ReplaceAll(arg, "{{target}}", target)
			replaced = strings.ReplaceAll(replaced, "{target}", target)
			args[i] = replaced
		}

		var currentLines []string
		seenTargets := make(map[string]bool)
		var mu sync.Mutex

		opts := runner.CommandOptions{
			Timeout:       step.Timeout,
			StderrHandler: step.StderrHandler,
		}

		// If this step takes input from a previous step, feed host/IP targets via Stdin pipe or Args
		if step.InputFrom != "" {
			prevLines := stepOutputs[step.InputFrom]
			// Fallback: If passive enumeration step yielded zero subdomains, pass target itself
			if len(prevLines) == 0 {
				prevLines = []string{target}
			}
			if step.PassOutputAs == InputSourceArgs {
				args = append(args, prevLines...)
			} else {
				joinedInput := strings.Join(prevLines, "\n") + "\n"
				opts.Stdin = bytes.NewReader([]byte(joinedInput))
			}
		}

		// Build stdout line handler to parse DB records and extract targets for subsequent steps
		opts.StdoutHandler = func(line []byte) error {
			extracted := processParserLine(db, step.ParserName, targetID, target, notif, &delta, &deltaMu, line)
			extracted = strings.TrimSpace(extracted)

			if extracted != "" {
				mu.Lock()
				if !seenTargets[extracted] && len(currentLines) < MaxStepOutputLines {
					seenTargets[extracted] = true
					currentLines = append(currentLines, extracted)
				}
				mu.Unlock()
			}

			if step.StdoutHandler != nil {
				return step.StdoutHandler(line)
			}
			return nil
		}

		logger.Info("Pipeline", fmt.Sprintf("Executing step '%s' [%s]", step.Name, step.Command), logger.Target(target), logger.Tool(step.Command))
		res, err := cmdRunner.Run(ctx, step.Command, args, opts)
		if err != nil {
			logger.Warn("Pipeline", fmt.Sprintf("Step '%s' finished with notice (exit code %d)", step.Name, res.ExitCode), logger.Tool(step.Command), logger.Err(err))
		} else {
			logger.Info("Pipeline", fmt.Sprintf("Step '%s' completed successfully in %v (%d targets collected)", step.Name, res.Duration, len(currentLines)), logger.Tool(step.Command))
		}

		stepOutputs[step.Name] = currentLines
	}

	return delta, nil
}

// MaxStepOutputLines limits the in-memory line buffer passed between pipeline steps to prevent OOM.
const MaxStepOutputLines = 50000

// Execute runs all steps sequentially for target domain without explicit DB targetID.
func (p *Pipeline) Execute(ctx context.Context, target string, stepCallback func(stepName string, stepIndex int, totalSteps int)) error {
	_, err := p.ExecuteContextWithNotifier(ctx, target, 0, nil, nil, stepCallback)
	return err
}

// processParserLine parses JSON lines, updates DB models, triggers notifications, and returns extracted target string for next pipeline steps.
func processParserLine(db *gorm.DB, parserName string, targetID uint, target string, notif *notifier.Notifier, delta *ScanDelta, deltaMu *sync.Mutex, line []byte) string {
	defer func() {
		if rec := recover(); rec != nil {
			logger.Error("Parser", fmt.Sprintf("Recovered from panic during parser execution (%s)", parserName), logger.Raw(string(line)))
		}
	}()

	trimmedLine := strings.TrimSpace(string(line))
	if trimmedLine == "" {
		return ""
	}

	switch parserName {
	case "subfinder_parser":
		host, err := runner.ParseSubfinderLine(line)
		if err == nil && host != "" {
			if db != nil && targetID > 0 {
				subdomain := models.Subdomain{TargetID: targetID, Host: host}
				res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&subdomain)
				if res.Error == nil && res.RowsAffected > 0 && delta != nil && deltaMu != nil {
					deltaMu.Lock()
					delta.NewSubdomains++
					deltaMu.Unlock()
				}
			}
			return host
		}
		return trimmedLine

	case "dnsx_parser":
		subdomain, err := runner.ParseDNSXLine(line, targetID)
		if err == nil && subdomain.Host != "" {
			if db != nil && targetID > 0 {
				if dbErr := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "target_id"}, {Name: "host"}},
					DoUpdates: clause.AssignmentColumns([]string{"ip", "updated_at"}),
				}).Create(&subdomain).Error; dbErr != nil {
					logger.Error("DB", "Failed to update subdomain from dnsx", logger.Err(dbErr), logger.Target(subdomain.Host))
				}
			}
			return subdomain.Host
		}
		return trimmedLine

	case "naabu_parser":
		subdomain, err := runner.ParseNaabuLine(line, targetID)
		if err == nil && subdomain.Host != "" {
			if db != nil && targetID > 0 {
				if dbErr := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "target_id"}, {Name: "host"}},
					DoUpdates: clause.AssignmentColumns([]string{"ip", "updated_at"}),
				}).Create(&subdomain).Error; dbErr != nil {
					logger.Error("DB", "Failed to update subdomain from naabu", logger.Err(dbErr), logger.Target(subdomain.Host))
				}
			}
			return subdomain.Host
		}
		return trimmedLine

	case "httpx_parser", "httpx_screenshot_parser":
		subdomain, err := runner.ParseHTTPXLine(line, targetID)
		if err == nil && subdomain.Host != "" {
			if db != nil && targetID > 0 {
				updateCols := []string{"status_code", "title", "technologies", "content_length", "updated_at"}
				if subdomain.ScreenshotPath != "" {
					updateCols = append(updateCols, "screenshot_path")
				}
				if dbErr := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "target_id"}, {Name: "host"}},
					DoUpdates: clause.AssignmentColumns(updateCols),
				}).Create(&subdomain).Error; dbErr != nil {
					logger.Error("DB", "Failed to update subdomain from httpx", logger.Err(dbErr), logger.Target(subdomain.Host))
				}
			}
			return subdomain.Host
		}
		return trimmedLine

	case "katana_parser":
		_, targetURL, err := runner.ParseKatanaLine(line, targetID)
		if err == nil && targetURL != "" {
			if db != nil && targetID > 0 {
				subdomain := models.Subdomain{TargetID: targetID, Host: targetURL}
				res := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&subdomain)
				if res.Error == nil && res.RowsAffected > 0 && delta != nil && deltaMu != nil {
					deltaMu.Lock()
					delta.NewSubdomains++
					deltaMu.Unlock()
				}
			}
			return targetURL
		}
		return trimmedLine

	case "nuclei_parser":
		finding, err := runner.ParseNucleiLine(line, targetID)
		if err == nil && finding.Title != "" {
			isNewFinding := false
			if db != nil && targetID > 0 {
				finding.ComputeFingerprint()
				res := db.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "target_id"}, {Name: "fingerprint"}},
					DoNothing: true,
				}).Create(&finding)
				if dbErr := res.Error; dbErr != nil {
					logger.Error("DB", "Failed to insert finding from nuclei", logger.Err(dbErr))
				} else if res.RowsAffected > 0 {
					isNewFinding = true
					if delta != nil && deltaMu != nil {
						deltaMu.Lock()
						delta.NewFindings++
						deltaMu.Unlock()
					}
				}
			} else {
				isNewFinding = true
			}
			if isNewFinding && notif != nil {
				notif.NotifyFinding(target, finding)
			}
			return finding.Title
		}
		return ""
	}

	return trimmedLine
}
