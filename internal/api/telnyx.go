package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type TelnyxBalanceResponse struct {
	Data struct {
		Balance  string `json:"balance"`
		Currency string `json:"currency"`
	} `json:"data"`
}

type TelnyxAPI struct {
	APIURL string
	APIKey string
}

func NewTelnyxAPI(apiURL, apiKey string) *TelnyxAPI {
	return &TelnyxAPI{
		APIURL: apiURL,
		APIKey: apiKey,
	}
}

func (t *TelnyxAPI) GetBalance() (float64, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", t.APIURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Authorization", "Bearer "+t.APIKey)
	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch balance: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %v", err)
	}

	var balanceResponse TelnyxBalanceResponse
	err = json.Unmarshal(body, &balanceResponse)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	balance, err := strconv.ParseFloat(balanceResponse.Data.Balance, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse balance string '%s': %v", balanceResponse.Data.Balance, err)
	}

	return balance, nil
}
