package api

import "context"

// OpenRouterClient defines the interface for OpenRouter API operations.
// Swappable in tests.
type OpenRouterClient interface {
	GetAuthKey(ctx context.Context) (*OpenRouterKeyResponse, error)
	GetCredits(ctx context.Context) (*OpenRouterCreditsResponse, error)
	GetActivity(ctx context.Context, date, apiKeyHash string) (*OpenRouterActivityResponse, error)
}

// Compile-time interface check.
var _ OpenRouterClient = (*OpenRouterAPI)(nil)
