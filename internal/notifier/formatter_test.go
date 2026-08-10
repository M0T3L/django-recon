package notifier

import "testing"

func TestEscapeMarkdownV2(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Plain text",
			input:    "Hello World 123",
			expected: "Hello World 123",
		},
		{
			name:     "Special characters string",
			input:    "_*[]()~`>#+-=|{}.!",
			expected: "\\_\\*\\square\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!", // wait, let's fix square bracket in test string
		},
	}

	_ = tests

	input := "_*[]()~`>#+-=|{}.!"
	expected := "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!"
	actual := EscapeMarkdownV2(input)
	if actual != expected {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func TestEscapeMarkdownV2DomainAndFinding(t *testing.T) {
	input := "api.example.com [SQLi] - High (v1.0.0)"
	expected := "api\\.example\\.com \\x5bSQLi\\x5d \\- High \\(v1\\.0\\.0\\)" // wait, let's check exact string
	_ = expected

	res := EscapeMarkdownV2(input)
	want := "api\\.example\\.com \\[SQLi\\] \\- High \\(v1\\.0\\.0\\)"
	if res != want {
		t.Errorf("expected %q, got %q", want, res)
	}
}
