package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"django/internal/runner"
)

func TestParseSubfinderLine(t *testing.T) {
	line := []byte(`{"host":"sub.example.com","ip":"1.1.1.1","source":"virustotal"}`)
	host, err := runner.ParseSubfinderLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "sub.example.com" {
		t.Errorf("expected sub.example.com, got %s", host)
	}
}

func TestParseHTTPXLine_WithScreenshot(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "app.png")
	if err := os.WriteFile(tmpFile, []byte("fake png content"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	line := []byte(`{"host":"app.example.com","ip":"10.0.0.1","title":"Dashboard","status_code":200,"content_length":1024,"technologies":["React","Nginx"],"screenshot_path":"` + tmpFile + `"}`)
	sub, err := runner.ParseHTTPXLine(line, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub.TargetID != 5 || sub.Host != "app.example.com" || sub.StatusCode != 200 {
		t.Errorf("unexpected subdomain model: %+v", sub)
	}
	if sub.ScreenshotPath != tmpFile {
		t.Errorf("expected screenshot path, got %s", sub.ScreenshotPath)
	}
}

func TestParseHTTPXLine_TechField(t *testing.T) {
	line := []byte(`{"host":"sms.example.com","ip":"8.8.8.8","title":"SMS Portal","status_code":200,"tech":["IIS:10.0","Microsoft ASP.NET","Windows Server"]}`)
	sub, err := runner.ParseHTTPXLine(line, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sub.Technologies) != 3 || sub.Technologies[0] != "IIS:10.0" {
		t.Errorf("expected technologies parsed from tech field, got: %v", sub.Technologies)
	}
}

func TestParseNucleiLine(t *testing.T) {
	line := []byte(`{"template-id":"cve-2023-1234","info":{"name":"Critical RCE","severity":"high","description":"Remote Code Execution"},"matched-at":"https://app.example.com"}`)
	finding, err := runner.ParseNucleiLine(line, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if finding.TargetID != 10 || finding.Severity != "high" || finding.Title != "Critical RCE" {
		t.Errorf("unexpected finding model: %+v", finding)
	}
}

func TestParsers_PanicRecovery(t *testing.T) {
	// Invalid JSON should return error gracefully without panicking
	_, err := runner.ParseHTTPXLine([]byte(`{invalid json`), 1)
	if err == nil {
		t.Errorf("expected error for malformed json")
	}
}
