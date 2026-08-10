package pipeline_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"django/internal/models"
	"django/internal/pipeline"
)

func TestPipeline_ExecutionAndOutputPassing(t *testing.T) {
	reg := pipeline.NewRegistry()

	var step1Output []string
	var step2Output []string

	p := &pipeline.Pipeline{
		Name: "Test Custom Pipeline",
		Steps: []pipeline.Step{
			{
				Name:         "Step 1 - Echo Domain",
				Command:      "echo",
				Args:         []string{"domain:{target}"},
				Timeout:      5 * time.Second,
				PassOutputAs: pipeline.InputSourceNone,
				StdoutHandler: func(line []byte) error {
					step1Output = append(step1Output, string(line))
					return nil
				},
			},
			{
				Name:         "Step 2 - Pass Prev Output",
				Command:      "echo",
				Args:         []string{"processed:"},
				InputFrom:    "Step 1 - Echo Domain",
				Timeout:      5 * time.Second,
				PassOutputAs: pipeline.InputSourceArgs,
				StdoutHandler: func(line []byte) error {
					step2Output = append(step2Output, string(line))
					return nil
				},
			},
		},
	}

	reg.Register("custom", p)

	ctx := context.Background()
	err := p.Execute(ctx, "example.com", nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(step1Output) != 1 || step1Output[0] != "domain:example.com" {
		t.Errorf("unexpected step 1 output: %v", step1Output)
	}

	if len(step2Output) != 1 || !strings.Contains(step2Output[0], "domain:example.com") {
		t.Errorf("unexpected step 2 output: %v", step2Output)
	}
}

func TestPipeline_RegistryDefaults(t *testing.T) {
	reg := pipeline.NewRegistry()

	p1, ok1 := reg.Get("pipeline_1")
	if !ok1 || p1 == nil || len(p1.Steps) == 0 {
		t.Errorf("pipeline_1 not properly registered")
	}

	p2, ok2 := reg.Get("pipeline_2")
	if !ok2 || p2 == nil || len(p2.Steps) == 0 {
		t.Errorf("pipeline_2 not properly registered")
	}
}

func TestFinding_FingerprintDeduplication(t *testing.T) {
	f1 := models.Finding{
		TargetID:  1,
		ToolName:  "nuclei",
		Title:     "SQL Injection",
		RawOutput: "Matched at: http://example.com/api?id=1",
	}
	f2 := models.Finding{
		TargetID:  1,
		ToolName:  "nuclei",
		Title:     "SQL Injection",
		RawOutput: "Matched at: http://example.com/api?id=1",
	}
	f3 := models.Finding{
		TargetID:  1,
		ToolName:  "nuclei",
		Title:     "XSS",
		RawOutput: "Matched at: http://example.com/api?q=<script>",
	}

	fp1 := f1.ComputeFingerprint()
	fp2 := f2.ComputeFingerprint()
	fp3 := f3.ComputeFingerprint()

	if fp1 == "" {
		t.Errorf("expected non-empty fingerprint")
	}
	if fp1 != fp2 {
		t.Errorf("identical findings should produce identical fingerprints: %s vs %s", fp1, fp2)
	}
	if fp1 == fp3 {
		t.Errorf("different findings should produce different fingerprints: %s vs %s", fp1, fp3)
	}
}
