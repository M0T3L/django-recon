package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelegramClient_DisabledFallback(t *testing.T) {
	client := NewTelegramClient("", "")
	if client.IsEnabled() {
		t.Errorf("expected client to be disabled when token/chat_id are empty")
	}

	err := client.SendMessage(context.Background(), "test message")
	if err != nil {
		t.Errorf("expected nil error on disabled client fallback, got: %v", err)
	}
}

func TestTelegramClient_SendMessageSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		if r.URL.Path != "/bot12345:TEST_TOKEN/sendMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var payload TelegramSendMessagePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if payload.ChatID != "987654" {
			t.Errorf("expected chat_id 987654, got %s", payload.ChatID)
		}
		if payload.Text != "Test Message" {
			t.Errorf("expected text 'Test Message', got %s", payload.Text)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TelegramResponse{Ok: true})
	}))
	defer server.Close()

	client := NewTelegramClient("12345:TEST_TOKEN", "987654")
	client.SetBaseURL(server.URL)

	if !client.IsEnabled() {
		t.Errorf("expected client to be enabled")
	}

	err := client.SendMessage(context.Background(), "Test Message")
	if err != nil {
		t.Fatalf("expected successful send, got error: %v", err)
	}
}

func TestTelegramClient_SendMessageError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(TelegramResponse{
			Ok:          false,
			Description: "Bad Request: can't parse entities",
		})
	}))
	defer server.Close()

	client := NewTelegramClient("12345:TEST_TOKEN", "987654")
	client.SetBaseURL(server.URL)

	err := client.SendMessage(context.Background(), "Invalid [Markdown")
	if err == nil {
		t.Fatalf("expected error from API, got nil")
	}
}
