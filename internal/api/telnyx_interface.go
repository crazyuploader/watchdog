package api

// TelnyxClient defines the interface for Telnyx API operations.
// This allows for easy mocking in tests.
type TelnyxClient interface {
	GetBalance() (float64, error)
}

// Ensure TelnyxAPI implements TelnyxClient interface
var _ TelnyxClient = (*TelnyxAPI)(nil)
