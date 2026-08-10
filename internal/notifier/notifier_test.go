package notifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"django/internal/models"
)

func TestNotifier_DisabledFallback(t *testing.T) {
	n := NewNotifier("", "")
	if n.IsEnabled() {
		t.Errorf("expected notifier to be disabled")
	}

	// None of these should panic or error out
	n.Start()
	n.NotifyJobStarted(1, "example.com", "pipeline_1")
	n.NotifyJobCompleted(1, "example.com", 10*time.Second, 5, 1, 0, 0)
	n.NotifyJobFailed(1, "example.com", 2*time.Second, "some error")
	n.NotifyFinding("example.com", models.Finding{Severity: "critical", Title: "RCE"})
	n.Stop()
}

func TestNotifier_EventsFormatting(t *testing.T) {
	var mu sync.Mutex
	var receivedMessages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload TelegramSendMessagePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			mu.Lock()
			receivedMessages = append(receivedMessages, payload.Text)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewTelegramClient("BOT_TOKEN", "CHAT_ID")
	client.SetBaseURL(server.URL)

	n := &Notifier{
		queue: NewNotificationQueue(client, 10, 10*time.Millisecond),
	}
	n.Start()

	// 1. Job Started
	n.NotifyJobStarted(42, "test.com", "pipeline_2")

	// 2. Finding - info (should NOT generate notification)
	n.NotifyFinding("test.com", models.Finding{
		Severity: "info",
		Title:    "Info Finding",
	})

	// 3. Finding - critical (SHOULD generate notification)
	n.NotifyFinding("sub.test.com", models.Finding{
		ToolName:    "nuclei",
		Severity:    "critical",
		Title:       "SQL Injection",
		Description: "Vulnerable endpoint /api/v1/user",
	})

	// 4. Job Completed
	n.NotifyJobCompleted(42, "test.com", 45*time.Second, 12, 2, 3, 1)

	n.Stop()

	mu.Lock()
	msgs := receivedMessages
	mu.Unlock()

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages sent to Telegram, got %d", len(msgs))
	}

	if !containsSubstring(msgs[0], "Scan Started") || !containsSubstring(msgs[0], "test\\.com") {
		t.Errorf("unexpected JobStarted msg: %s", msgs[0])
	}

	if !containsSubstring(msgs[1], "CRITICAL FINDING DISCOVERED") || !containsSubstring(msgs[1], "SQL Injection") {
		t.Errorf("unexpected Finding msg: %s", msgs[1])
	}

	if !containsSubstring(msgs[2], "Scan Completed") || !containsSubstring(msgs[2], "12") {
		t.Errorf("unexpected JobCompleted msg: %s", msgs[2])
	}
}

func containsSubstring(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || len(str) > 0 && (func() bool {
		for i := 0; i <= len(str)-len(substr); i++ {
			if str[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})())
}
