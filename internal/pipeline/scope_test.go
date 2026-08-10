package pipeline_test

import (
	"testing"

	"django/internal/pipeline"
)

func TestScopeEngine(t *testing.T) {
	scope := pipeline.NewScopeEngine("example.com")

	tests := []struct {
		input    string
		expected bool
	}{
		{"example.com", true},
		{"sub.example.com", true},
		{"api.v2.sub.example.com", true},
		{"https://sub.example.com/api/v1/users", true},
		{"sub.example.com:8443", true},
		{"evil.com", false},
		{"example.com.attacker.com", false},
		{"https://sub.example.com/logout", false}, // Destructive token filter
		{"https://sub.example.com/user/delete", false},
	}

	for _, tt := range tests {
		result := scope.IsInScope(tt.input)
		if result != tt.expected {
			t.Errorf("IsInScope(%q) = %v; expected %v", tt.input, result, tt.expected)
		}
	}
}
