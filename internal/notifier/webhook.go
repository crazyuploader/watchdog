package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type WebhookPayload struct {
	URLs   []string `json:"urls"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Type   string   `json:"type"`
	Format string   `json:"format"`
}

type WebhookNotifier struct {
	WebhookURL string
	TargetURLs []string
}

func NewWebhookNotifier(webhookURL string, targetURLs []string) *WebhookNotifier {
	return &WebhookNotifier{
		WebhookURL: webhookURL,
		TargetURLs: targetURLs,
	}
}

func (w *WebhookNotifier) SendNotification(subject, message string) error {
	payload := WebhookPayload{
		URLs:   w.TargetURLs,
		Title:  subject,
		Body:   message,
		Type:   "info",
		Format: "text",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	resp, err := http.Post(w.WebhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to send webhook request: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
	}

	return nil
}
