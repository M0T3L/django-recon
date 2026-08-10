package runner_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"django/internal/models"
	"django/internal/runner"
)

func TestRunner_SuccessExecutionAndStreaming(t *testing.T) {
	r := runner.New()
	ctx := context.Background()

	var lines []string
	var mu sync.Mutex

	stdoutHandler := func(line []byte) error {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, string(line))
		return nil
	}

	opts := runner.CommandOptions{
		StdoutHandler: stdoutHandler,
		Timeout:       5 * time.Second,
	}

	res, err := r.Run(ctx, "echo", []string{"hello\nworld"}, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "hello" || lines[1] != "world" {
		t.Errorf("unexpected output lines: %v", lines)
	}
}

func TestRunner_TimeoutCancellation(t *testing.T) {
	r := runner.New()
	ctx := context.Background()

	opts := runner.CommandOptions{
		Timeout: 200 * time.Millisecond,
	}

	res, err := r.Run(ctx, "sleep", []string{"2"}, opts)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	if res.ExitCode != -1 {
		t.Errorf("expected exit code -1 on timeout, got %d", res.ExitCode)
	}
}

func TestRunner_PanicRecovery(t *testing.T) {
	r := runner.New()
	ctx := context.Background()

	panicHandler := func(line []byte) error {
		panic("simulated panic inside handler")
	}

	opts := runner.CommandOptions{
		StdoutHandler: panicHandler,
	}

	res, err := r.Run(ctx, "echo", []string{"trigger panic"}, opts)
	if err == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}

	if !strings.Contains(err.Error(), "handler panicked") && !strings.Contains(err.Error(), "recovered from panic") {
		t.Errorf("expected error message to contain panic error, got: %v", err)
	}

	if res.ExitCode != -1 {
		t.Errorf("expected exit code -1 after panic recovery, got %d", res.ExitCode)
	}
}

func TestGenericJSONParser_WithSubdomainModel(t *testing.T) {
	parser := runner.NewJSONParser[models.Subdomain]()

	jsonLine := []byte(`{"id": 1, "target_id": 10, "host": "api.example.com", "ip": "192.168.1.1", "status_code": 200}`)

	subdomain, err := parser.Parse(jsonLine)
	if err != nil {
		t.Fatalf("failed to parse JSON line: %v", err)
	}

	if subdomain.Host != "api.example.com" {
		t.Errorf("expected host 'api.example.com', got '%s'", subdomain.Host)
	}
	if subdomain.StatusCode != 200 {
		t.Errorf("expected status code 200, got %d", subdomain.StatusCode)
	}
}

type RawSubdomainInput struct {
	Domain string `json:"domain"`
	IPAddr string `json:"ip_address"`
}

type SubdomainMapper struct {
	TargetID uint
}

func (m *SubdomainMapper) Map(in RawSubdomainInput) (models.Subdomain, error) {
	return models.Subdomain{
		TargetID: m.TargetID,
		Host:     in.Domain,
		IP:       in.IPAddr,
	}, nil
}

func TestBuildMappedLineHandler(t *testing.T) {
	parser := runner.NewJSONParser[RawSubdomainInput]()
	mapper := &SubdomainMapper{TargetID: 42}

	var parsedModels []models.Subdomain
	handler := runner.BuildMappedLineHandler(parser, mapper, func(item models.Subdomain) error {
		parsedModels = append(parsedModels, item)
		return nil
	})

	line := []byte(`{"domain": "sub.example.com", "ip_address": "10.0.0.1"}`)
	err := handler(line)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	if len(parsedModels) != 1 {
		t.Fatalf("expected 1 parsed model, got %d", len(parsedModels))
	}

	if parsedModels[0].TargetID != 42 || parsedModels[0].Host != "sub.example.com" {
		t.Errorf("unexpected mapped struct: %+v", parsedModels[0])
	}
}
