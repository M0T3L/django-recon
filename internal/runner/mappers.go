package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"django/internal/models"
)

// --- Subfinder Structs ---
type SubfinderOutput struct {
	Host   string `json:"host"`
	IP     string `json:"ip"`
	Source string `json:"source"`
}

// ParseSubfinderLine parses subfinder JSON line into host domain string.
func ParseSubfinderLine(line []byte) (host string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("subfinder parser panic recovered: %v", r)
		}
	}()

	var out SubfinderOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Host), nil
}

// --- DNSX Structs ---
type DNSXOutput struct {
	Host  string   `json:"host"`
	A     []string `json:"a"`
	AAAA  []string `json:"aaaa"`
	CNAME []string `json:"cname"`
}

// ParseDNSXLine parses dnsx JSON line into Subdomain model.
func ParseDNSXLine(line []byte, targetID uint) (sub models.Subdomain, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("dnsx parser panic recovered: %v", r)
		}
	}()

	var out DNSXOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return sub, err
	}

	ipStr := ""
	if len(out.A) > 0 {
		ipStr = out.A[0]
	} else if len(out.AAAA) > 0 {
		ipStr = out.AAAA[0]
	}

	return models.Subdomain{
		TargetID: targetID,
		Host:     strings.TrimSpace(out.Host),
		IP:       ipStr,
	}, nil
}

// --- Naabu Structs ---
type NaabuOutput struct {
	Host string `json:"host"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ParseNaabuLine parses naabu JSON line into Subdomain model with host:port.
func ParseNaabuLine(line []byte, targetID uint) (sub models.Subdomain, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("naabu parser panic recovered: %v", r)
		}
	}()

	var out NaabuOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return sub, err
	}

	host := strings.TrimSpace(out.Host)
	if out.Port > 0 && out.Port != 80 && out.Port != 443 {
		host = fmt.Sprintf("%s:%d", host, out.Port)
	}

	return models.Subdomain{
		TargetID: targetID,
		Host:     host,
		IP:       out.IP,
	}, nil
}

// --- HTTPX Structs ---
type HTTPXOutput struct {
	Input         string   `json:"input"`
	URL           string   `json:"url"`
	Host          string   `json:"host"`
	IP            string   `json:"ip"`
	Title         string   `json:"title"`
	StatusCode    int      `json:"status_code"`
	ContentLength int64    `json:"content_length"`
	Tech          []string `json:"tech"`
	Technologies  []string `json:"technologies"`
	Screenshot    string   `json:"screenshot_path"`
	StoredResp    string   `json:"stored_response_path"`
}

// ParseHTTPXLine parses httpx JSON line into Subdomain model.
func ParseHTTPXLine(line []byte, targetID uint) (sub models.Subdomain, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("httpx parser panic recovered: %v", r)
		}
	}()

	var out HTTPXOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return sub, err
	}

	host := strings.TrimSpace(out.Host)
	if host == "" {
		host = strings.TrimSpace(out.Input)
	}

	techs := out.Tech
	if len(techs) == 0 {
		techs = out.Technologies
	}

	screenshotPath := strings.TrimSpace(out.Screenshot)
	if screenshotPath != "" {
		if info, statErr := os.Stat(screenshotPath); statErr != nil || info.Size() == 0 {
			screenshotPath = ""
		}
	}

	return models.Subdomain{
		TargetID:       targetID,
		Host:           host,
		IP:             out.IP,
		StatusCode:     out.StatusCode,
		Title:          out.Title,
		Technologies:   models.StringArray(techs),
		ContentLength:  out.ContentLength,
		ScreenshotPath: screenshotPath,
	}, nil
}

// --- Nuclei Structs ---
type NucleiInfo struct {
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type NucleiOutput struct {
	TemplateID string     `json:"template-id"`
	Info       NucleiInfo `json:"info"`
	MatchedAt  string     `json:"matched-at"`
	Type       string     `json:"type"`
	IP         string     `json:"ip"`
	Extracts   []string   `json:"extracted-results"`
}

// ParseNucleiLine parses nuclei JSON/JSONL line into Finding model.
func ParseNucleiLine(line []byte, targetID uint) (finding models.Finding, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("nuclei parser panic recovered: %v", r)
		}
	}()

	var out NucleiOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return finding, err
	}

	severity := strings.ToLower(out.Info.Severity)
	if severity == "" {
		severity = "info"
	}

	title := out.Info.Name
	if title == "" {
		title = out.TemplateID
	}

	rawOutput := fmt.Sprintf("Matched at: %s | Template: %s", out.MatchedAt, out.TemplateID)

	return models.Finding{
		TargetID:    targetID,
		ToolName:    "nuclei",
		Severity:    severity,
		Title:       title,
		Description: out.Info.Description,
		RawOutput:   rawOutput,
	}, nil
}

// --- Katana Structs ---
type KatanaRequest struct {
	Endpoint string `json:"endpoint"`
	URL      string `json:"url"`
}

type KatanaOutput struct {
	Timestamp string        `json:"timestamp"`
	Request   KatanaRequest `json:"request"`
}

// ParseKatanaLine parses katana crawler JSON line into Finding model or target URL string.
func ParseKatanaLine(line []byte, targetID uint) (finding models.Finding, targetURL string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("katana parser panic recovered: %v", r)
		}
	}()

	var out KatanaOutput
	if err := json.Unmarshal(line, &out); err != nil {
		return finding, "", err
	}

	endpoint := out.Request.Endpoint
	if endpoint == "" {
		endpoint = out.Request.URL
	}
	endpoint = strings.TrimSpace(endpoint)

	if endpoint == "" {
		return finding, "", nil
	}

	finding = models.Finding{
		TargetID:    targetID,
		ToolName:    "katana",
		Severity:    "info",
		Title:       "Crawled Endpoint Discovered",
		Description: fmt.Sprintf("Discovered endpoint: %s", endpoint),
		RawOutput:   endpoint,
	}

	return finding, endpoint, nil
}
