package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StepConfig defines a single tool execution step in a pipeline.
type StepConfig struct {
	Name           string   `yaml:"name"`
	Tool           string   `yaml:"tool,omitempty"`
	Args           []string `yaml:"args,omitempty"`
	InputFrom      string   `yaml:"input_from,omitempty"`
	OutputType     string   `yaml:"output_type,omitempty"`
	Parser         string   `yaml:"parser,omitempty"`
	InternalModule string   `yaml:"internal_module,omitempty"`
	Timeout        string   `yaml:"timeout,omitempty"`
}

// PipelineConfig defines a pipeline strategy with steps.
type PipelineConfig struct {
	Name  string       `yaml:"name"`
	Steps []StepConfig `yaml:"steps"`
}

// ToolsConfig maps pipeline names to their respective PipelineConfig.
type ToolsConfig struct {
	Pipelines map[string]PipelineConfig `yaml:"pipelines"`
}

// LoadToolsConfig reads and parses configs/tools.yaml.
func LoadToolsConfig(path string) (*ToolsConfig, error) {
	if path == "" {
		path = "configs/tools.yaml"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools config file at %s: %w", path, err)
	}

	var cfg ToolsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse tools yaml configuration: %w", err)
	}

	return &cfg, nil
}
