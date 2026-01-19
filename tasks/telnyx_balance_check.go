package tasks

import (
	"context"
	"fmt"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/notifier"

	"github.com/rs/zerolog/log"
)

// TelnyxBalanceCheckTask monitors your Telnyx account balance.
// It periodically checks the balance and sends an alert if it falls below a configured threshold.
//
// The task:
//  1. Fetches the current balance from Telnyx API
//  2. Compares it against the configured threshold
//  3. Sends a notification if balance is too low (with cooldown to prevent spam)
//
// This implements the scheduler.Task interface via the Run() method.
type TelnyxBalanceCheckTask struct {
	// threshold is the minimum acceptable balance in dollars
	// If balance < threshold, an alert is sent
	threshold float64

	// notificationCooldown prevents spam by limiting alert frequency
	// Default is 6 hours - we won't send another alert until this time has passed
	notificationCooldown time.Duration

	// lastNotificationTime tracks when we last sent a low balance alert
	// Used to enforce the cooldown period
	lastNotificationTime time.Time

	// apiClient is used to fetch balance data from Telnyx
	apiClient api.TelnyxClient

	// notifier is used to send alerts (via Apprise/Telegram/Discord/etc.)
	notifier notifier.Notifier

	// lastObservedBalance tracks the previously fetched balance
	// Used to deduplicate logs - we only log when the balance changes
	lastObservedBalance float64

	// hasRunBefore indicates if this task has executed at least once
	// Used to ensure we always log the balance on the very first run
	hasRunBefore bool
}

// NewTelnyxBalanceCheckTask creates a new Telnyx balance monitoring task.
// Parameters:
//   - apiURL: The Telnyx API endpoint (e.g., "https://api.telnyx.com/v2/balance")
//   - apiKey: Your Telnyx API key (starts with "KEY...")
//   - threshold: Minimum acceptable balance in dollars (e.g., 10.0)
//   - cooldown: How long to wait between notifications (e.g., 6*time.Hour)
//   - notifier: Where to send alerts (Apprise webhook, Telegram, etc.)
//
// Example:
//
//	task := NewTelnyxBalanceCheckTask(
//	    "https://api.telnyx.com/v2/balance",
//	    "KEY123...",
//	    10.0,
//	    6*time.Hour,
//	    myNotifier,
//	)
func NewTelnyxBalanceCheckTask(apiURL, apiKey string, threshold float64, cooldown time.Duration, notifier notifier.Notifier) *TelnyxBalanceCheckTask {
	return &TelnyxBalanceCheckTask{
		threshold:            threshold,
		notificationCooldown: cooldown,
		apiClient:            api.NewTelnyxAPI(apiURL, apiKey),
		notifier:             notifier,
	}
}

// Run executes the balance check logic.
// This method is called periodically by the scheduler (e.g., every 5 minutes).
//
// It performs the following steps:
//  1. Fetches the current balance from Telnyx API
//  2. Logs the balance to console
//  3. If balance < threshold:
//     a. Checks if we're still in the cooldown period
//     b. If cooldown expired, sends a notification
//     c. Records the notification time to start a new cooldown
//
// Returns:
//   - An error if the API request fails
//   - An error if the notification fails to send
//   - nil if everything succeeds or if no notification is needed
//
// The cooldown mechanism prevents spamming alerts every 5 minutes when balance is low.
// For example, with a 6-hour cooldown, you'll only get one alert every 6 hours.
func (t *TelnyxBalanceCheckTask) Run() error {
	// Create a context with a reasonable timeout for the task
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch current balance from Telnyx
	balance, err := t.apiClient.GetBalance(ctx)
	if err != nil {
		return fmt.Errorf("failed to get balance: %v", err)
	}

	// Log the balance ONLY if it has changed since the last check
	// This reduces log spam in the console
	if !t.hasRunBefore || balance != t.lastObservedBalance {
		log.Info().Float64("balance", balance).Msg("Current Telnyx balance")
		t.lastObservedBalance = balance
		t.hasRunBefore = true
	}

	// Check if balance is below threshold
	if balance < t.threshold {
		// Check notification cooldown
		// We don't want to spam notifications every 5 minutes when balance is low
		// Only send if we haven't notified recently (or if this is the first notification)
		if !t.lastNotificationTime.IsZero() && time.Since(t.lastNotificationTime) < t.notificationCooldown {
			log.Info().
				Float64("balance", balance).
				Time("last_sent", t.lastNotificationTime).
				Msg("Balance below threshold, skipping notification due to cooldown")
			return nil
		}

		// Balance is low and cooldown has expired - send notification
		subject := "Telnyx Balance Alert"
		message := fmt.Sprintf("Your Telnyx balance ($%.2f) has fallen below the $%.2f threshold.", balance, t.threshold)
		err = t.notifier.SendNotification(ctx, subject, message)
		if err != nil {
			return fmt.Errorf("failed to send notification: %v", err)
		}

		// Record that we sent a notification
		// This starts the cooldown period
		t.lastNotificationTime = time.Now()
	}

	return nil
}
