package config_test

import (
	"path/filepath"
	"testing"

	"django/internal/config"
)

func TestLoadToolsConfig(t *testing.T) {
	// Test loading actual tools.yaml file
	path := filepath.Join("..", "..", "configs", "tools.yaml")
	cfg, err := config.LoadToolsConfig(path)
	if err != nil {
		t.Fatalf("failed to load tools config: %v", err)
	}

	p1, exists := cfg.Pipelines["pipeline_1"]
	if !exists {
		t.Fatalf("expected pipeline_1 in config")
	}

	if len(p1.Steps) == 0 {
		t.Fatalf("pipeline_1 has no steps")
	}

	if p1.Steps[0].Name != "subdomain_enum" || p1.Steps[0].Tool != "subfinder" {
		t.Errorf("unexpected first step config: %+v", p1.Steps[0])
	}
}
