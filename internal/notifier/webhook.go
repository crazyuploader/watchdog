package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// webhookHTTPClient is a shared HTTP client for webhook requests.
var webhookHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

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
//   - webhookURL: The Apprise API endpoint URL (e.g., "https://apprise.example.com/notify")
//   - targetURLs: List of Apprise service URLs (Telegram, Discord, etc.)
//
// Example:
//
//	notifier := NewWebhookNotifier(
//	    "https://apprise.example.com/notify",
//	    []string{"tgram://botToken/chatID", "discord://webhook_id/token"},
//	)
func NewWebhookNotifier(webhookURL string, targetURLs []string) *WebhookNotifier {
	return &WebhookNotifier{
		WebhookURL: webhookURL,
		TargetURLs: targetURLs,
	}
}

// webhookRetryConfig defines retry behavior for webhook requests.
var webhookRetryConfig = struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
}{
	MaxRetries:        3,
	InitialBackoff:    500 * time.Millisecond,
	MaxBackoff:        10 * time.Second,
	BackoffMultiplier: 2.0,
}

// SendNotification sends a notification via the Apprise webhook.
// It constructs a WebhookPayload, marshals it to JSON, and POSTs it to the Apprise API.
//
// Parameters:
//   - ctx: Context for cancellation and deadline propagation
//   - subject: The notification title (e.g., "Telnyx Balance Alert")
//   - message: The notification body (e.g., "Balance is $5.00, below $10.00 threshold")
//
// Returns:
//   - An error if the webhook request fails or returns a non-2xx status code
//   - nil if the notification was sent successfully
//
// The Apprise API will then forward the notification to all configured services
// (Telegram, Discord, etc.) specified in the TargetURLs.
func (w *WebhookNotifier) SendNotification(ctx context.Context, subject, message string) error {
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

	// Retry loop with exponential backoff
	var lastErr error
	for attempt := 0; attempt <= webhookRetryConfig.MaxRetries; attempt++ {
		// Check context before attempting
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Create the POST request
		req, err := http.NewRequestWithContext(ctx, "POST", w.WebhookURL, bytes.NewBuffer(data))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err := webhookHTTPClient.Do(req)
		if err != nil {
			lastErr = err
			// Check if error is retryable (timeout)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if attempt < webhookRetryConfig.MaxRetries {
					backoff := calculateBackoff(attempt)
					log.Warn().
						Err(err).
						Int("attempt", attempt+1).
						Dur("backoff", backoff).
						Msg("Webhook request failed, retrying...")
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(backoff):
					}
					continue
				}
			}
			return fmt.Errorf("failed to send webhook request: %v", err)
		}

		// Ensure response body is closed
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		// Check if the request was successful (2xx status code)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}

		// Check if status code is retryable (5xx errors)
		if resp.StatusCode >= 500 && attempt < webhookRetryConfig.MaxRetries {
			backoff := calculateBackoff(attempt)
			log.Warn().
				Int("status_code", resp.StatusCode).
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("Webhook request failed, retrying...")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			continue
		}

		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	if lastErr != nil {
		return fmt.Errorf("failed to send webhook request after retries: %v", lastErr)
	}
	return nil
}

// calculateBackoff computes the backoff duration for a given attempt.
func calculateBackoff(attempt int) time.Duration {
	backoff := float64(webhookRetryConfig.InitialBackoff) * math.Pow(webhookRetryConfig.BackoffMultiplier, float64(attempt))
	if backoff > float64(webhookRetryConfig.MaxBackoff) {
		backoff = float64(webhookRetryConfig.MaxBackoff)
	}
	return time.Duration(backoff)
}
