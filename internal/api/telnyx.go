package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// TelnyxBalanceResponse represents the JSON structure returned by the Telnyx balance API.
// The Telnyx API returns balance as a string (e.g., "25.50") which we need to parse into a float.
// Example response: {"data": {"balance": "25.50", "currency": "USD"}}
type TelnyxBalanceResponse struct {
	Data struct {
		// Balance is the account balance as a string (e.g., "25.50")
		// We convert this to float64 for comparison with the threshold
		Balance string `json:"balance"`

		// Currency is the currency code (e.g., "USD")
		// Currently not used but included for completeness
		Currency string `json:"currency"`
	} `json:"data"`
}

// TelnyxAPI is a client for interacting with the Telnyx REST API.
// It handles authentication and provides methods for checking account balance.
type TelnyxAPI struct {
	// APIURL is the Telnyx API endpoint (usually https://api.telnyx.com/v2/balance)
	APIURL string

	// APIKey is your Telnyx API key for authentication (starts with "KEY...")
	// This is sent as a Bearer token in the Authorization header
	APIKey string
}

// NewTelnyxAPI creates a new Telnyx API client.
// Parameters:
//   - apiURL: The Telnyx API endpoint (e.g., "https://api.telnyx.com/v2/balance")
// NewTelnyxAPI creates a TelnyxAPI client configured with the provided API URL and API key.
// The apiKey should be a Telnyx API key (typically begins with "KEY...").
func NewTelnyxAPI(apiURL, apiKey string) *TelnyxAPI {
	return &TelnyxAPI{
		APIURL: apiURL,
		APIKey: apiKey,
	}
}

// GetBalance fetches the current account balance from Telnyx.
// It makes an authenticated GET request to the Telnyx API and parses the balance.
//
// Returns:
//   - The account balance as a float64 (e.g., 25.50)
//   - An error if the request fails, authentication fails, or the response is invalid
//
// The balance is returned as a float so it can be easily compared with the threshold
// configured in the application settings.
func (t *TelnyxAPI) GetBalance() (float64, error) {
	// Create HTTP client with a 10-second timeout to prevent hanging
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create GET request to the balance endpoint
	req, err := http.NewRequest("GET", t.APIURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	// Add authentication header - Telnyx uses Bearer token authentication
	req.Header.Add("Authorization", "Bearer "+t.APIKey)
	req.Header.Add("Accept", "application/json")

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch balance: %v", err)
	}

	// Ensure response body is closed when we're done
	// We explicitly ignore the error since there's nothing we can do about it
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the request was successful
	// Non-200 status could indicate authentication failure or API issues
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse the JSON response
	var balanceResponse TelnyxBalanceResponse
	err = json.Unmarshal(body, &balanceResponse)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	// Convert the balance string to a float
	// Telnyx returns balance as a string, so we need to parse it
	balance, err := strconv.ParseFloat(balanceResponse.Data.Balance, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse balance string '%s': %v", balanceResponse.Data.Balance, err)
	}

	return balance, nil
}