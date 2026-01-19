package api

import (
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultHTTPClient is a shared HTTP client with connection pooling.
// Reusing a single client avoids creating new connections for each request,
// improving performance and reducing resource usage.
var DefaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

// RetryConfig configures the retry behavior for HTTP requests.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries)
	MaxRetries int

	// InitialBackoff is the initial wait time before the first retry
	InitialBackoff time.Duration

	// MaxBackoff is the maximum wait time between retries
	MaxBackoff time.Duration

	// BackoffMultiplier increases the backoff time after each retry
	BackoffMultiplier float64
}

// DefaultRetryConfig provides sensible defaults for retry behavior.
var DefaultRetryConfig = RetryConfig{
	MaxRetries:        3,
	InitialBackoff:    500 * time.Millisecond,
	MaxBackoff:        10 * time.Second,
	BackoffMultiplier: 2.0,
}

// isRetryableError checks if an error is transient and worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Network timeout errors are retryable
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return false
}

// isRetryableStatusCode checks if an HTTP status code indicates a transient error.
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusTooManyRequests, // 429 - Rate limited
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// DoWithRetry executes an HTTP request with automatic retry on transient failures.
// It uses exponential backoff between retries and respects the context for cancellation.
//
// Parameters:
//   - ctx: Context for cancellation and deadline propagation
//   - client: HTTP client to use (typically DefaultHTTPClient)
//   - req: The HTTP request to execute
//   - config: Retry configuration (use DefaultRetryConfig for sensible defaults)
//
// Returns:
//   - The HTTP response if successful
//   - An error if all retries are exhausted or a non-retryable error occurs
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request, config RetryConfig) (*http.Response, error) {
	var lastErr error
	var resp *http.Response

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled before attempting
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Clone the request to ensure fresh body for retries
		reqClone := req.Clone(ctx)

		// Execute the request
		resp, lastErr = client.Do(reqClone)

		// Success - return the response
		if lastErr == nil && !isRetryableStatusCode(resp.StatusCode) {
			return resp, nil
		}

		// Check if we should retry
		shouldRetry := false
		if lastErr != nil && isRetryableError(lastErr) {
			shouldRetry = true
		} else if resp != nil && isRetryableStatusCode(resp.StatusCode) {
			shouldRetry = true
			// Close the response body before retrying to prevent resource leak
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		// If not retryable or out of retries, return
		if !shouldRetry || attempt >= config.MaxRetries {
			if lastErr != nil {
				return nil, lastErr
			}
			return resp, nil
		}

		// Calculate backoff with exponential increase
		backoff := float64(config.InitialBackoff) * math.Pow(config.BackoffMultiplier, float64(attempt))
		if backoff > float64(config.MaxBackoff) {
			backoff = float64(config.MaxBackoff)
		}

		log.Warn().
			Int("attempt", attempt+1).
			Int("max_retries", config.MaxRetries).
			Dur("backoff", time.Duration(backoff)).
			Msg("Request failed, retrying...")

		// Wait before retrying
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(backoff)):
		}
	}

	return resp, lastErr
}
