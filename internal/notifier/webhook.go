package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookPayload represents the JSON structure sent to the Apprise API.
// Apprise is a universal notification library that supports 70+ notification services
// including Telegram, Discord, Slack, email, SMS, and many more.
//
// The payload tells Apprise:
//   - Which services to notify (URLs field)
//   - What the notification title should be (Title field)
//   - What the notification message should be (Body field)
//   - The notification type/severity (Type field: info, success, warning, failure)
//   - The message format (Format field: text, markdown, html)
type WebhookPayload struct {
	// URLs is a list of Apprise service URLs to send the notification to.
	// Examples:
	//   - "tgram://botToken/chatID" for Telegram
	//   - "discord://webhook_id/webhook_token" for Discord
	//   - "mailto://user:pass@gmail.com" for email
	URLs []string `json:"urls"`

	// Title is the notification subject/header
	Title string `json:"title"`

	// Body is the main notification message content
	Body string `json:"body"`

	// Type indicates the notification severity/type
	// Common values: "info", "success", "warning", "failure"
	Type string `json:"type"`

	// Format specifies how the body should be interpreted
	// Common values: "text", "markdown", "html"
	Format string `json:"format"`
}

// WebhookNotifier implements the Notifier interface using Apprise webhooks.
// It sends notifications by making HTTP POST requests to an Apprise API server,
// which then forwards the notifications to configured services (Telegram, Discord, etc.)
//
// This is the primary notification backend used by watchdog.
type WebhookNotifier struct {
	// WebhookURL is the Apprise API endpoint (e.g., "https://apprise.example.com/notify")
	WebhookURL string

	// TargetURLs is a list of Apprise service URLs to send notifications to.
	// These are parsed from the comma-separated apprise_service_url config value.
	TargetURLs []string
}

// NewWebhookNotifier creates a new webhook-based notifier.
// Parameters:
//   - webhookURL: The Apprise API endpoint URL
//   - targetURLs: List of Apprise service URLs (Telegram, Discord, etc.)
//
// Example:
//
//	notifier := NewWebhookNotifier(
//	    "https://apprise.example.com/notify",
//	    []string{"tgram://botToken/chatID", "discord://webhook_id/token"}
// NewWebhookNotifier creates a WebhookNotifier configured to send notifications to an Apprise webhook.
// The webhookURL is the Apprise API endpoint (for example, "https://apprise.example.com/notify") and
// targetURLs are the Apprise service URLs (e.g., Telegram, Discord, email) that will receive notifications.
func NewWebhookNotifier(webhookURL string, targetURLs []string) *WebhookNotifier {
	return &WebhookNotifier{
		WebhookURL: webhookURL,
		TargetURLs: targetURLs,
	}
}

// SendNotification sends a notification via the Apprise webhook.
// It constructs a WebhookPayload, marshals it to JSON, and POSTs it to the Apprise API.
//
// Parameters:
//   - subject: The notification title (e.g., "Telnyx Balance Alert")
//   - message: The notification body (e.g., "Balance is $5.00, below $10.00 threshold")
//
// Returns:
//   - An error if the webhook request fails or returns a non-2xx status code
//   - nil if the notification was sent successfully
//
// The Apprise API will then forward the notification to all configured services
// (Telegram, Discord, etc.) specified in the TargetURLs.
func (w *WebhookNotifier) SendNotification(subject, message string) error {
	// Construct the payload for Apprise
	payload := WebhookPayload{
		URLs:   w.TargetURLs,
		Title:  subject,
		Body:   message,
		Type:   "info", // Could be made configurable in the future
		Format: "text", // Plain text format (could support markdown/html later)
	}

	// Marshal the payload to JSON
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	// Create HTTP client with timeout to prevent hanging on slow webhook endpoints
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create the POST request
	req, err := http.NewRequest("POST", w.WebhookURL, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %v", err)
	}

	// Ensure response body is closed
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the request was successful (2xx status code)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}