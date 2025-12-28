package tasks

import (
	"fmt"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/notifier"
)

type TelnyxBalanceCheckTask struct {
	apiURL               string
	threshold            float64
	notificationCooldown time.Duration
	lastNotificationTime time.Time
	apiClient            *api.TelnyxAPI
	notifier             notifier.Notifier
}

func NewTelnyxBalanceCheckTask(apiURL, apiKey string, threshold float64, cooldown time.Duration, notifier notifier.Notifier) *TelnyxBalanceCheckTask {
	return &TelnyxBalanceCheckTask{
		apiURL:               apiURL,
		threshold:            threshold,
		notificationCooldown: cooldown,
		apiClient:            api.NewTelnyxAPI(apiURL, apiKey),
		notifier:             notifier,
	}
}

func (t *TelnyxBalanceCheckTask) Run() error {
	balance, err := t.apiClient.GetBalance()
	if err != nil {
		return fmt.Errorf("failed to get balance: %v", err)
	}

	fmt.Printf("Current balance: $%.2f\n", balance)

	if balance < t.threshold {
		// Check cooldown
		if !t.lastNotificationTime.IsZero() && time.Since(t.lastNotificationTime) < t.notificationCooldown {
			fmt.Printf("Balance below threshold, but notification skipped due to cooldown (last sent: %v)\n", t.lastNotificationTime)
			return nil
		}

		subject := "Telnyx Balance Alert"
		message := fmt.Sprintf("Telnyx balance is below threshold: $%.2f", balance)
		err = t.notifier.SendNotification(subject, message)
		if err != nil {
			return fmt.Errorf("failed to send notification: %v", err)
		}
		t.lastNotificationTime = time.Now()
	}

	return nil
}
