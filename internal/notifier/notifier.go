package notifier

import (
	"fmt"
	"strings"
	"time"

	"django/internal/config"
	"django/internal/models"
)

// Notifier defines the event-driven notification manager interface/struct.
type Notifier struct {
	queue *NotificationQueue
}

// NewNotifier creates and initializes a new Notifier with token and chat ID.
func NewNotifier(botToken, chatID string) *Notifier {
	client := NewTelegramClient(botToken, chatID)
	queue := NewNotificationQueue(client, 100, 500*time.Millisecond)
	return &Notifier{
		queue: queue,
	}
}

// NewNotifierFromConfig creates Notifier from application Config.
func NewNotifierFromConfig(cfg *config.Config) *Notifier {
	if cfg == nil {
		return NewNotifier("", "")
	}
	return NewNotifier(cfg.TelegramBotToken, cfg.TelegramChatID)
}

// Start starts the underlying notification queue.
func (n *Notifier) Start() {
	if n != nil && n.queue != nil {
		n.queue.Start()
	}
}

// Stop gracefully stops the notifier and drains messages.
func (n *Notifier) Stop() {
	if n != nil && n.queue != nil {
		n.queue.Stop()
	}
}

// IsEnabled checks whether notifications are active.
func (n *Notifier) IsEnabled() bool {
	return n != nil && n.queue != nil && n.queue.IsEnabled()
}

// NotifyJobStarted sends a notification when a scan job starts.
func (n *Notifier) NotifyJobStarted(jobID uint, domain string, pipelineType string) {
	if !n.IsEnabled() {
		return
	}

	escapedDomain := EscapeMarkdownV2(domain)
	escapedPipeline := EscapeMarkdownV2(pipelineType)

	msg := fmt.Sprintf("🚀 *Scan Started*\n\n"+
		"*Job ID:* `%d`\n"+
		"*Target:* `%s`\n"+
		"*Pipeline:* `%s`",
		jobID, escapedDomain, escapedPipeline)

	n.queue.Enqueue(msg)
}

// NotifyJobCompleted sends a summary notification when a scan job completes successfully, including delta change counts.
func (n *Notifier) NotifyJobCompleted(jobID uint, domain string, duration time.Duration, liveSubdomains int, totalFindings int, newSubdomains int, newFindings int) {
	if !n.IsEnabled() {
		return
	}

	escapedDomain := EscapeMarkdownV2(domain)
	durStr := EscapeMarkdownV2(duration.Round(time.Second).String())

	subdomainsStr := fmt.Sprintf("%d", liveSubdomains)
	if newSubdomains > 0 {
		subdomainsStr += fmt.Sprintf(" _(\\+%d NEW 🆕)_", newSubdomains)
	}

	findingsStr := fmt.Sprintf("%d", totalFindings)
	if newFindings > 0 {
		findingsStr += fmt.Sprintf(" _(\\+%d NEW 🔥)_", newFindings)
	}

	msg := fmt.Sprintf("✅ *Scan Completed*\n\n"+
		"*Job ID:* `%d`\n"+
		"*Target:* `%s`\n"+
		"*Duration:* `%s`\n"+
		"*Live Subdomains:* %s\n"+
		"*Total Findings:* %s",
		jobID, escapedDomain, durStr, subdomainsStr, findingsStr)

	n.queue.Enqueue(msg)
}

// NotifyJobFailed sends a notification when a scan job fails.
func (n *Notifier) NotifyJobFailed(jobID uint, domain string, duration time.Duration, errStr string) {
	if !n.IsEnabled() {
		return
	}

	escapedDomain := EscapeMarkdownV2(domain)
	durStr := EscapeMarkdownV2(duration.Round(time.Second).String())
	escapedErr := EscapeMarkdownV2(errStr)

	msg := fmt.Sprintf("❌ *Scan Failed*\n\n"+
		"*Job ID:* `%d`\n"+
		"*Target:* `%s`\n"+
		"*Duration:* `%s`\n"+
		"*Error:* `%s`",
		jobID, escapedDomain, durStr, escapedErr)

	n.queue.Enqueue(msg)
}

// NotifyFinding sends an instant detailed alert if finding severity is critical or high.
func (n *Notifier) NotifyFinding(domain string, finding models.Finding) {
	if !n.IsEnabled() {
		return
	}

	severityLower := strings.ToLower(strings.TrimSpace(finding.Severity))
	if severityLower != "critical" && severityLower != "high" {
		return // Only alert on critical or high findings
	}

	emoji := "🔥"
	if severityLower == "critical" {
		emoji = "🚨"
	}

	escapedDomain := EscapeMarkdownV2(domain)
	escapedTitle := EscapeMarkdownV2(finding.Title)
	escapedSeverity := EscapeMarkdownV2(strings.ToUpper(finding.Severity))
	escapedTool := EscapeMarkdownV2(finding.ToolName)

	details := finding.Description
	if details == "" {
		details = finding.RawOutput
	}
	escapedDetails := EscapeMarkdownV2(details)

	msg := fmt.Sprintf("%s *%s FINDING DISCOVERED*\n\n"+
		"*Target:* `%s`\n"+
		"*Title:* `%s`\n"+
		"*Severity:* *%s*\n"+
		"*Tool:* `%s`\n"+
		"*Details:* %s",
		emoji, escapedSeverity, escapedDomain, escapedTitle, escapedSeverity, escapedTool, escapedDetails)

	n.queue.Enqueue(msg)
}
