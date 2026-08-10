package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TelegramClient handles sending notification messages via Telegram Bot API.
type TelegramClient struct {
	botToken   string
	chatID     string
	baseURL    string
	httpClient *http.Client
	enabled    bool
}

// TelegramSendMessagePayload defines JSON payload for Telegram sendMessage API.
type TelegramSendMessagePayload struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

// TelegramResponse defines standard Telegram API response.
type TelegramResponse struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

// NewTelegramClient creates a new Telegram HTTP client. If token or chatID is empty, it returns a disabled client.
func NewTelegramClient(token, chatID string) *TelegramClient {
	token = strings.TrimSpace(token)
	chatID = strings.TrimSpace(chatID)

	enabled := token != "" && chatID != ""

	return &TelegramClient{
		botToken: token,
		chatID:   chatID,
		baseURL:  "https://api.telegram.org",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		enabled: enabled,
	}
}

// SetBaseURL overrides the default Telegram API base URL (useful for testing).
func (tc *TelegramClient) SetBaseURL(url string) {
	if tc != nil {
		tc.baseURL = strings.TrimSuffix(url, "/")
	}
}

// SetHTTPClient overrides the default HTTP client (useful for testing).
func (tc *TelegramClient) SetHTTPClient(client *http.Client) {
	if tc != nil && client != nil {
		tc.httpClient = client
	}
}

// IsEnabled checks whether Telegram notifications are active.
func (tc *TelegramClient) IsEnabled() bool {
	return tc != nil && tc.enabled
}

// SendMessage sends a text message using MarkdownV2 formatting.
func (tc *TelegramClient) SendMessage(ctx context.Context, text string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("telegram client panic recovered: %v", r)
		}
	}()

	if tc == nil || !tc.enabled {
		// Graceful fallback - silently pass
		return nil
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", tc.baseURL, tc.botToken)

	payload := TelegramSendMessagePayload{
		ChatID:                tc.chatID,
		Text:                  text,
		ParseMode:             "MarkdownV2",
		DisableWebPagePreview: true,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram HTTP POST failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var tgResp TelegramResponse
		_ = json.Unmarshal(respBody, &tgResp)
		return fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, tgResp.Description)
	}

	return nil
}
