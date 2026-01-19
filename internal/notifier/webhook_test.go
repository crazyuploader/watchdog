package notifier

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebhookNotifier(t *testing.T) {
	webhookURL := "https://apprise.example.com/notify"
	targetURLs := []string{"tgram://token/id", "discord://webhook/token"}

	notifier := NewWebhookNotifier(webhookURL, targetURLs)

	assert.NotNil(t, notifier)
	assert.Equal(t, webhookURL, notifier.WebhookURL)
	assert.Equal(t, targetURLs, notifier.TargetURLs)
}

func TestWebhookNotifier_SendNotification_Success(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and headers
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Read and parse request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(body, &receivedPayload)
		require.NoError(t, err)

		// Send success response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURLs := []string{"tgram://token/id"}
	notifier := NewWebhookNotifier(server.URL, targetURLs)

	subject := "Test Alert"
	message := "This is a test message"

	err := notifier.SendNotification(subject, message)

	assert.NoError(t, err)
	assert.Equal(t, subject, receivedPayload.Title)
	assert.Equal(t, message, receivedPayload.Body)
	assert.Equal(t, "info", receivedPayload.Type)
	assert.Equal(t, "text", receivedPayload.Format)
	assert.Equal(t, targetURLs, receivedPayload.URLs)
}

func TestWebhookNotifier_SendNotification_MultipleTargets(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	targetURLs := []string{
		"tgram://token1/id1",
		"discord://webhook1/token1",
		"mailto://user:pass@gmail.com",
	}
	notifier := NewWebhookNotifier(server.URL, targetURLs)

	err := notifier.SendNotification("Subject", "Message")

	assert.NoError(t, err)
	assert.Len(t, receivedPayload.URLs, 3)
	assert.Equal(t, targetURLs, receivedPayload.URLs)
}

func TestWebhookNotifier_SendNotification_Non2xxStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"bad request", http.StatusBadRequest},
		{"unauthorized", http.StatusUnauthorized},
		{"forbidden", http.StatusForbidden},
		{"not found", http.StatusNotFound},
		{"internal server error", http.StatusInternalServerError},
		{"service unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
			err := notifier.SendNotification("Subject", "Message")

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "webhook request failed with status code")
		})
	}
}

func TestWebhookNotifier_SendNotification_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than 10s timeout
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject", "Message")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send webhook request")
}

func TestWebhookNotifier_SendNotification_EmptyTargets(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{})
	err := notifier.SendNotification("Subject", "Message")

	assert.NoError(t, err)
	assert.Empty(t, receivedPayload.URLs)
}

func TestWebhookNotifier_SendNotification_SpecialCharacters(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})

	subject := "Alert: Balance < $10.00"
	message := "Your balance is $5.50\nThis includes \"special\" characters & symbols!"

	err := notifier.SendNotification(subject, message)

	assert.NoError(t, err)
	assert.Equal(t, subject, receivedPayload.Title)
	assert.Equal(t, message, receivedPayload.Body)
}

func TestWebhookNotifier_SendNotification_LongMessage(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})

	// Create a very long message
	longMessage := ""
	for i := 0; i < 1000; i++ {
		longMessage += "This is a test message. "
	}

	err := notifier.SendNotification("Subject", longMessage)

	assert.NoError(t, err)
	assert.Equal(t, longMessage, receivedPayload.Body)
}

func TestWebhookNotifier_SendNotification_EmptySubject(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("", "Message only")

	assert.NoError(t, err)
	assert.Empty(t, receivedPayload.Title)
	assert.Equal(t, "Message only", receivedPayload.Body)
}

func TestWebhookNotifier_SendNotification_EmptyMessage(t *testing.T) {
	var receivedPayload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject only", "")

	assert.NoError(t, err)
	assert.Equal(t, "Subject only", receivedPayload.Title)
	assert.Empty(t, receivedPayload.Body)
}

func TestWebhookPayload_JSONMarshaling(t *testing.T) {
	payload := WebhookPayload{
		URLs:   []string{"tgram://token/id", "discord://webhook/token"},
		Title:  "Test Subject",
		Body:   "Test Message",
		Type:   "info",
		Format: "text",
	}

	// Marshal to JSON
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	// Unmarshal back
	var decoded WebhookPayload
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, payload.URLs, decoded.URLs)
	assert.Equal(t, payload.Title, decoded.Title)
	assert.Equal(t, payload.Body, decoded.Body)
	assert.Equal(t, payload.Type, decoded.Type)
	assert.Equal(t, payload.Format, decoded.Format)
}

func TestWebhookNotifier_SendNotification_ServerClosesConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection without sending response
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject", "Message")

	assert.Error(t, err)
}

func TestWebhookNotifier_SendNotification_Redirect(t *testing.T) {
	// Final destination server
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer finalServer.Close()

	// Redirect server
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	notifier := NewWebhookNotifier(redirectServer.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject", "Message")

	// HTTP client follows redirects by default
	assert.NoError(t, err)
}

func TestWebhookNotifier_SendNotification_201Accepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject", "Message")

	assert.NoError(t, err)
}

func TestWebhookNotifier_SendNotification_202Accepted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, []string{"tgram://token/id"})
	err := notifier.SendNotification("Subject", "Message")

	assert.NoError(t, err)
}
