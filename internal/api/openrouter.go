package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// OpenRouterKeyResponse is the JSON response from the OpenRouter /api/v1/key endpoint.
// This provides information about the API key including usage and any free tier limits.
type OpenRouterKeyResponse struct {
	Data struct {
		Usage      float64  `json:"usage"`
		Limit      *float64 `json:"limit"`
		IsFreeTier bool     `json:"is_free_tier"`
	} `json:"data"`
}

// OpenRouterCreditsResponse is the JSON response from the OpenRouter /api/v1/credits endpoint.
// This provides total purchased credits and total usage; remaining credits are their difference.
type OpenRouterCreditsResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// OpenRouterActivityResponse is the JSON response from the OpenRouter /api/v1/activity endpoint.
// Activity is grouped by completed UTC day, model, and provider endpoint.
type OpenRouterActivityResponse struct {
	Data []OpenRouterActivityEntry `json:"data"`
}

// OpenRouterActivityEntry contains usage and request counts for one activity group.
type OpenRouterActivityEntry struct {
	Date               string  `json:"date"`
	ModelPermaslug     string  `json:"model_permaslug"`
	EndpointID         string  `json:"endpoint_id"`
	Usage              float64 `json:"usage"`
	BYOKUsageInference float64 `json:"byok_usage_inference"`
	Requests           int     `json:"requests"`
	PromptTokens       int64   `json:"prompt_tokens"`
	CompletionTokens   int64   `json:"completion_tokens"`
	ReasoningTokens    int64   `json:"reasoning_tokens"`
	BYOKRequests       int     `json:"byok_requests"`
	Model              string  `json:"model"`
	ProviderName       string  `json:"provider_name"`
}

// OpenRouterAPI is a client for interacting with the OpenRouter API.
// It handles authentication and provides methods for checking account credits and usage limits.
type OpenRouterAPI struct {
	BaseURL string
	APIKey  string
}

// NewOpenRouterAPI creates a new OpenRouter API client.
func NewOpenRouterAPI(baseURL, apiKey string) *OpenRouterAPI {
	return &OpenRouterAPI{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
}

// GetAuthKey fetches the OpenRouter API key info including usage and limit.
func (o *OpenRouterAPI) GetAuthKey(ctx context.Context) (*OpenRouterKeyResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/key", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth key request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+o.APIKey)
	req.Header.Add("Accept", "application/json")

	resp, err := DoWithRetry(ctx, DefaultHTTPClient, req, DefaultRetryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch auth key info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("auth key request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read auth key response body: %w", err)
	}

	var keyResp OpenRouterKeyResponse
	err = json.Unmarshal(body, &keyResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal auth key response: %w", err)
	}

	return &keyResp, nil
}

// GetCredits fetches the OpenRouter account credit balance and usage.
func (o *OpenRouterAPI) GetCredits(ctx context.Context) (*OpenRouterCreditsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.BaseURL+"/credits", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credits request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+o.APIKey)
	req.Header.Add("Accept", "application/json")

	resp, err := DoWithRetry(ctx, DefaultHTTPClient, req, DefaultRetryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credits: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("credits request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read credits response body: %w", err)
	}

	var creditsResp OpenRouterCreditsResponse
	err = json.Unmarshal(body, &creditsResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal credits response: %w", err)
	}

	return &creditsResp, nil
}

// GetActivity fetches OpenRouter activity for a completed UTC date.
// Passing apiKeyHash filters the activity to one API key; an empty hash returns account-level activity.
func (o *OpenRouterAPI) GetActivity(ctx context.Context, date, apiKeyHash string) (*OpenRouterActivityResponse, error) {
	endpoint, err := url.Parse(o.BaseURL + "/activity")
	if err != nil {
		return nil, fmt.Errorf("failed to parse activity URL: %w", err)
	}

	q := endpoint.Query()
	q.Set("date", date)
	if apiKeyHash != "" {
		q.Set("api_key_hash", apiKeyHash)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create activity request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+o.APIKey)
	req.Header.Add("Accept", "application/json")

	resp, err := DoWithRetry(ctx, DefaultHTTPClient, req, DefaultRetryConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch activity: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("activity request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read activity response body: %w", err)
	}

	var activityResp OpenRouterActivityResponse
	err = json.Unmarshal(body, &activityResp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal activity response: %w", err)
	}

	return &activityResp, nil
}
