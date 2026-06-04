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

func TestNewOpenRouterAPI(t *testing.T) {
	baseURL := "https://openrouter.ai/api/v1"
	apiKey := "sk-or-v1-test"

	api := NewOpenRouterAPI(baseURL, apiKey)

	assert.NotNil(t, api)
	assert.Equal(t, baseURL, api.BaseURL)
	assert.Equal(t, apiKey, api.APIKey)
}

func TestOpenRouterAPI_GetAuthKey_Success(t *testing.T) {
	tests := []struct {
		name             string
		usage            float64
		limit            interface{}
		isFreeTier       bool
		expectedUsage    float64
		expectedLimitNil bool
		expectedLimitVal float64
	}{
		{
			name:             "free tier with limit",
			usage:            45.50,
			limit:            500.0,
			isFreeTier:       true,
			expectedUsage:    45.50,
			expectedLimitNil: false,
			expectedLimitVal: 500.0,
		},
		{
			name:             "paid tier no limit",
			usage:            100.0,
			limit:            nil,
			isFreeTier:       false,
			expectedUsage:    100.0,
			expectedLimitNil: true,
		},
		{
			name:             "zero usage",
			usage:            0,
			limit:            100.0,
			isFreeTier:       true,
			expectedUsage:    0,
			expectedLimitNil: false,
			expectedLimitVal: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/key", r.URL.Path)
				assert.Equal(t, "Bearer testkey", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))

				resp := map[string]interface{}{
					"data": map[string]interface{}{
						"usage":        tt.usage,
						"limit":        tt.limit,
						"is_free_tier": tt.isFreeTier,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			api := &OpenRouterAPI{
				BaseURL: server.URL,
				APIKey:  "testkey",
			}

			ctx := context.Background()
			keyResp, err := api.GetAuthKey(ctx)

			require.NoError(t, err)
			require.NotNil(t, keyResp)
			assert.Equal(t, tt.expectedUsage, keyResp.Data.Usage)
			assert.Equal(t, tt.isFreeTier, keyResp.Data.IsFreeTier)

			if tt.expectedLimitNil {
				assert.Nil(t, keyResp.Data.Limit)
			} else {
				require.NotNil(t, keyResp.Data.Limit)
				assert.Equal(t, tt.expectedLimitVal, *keyResp.Data.Limit)
			}
		})
	}
}

func TestOpenRouterAPI_GetAuthKey_NonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"error":{"code":"unauthorized","message":"Invalid API key"}}`,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"forbidden","message":"Access denied"}}`,
		},
		{
			name:       "429 rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"code":"rate_limited","message":"Too many requests"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			api := &OpenRouterAPI{
				BaseURL: server.URL,
				APIKey:  "testkey",
			}

			ctx := context.Background()
			keyResp, err := api.GetAuthKey(ctx)

			assert.Error(t, err)
			assert.Nil(t, keyResp)
			assert.Contains(t, err.Error(), "auth key request failed")
		})
	}
}

func TestOpenRouterAPI_GetAuthKey_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx := context.Background()
	keyResp, err := api.GetAuthKey(ctx)

	assert.Error(t, err)
	assert.Nil(t, keyResp)
	assert.Contains(t, err.Error(), "failed to unmarshal auth key response")
}

func TestOpenRouterAPI_GetAuthKey_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second)
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	keyResp, err := api.GetAuthKey(ctx)

	assert.Error(t, err)
	assert.Nil(t, keyResp)
}

func TestOpenRouterAPI_GetCredits_Success(t *testing.T) {
	tests := []struct {
		name            string
		totalCredits    float64
		totalUsage      float64
		expectedCredits float64
		expectedUsage   float64
	}{
		{
			name:            "positive credits with usage",
			totalCredits:    75.25,
			totalUsage:      24.75,
			expectedCredits: 75.25,
			expectedUsage:   24.75,
		},
		{
			name:            "zero credits",
			totalCredits:    0,
			totalUsage:      100.0,
			expectedCredits: 0,
			expectedUsage:   100.0,
		},
		{
			name:            "high credits no usage",
			totalCredits:    500.0,
			totalUsage:      0,
			expectedCredits: 500.0,
			expectedUsage:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/credits", r.URL.Path)
				assert.Equal(t, "Bearer testkey", r.Header.Get("Authorization"))

				resp := map[string]interface{}{
					"data": map[string]interface{}{
						"total_credits": tt.totalCredits,
						"total_usage":   tt.totalUsage,
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			api := &OpenRouterAPI{
				BaseURL: server.URL,
				APIKey:  "testkey",
			}

			ctx := context.Background()
			creditsResp, err := api.GetCredits(ctx)

			require.NoError(t, err)
			require.NotNil(t, creditsResp)
			assert.Equal(t, tt.expectedCredits, creditsResp.Data.TotalCredits)
			assert.Equal(t, tt.expectedUsage, creditsResp.Data.TotalUsage)
		})
	}
}

func TestOpenRouterAPI_GetCredits_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx := context.Background()
	creditsResp, err := api.GetCredits(ctx)

	assert.Error(t, err)
	assert.Nil(t, creditsResp)
	assert.Contains(t, err.Error(), "credits request failed")
}

func TestOpenRouterAPI_GetCredits_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx := context.Background()
	creditsResp, err := api.GetCredits(ctx)

	assert.Error(t, err)
	assert.Nil(t, creditsResp)
	assert.Contains(t, err.Error(), "failed to unmarshal credits response")
}

