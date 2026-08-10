package notifier

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotificationQueue_Disabled(t *testing.T) {
	queue := NewNotificationQueue(nil, 10, 100*time.Millisecond)
	if queue.IsEnabled() {
		t.Errorf("expected queue to be disabled when client is nil")
	}

	queue.Start()
	ok := queue.Enqueue("test msg")
	if ok {
		t.Errorf("expected Enqueue to return false when queue is disabled")
	}
	queue.Stop()
}

func TestNotificationQueue_EnqueueAndRateLimit(t *testing.T) {
	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewTelegramClient("BOT_TOKEN", "CHAT_ID")
	client.SetBaseURL(server.URL)

	// Rate limit: 50ms interval between messages
	queue := NewNotificationQueue(client, 10, 50*time.Millisecond)
	queue.Start()

	startTime := time.Now()
	queue.Enqueue("Msg 1")
	queue.Enqueue("Msg 2")
	queue.Enqueue("Msg 3")

	queue.Stop()
	elapsed := time.Since(startTime)

	if count := atomic.LoadInt32(&callCount); count != 3 {
		t.Errorf("expected 3 messages sent, got %d", count)
	}

	if elapsed < 80*time.Millisecond {
		t.Errorf("expected rate limiter to take at least 80ms, took %v", elapsed)
	}
}
