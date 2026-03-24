package api

import "context"

// TelnyxClient defines the interface for Telnyx API operations.
// This allows for easy mocking in tests.
type TelnyxClient interface {
	GetBalance(ctx context.Context) (float64, error)
}

// Ensure TelnyxAPI implements TelnyxClient interface at compile time
var _ TelnyxClient = (*TelnyxAPI)(nil)
