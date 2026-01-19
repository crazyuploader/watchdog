package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTelnyxAPI(t *testing.T) {
	apiURL := "https://api.telnyx.com/v2/balance"
	apiKey := "KEY123test"

	api := NewTelnyxAPI(apiURL, apiKey)

	assert.NotNil(t, api)
	assert.Equal(t, apiURL, api.APIURL)
	assert.Equal(t, apiKey, api.APIKey)
}

func TestTelnyxAPI_GetBalance_Success(t *testing.T) {
	tests := []struct {
		name            string
		balanceString   string
		expectedBalance float64
		currency        string
	}{
		{
			name:            "positive balance",
			balanceString:   "25.50",
			expectedBalance: 25.50,
			currency:        "USD",
		},
		{
			name:            "zero balance",
			balanceString:   "0.00",
			expectedBalance: 0.00,
			currency:        "USD",
		},
		{
			name:            "high balance",
			balanceString:   "1234.56",
			expectedBalance: 1234.56,
			currency:        "USD",
		},
		{
			name:            "low balance",
			balanceString:   "0.01",
			expectedBalance: 0.01,
			currency:        "USD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "Bearer testkey", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))

				// Send mock response
				resp := TelnyxBalanceResponse{}
				resp.Data.Balance = tt.balanceString
				resp.Data.Currency = tt.currency

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			api := &TelnyxAPI{
				APIURL: server.URL,
				APIKey: "testkey",
			}

			ctx := context.Background()
			balance, err := api.GetBalance(ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedBalance, balance)
		})
	}
}

func TestTelnyxAPI_GetBalance_NonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"errors":[{"code":"unauthorized","title":"Unauthorized"}]}`,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"errors":[{"code":"forbidden","title":"Forbidden"}]}`,
		},
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			body:       `{"errors":[{"code":"not_found","title":"Not Found"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			api := &TelnyxAPI{
				APIURL: server.URL,
				APIKey: "testkey",
			}

			ctx := context.Background()
			balance, err := api.GetBalance(ctx)
			assert.Error(t, err)
			assert.Equal(t, 0.0, balance)
			assert.Contains(t, err.Error(), "api request failed")
		})
	}
}

func TestTelnyxAPI_GetBalance_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`invalid json response`))
	}))
	defer server.Close()

	api := &TelnyxAPI{
		APIURL: server.URL,
		APIKey: "testkey",
	}

	ctx := context.Background()
	balance, err := api.GetBalance(ctx)
	assert.Error(t, err)
	assert.Equal(t, 0.0, balance)
	assert.Contains(t, err.Error(), "failed to unmarshal response")
}

func TestTelnyxAPI_GetBalance_InvalidBalanceString(t *testing.T) {
	tests := []struct {
		name          string
		balanceString string
	}{
		{
			name:          "non-numeric balance",
			balanceString: "not-a-number",
		},
		{
			name:          "empty balance",
			balanceString: "",
		},
		{
			name:          "invalid format",
			balanceString: "25.50.10",
		},
		{
			name:          "balance with currency symbol",
			balanceString: "$25.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				resp := TelnyxBalanceResponse{}
				resp.Data.Balance = tt.balanceString
				resp.Data.Currency = "USD"

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			api := &TelnyxAPI{
				APIURL: server.URL,
				APIKey: "testkey",
			}

			ctx := context.Background()
			balance, err := api.GetBalance(ctx)
			assert.Error(t, err)
			assert.Equal(t, 0.0, balance)
			assert.Contains(t, err.Error(), "failed to parse balance string")
		})
	}
}

func TestTelnyxAPI_GetBalance_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than timeout
	}))
	defer server.Close()

	api := &TelnyxAPI{
		APIURL: server.URL,
		APIKey: "testkey",
	}

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	balance, err := api.GetBalance(ctx)
	assert.Error(t, err)
	assert.Equal(t, 0.0, balance)
}

func TestTelnyxAPI_GetBalance_NegativeBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := TelnyxBalanceResponse{}
		resp.Data.Balance = "-10.50"
		resp.Data.Currency = "USD"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	api := &TelnyxAPI{
		APIURL: server.URL,
		APIKey: "testkey",
	}

	ctx := context.Background()
	balance, err := api.GetBalance(ctx)
	require.NoError(t, err)
	assert.Equal(t, -10.50, balance)
}

func TestTelnyxBalanceResponse_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"data": {
			"balance": "123.45",
			"currency": "USD"
		}
	}`

	var resp TelnyxBalanceResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	require.NoError(t, err)
	assert.Equal(t, "123.45", resp.Data.Balance)
	assert.Equal(t, "USD", resp.Data.Currency)
}