func TestOpenRouterAPI_GetCredits_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second)
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	creditsResp, err := api.GetCredits(ctx)

	assert.Error(t, err)
	assert.Nil(t, creditsResp)
}

func TestOpenRouterAPI_GetActivity_Success(t *testing.T) {
	tests := []struct {
		name       string
		apiKeyHash string
	}{
		{
			name:       "account level activity",
			apiKeyHash: "",
		},
		{
			name:       "api key filtered activity",
			apiKeyHash: "hash_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/activity", r.URL.Path)
				assert.Equal(t, "2026-06-03", r.URL.Query().Get("date"))
				assert.Equal(t, tt.apiKeyHash, r.URL.Query().Get("api_key_hash"))
				assert.Equal(t, "Bearer testkey", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))

				resp := map[string]interface{}{
					"data": []map[string]interface{}{
						{
							"date":                 "2026-06-03 00:00:00",
							"model_permaslug":      "openrouter/owl-alpha",
							"endpoint_id":          "endpoint-123",
							"usage":                0,
							"byok_usage_inference": 0,
							"requests":             274,
							"prompt_tokens":        14993669,
							"completion_tokens":    111935,
							"reasoning_tokens":     0,
							"byok_requests":        0,
							"model":                "openrouter/owl-alpha",
							"provider_name":        "stealth/int8",
						},
					},
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			api := &OpenRouterAPI{
				BaseURL: server.URL,
				APIKey:  "testkey",
			}

			ctx := context.Background()
			activityResp, err := api.GetActivity(ctx, "2026-06-03", tt.apiKeyHash)

			require.NoError(t, err)
			require.NotNil(t, activityResp)
			require.Len(t, activityResp.Data, 1)
			assert.Equal(t, 274, activityResp.Data[0].Requests)
			assert.Equal(t, "openrouter/owl-alpha", activityResp.Data[0].ModelPermaslug)
			assert.Equal(t, "stealth/int8", activityResp.Data[0].ProviderName)
		})
	}
}

func TestOpenRouterAPI_GetActivity_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Date must be within the last 30 completed UTC days","code":400}}`))
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx := context.Background()
	activityResp, err := api.GetActivity(ctx, "2026-06-04", "")

	assert.Error(t, err)
	assert.Nil(t, activityResp)
	assert.Contains(t, err.Error(), "activity request failed")
}

func TestOpenRouterAPI_GetActivity_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	api := &OpenRouterAPI{
		BaseURL: server.URL,
		APIKey:  "testkey",
	}

	ctx := context.Background()
	activityResp, err := api.GetActivity(ctx, "2026-06-03", "")

	assert.Error(t, err)
	assert.Nil(t, activityResp)
	assert.Contains(t, err.Error(), "failed to unmarshal activity response")
}

func TestOpenRouterKeyResponse_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"data": {
			"usage": 45.50,
			"limit": 500.0,
			"is_free_tier": true
		}
	}`

	var resp OpenRouterKeyResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	require.NoError(t, err)
	assert.Equal(t, 45.50, resp.Data.Usage)
	require.NotNil(t, resp.Data.Limit)
	assert.Equal(t, 500.0, *resp.Data.Limit)
	assert.True(t, resp.Data.IsFreeTier)
}

func TestOpenRouterKeyResponse_JSONUnmarshal_NullLimit(t *testing.T) {
	jsonData := `{
		"data": {
			"usage": 100.0,
			"limit": null,
			"is_free_tier": false
		}
	}`

	var resp OpenRouterKeyResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	require.NoError(t, err)
	assert.Equal(t, 100.0, resp.Data.Usage)
	assert.Nil(t, resp.Data.Limit)
	assert.False(t, resp.Data.IsFreeTier)
}

func TestOpenRouterCreditsResponse_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"data": {
			"total_credits": 75.25,
			"total_usage": 24.75
		}
	}`

	var resp OpenRouterCreditsResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	require.NoError(t, err)
	assert.Equal(t, 75.25, resp.Data.TotalCredits)
	assert.Equal(t, 24.75, resp.Data.TotalUsage)
}

func TestOpenRouterActivityResponse_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"data": [
			{
				"date": "2026-06-03 00:00:00",
				"model_permaslug": "openrouter/owl-alpha",
				"endpoint_id": "703f11d6-5f20-473e-b2ba-d9e8df3a6d56",
				"usage": 0,
				"byok_usage_inference": 0,
				"requests": 274,
				"prompt_tokens": 14993669,
				"completion_tokens": 111935,
				"reasoning_tokens": 0,
				"byok_requests": 0,
				"model": "openrouter/owl-alpha",
				"provider_name": "stealth/int8"
			}
		]
	}`

	var resp OpenRouterActivityResponse
	err := json.Unmarshal([]byte(jsonData), &resp)

	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "2026-06-03 00:00:00", resp.Data[0].Date)
	assert.Equal(t, "openrouter/owl-alpha", resp.Data[0].ModelPermaslug)
	assert.Equal(t, "703f11d6-5f20-473e-b2ba-d9e8df3a6d56", resp.Data[0].EndpointID)
	assert.Equal(t, 274, resp.Data[0].Requests)
	assert.Equal(t, int64(14993669), resp.Data[0].PromptTokens)
	assert.Equal(t, "stealth/int8", resp.Data[0].ProviderName)
}
