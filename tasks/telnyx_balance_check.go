package tasks

import (
	"fmt"
	"watchdog/internal/api"
	"watchdog/internal/notifier"
)

type TelnyxBalanceCheckTask struct {
	apiURL    string
	threshold float64
	apiClient *api.TelnyxAPI
	notifier  notifier.Notifier
}

func NewTelnyxBalanceCheckTask(apiURL, apiKey string, threshold float64, notifier notifier.Notifier) *TelnyxBalanceCheckTask {
	return &TelnyxBalanceCheckTask{
		apiURL:    apiURL,
		threshold: threshold,
		apiClient: api.NewTelnyxAPI(apiURL, apiKey),
		notifier:  notifier,
	}
}

func (t *TelnyxBalanceCheckTask) Run() error {
	balance, err := t.apiClient.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get balance: %v", err)
	}

	fmt.Printf("Current balance: $%.2f\n", balance)

	if balance < t.threshold {
		subject := "Telnyx Balance Alert"
		message := fmt.Sprintf("Telnyx balance is below threshold: $%.2f", balance)
		err = t.notifier.SendNotification(subject, message)
		if err != nil {
			return fmt.Errorf("failed to send notification: %v", err)
		}
	}

	return nil
}
