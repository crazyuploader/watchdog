package tasks

import (
	"context"
	"fmt"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/notifier"

	"github.com/rs/zerolog/log"
)

// OpenRouterBalanceCheckTask monitors your OpenRouter account credits and usage limits.
// It periodically checks:
//  1. Remaining credit balance (from purchased top-ups)
//  2. Usage relative to any set free tier/monthly limit
//  3. Request counts for the latest completed UTC day
//
// The task:
//  1. Fetches the current credits and usage info from OpenRouter API
//  2. Compares remaining balance against the configured threshold
//  3. Compares usage/limit ratio against the configured threshold (if limit is set)
//  4. Sends a combined notification if any condition is triggered (with cooldown)
type OpenRouterBalanceCheckTask struct {
	// balanceThreshold is the minimum acceptable remaining credits.
	// If remaining credits < balanceThreshold, an alert is sent.
	// 0 disables balance monitoring.
	balanceThreshold float64

	// usageLimitRatio is the threshold ratio for usage vs limit alerts.
	// If the API key has a usage limit set, an alert is sent when
	// usage exceeds limit * usageLimitRatio (e.g., 0.8 = 80%).
	usageLimitRatio float64

	// notificationCooldown prevents spam by limiting alert frequency.
	notificationCooldown time.Duration

	// dailyRequestLimit is the maximum allowed request count for a completed UTC day.
	// 0 disables request count monitoring.
	dailyRequestLimit int

	// dailyRequestRatio is the threshold ratio for daily request alerts.
	// If the request count exceeds dailyRequestLimit * dailyRequestRatio, an alert is sent.
	dailyRequestRatio float64

	// dailyRequestFreeOnly counts only zero-billed activity rows when true.
	dailyRequestFreeOnly bool

	// activityAPIKeyHash optionally filters activity checks to one API key.
	activityAPIKeyHash string

	// lastNotificationTime tracks when we last sent an alert.
	lastNotificationTime time.Time

	// apiClient is used to fetch data from OpenRouter API.
	apiClient api.OpenRouterClient

	// notifier is used to send alerts (via Apprise/Telegram/Discord/etc.).
	notifier notifier.Notifier

	// lastKnownBalance tracks the previously fetched remaining credits.
	lastKnownBalance float64

	// lastKnownUsage tracks the previously fetched usage value.
	lastKnownUsage float64

	// lastKnownDailyRequestDate tracks the latest activity date we logged.
	lastKnownDailyRequestDate string

	// lastKnownDailyRequests tracks the previously fetched daily request count.
	lastKnownDailyRequests int

	// now returns the current time. Tests may override this for deterministic activity dates.
	now func() time.Time

	// hasRunBefore indicates if this task has executed at least once.
	hasRunBefore bool
}

// OpenRouterBalanceCheckOptions configures OpenRouter balance, usage, and request monitoring.
type OpenRouterBalanceCheckOptions struct {
	BaseURL              string
	APIKey               string
	BalanceThreshold     float64
	UsageLimitRatio      float64
	DailyRequestLimit    int
	DailyRequestRatio    float64
	DailyRequestFreeOnly bool
	ActivityAPIKeyHash   string
	NotificationCooldown time.Duration
	Notifier             notifier.Notifier
}

// NewOpenRouterBalanceCheckTask creates a new OpenRouter balance monitoring task.
func NewOpenRouterBalanceCheckTask(
	baseURL, apiKey string,
	balanceThreshold, usageLimitRatio float64,
	cooldown time.Duration,
	notifier notifier.Notifier,
) *OpenRouterBalanceCheckTask {
	return NewOpenRouterBalanceCheckTaskWithOptions(OpenRouterBalanceCheckOptions{
		BaseURL:              baseURL,
		APIKey:               apiKey,
		BalanceThreshold:     balanceThreshold,
		UsageLimitRatio:      usageLimitRatio,
		DailyRequestFreeOnly: true,
		NotificationCooldown: cooldown,
		Notifier:             notifier,
	})
}

// NewOpenRouterBalanceCheckTaskWithOptions creates a new OpenRouter monitoring task.
func NewOpenRouterBalanceCheckTaskWithOptions(opts OpenRouterBalanceCheckOptions) *OpenRouterBalanceCheckTask {
	return &OpenRouterBalanceCheckTask{
		balanceThreshold:     opts.BalanceThreshold,
		usageLimitRatio:      opts.UsageLimitRatio,
		notificationCooldown: opts.NotificationCooldown,
		dailyRequestLimit:    opts.DailyRequestLimit,
		dailyRequestRatio:    opts.DailyRequestRatio,
		dailyRequestFreeOnly: opts.DailyRequestFreeOnly,
		activityAPIKeyHash:   opts.ActivityAPIKeyHash,
		apiClient:            api.NewOpenRouterAPI(opts.BaseURL, opts.APIKey),
		notifier:             opts.Notifier,
		now:                  time.Now,
	}
}

// Run executes the credit monitoring logic.
func (t *OpenRouterBalanceCheckTask) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var alerts []string
	var remaining float64
	var usage float64
	hasCredits := false
	hasUsage := false

	if t.balanceThreshold > 0 {
		credits, err := t.apiClient.GetCredits(ctx)
		if err != nil {
			return fmt.Errorf("failed to get credits: %w", err)
		}

		remaining = credits.Data.TotalCredits - credits.Data.TotalUsage
		hasCredits = true

		if remaining < t.balanceThreshold {
			alerts = append(alerts, fmt.Sprintf(
				"Remaining credits ($%.2f) have fallen below the $%.2f threshold.",
				remaining, t.balanceThreshold,
			))
		}
	}

	if t.usageLimitRatio > 0 {
		authKey, err := t.apiClient.GetAuthKey(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth key info: %w", err)
		}

		usage = authKey.Data.Usage
		hasUsage = true

		if authKey.Data.Limit != nil && *authKey.Data.Limit > 0 {
			ratio := usage / *authKey.Data.Limit
			if ratio > t.usageLimitRatio {
				alerts = append(alerts, fmt.Sprintf(
					"Usage ($%.2f / $%.2f = %.0f%%) exceeds %.0f%% of your monthly limit.",
					usage, *authKey.Data.Limit, ratio*100, t.usageLimitRatio*100,
				))
			}
		}
	}

	if hasCredits || hasUsage {
		shouldLog := !t.hasRunBefore ||
			(hasCredits && remaining != t.lastKnownBalance) ||
			(hasUsage && usage != t.lastKnownUsage)
		if shouldLog {
			event := log.Info()
			if hasCredits {
				event = event.Float64("remaining_credits", remaining)
			}
			if hasUsage {
				event = event.Float64("usage", usage)
			}
			event.Msg("Current OpenRouter credits")
			if hasCredits {
				t.lastKnownBalance = remaining
			}
			if hasUsage {
				t.lastKnownUsage = usage
			}
			t.hasRunBefore = true
		}
	}

	if t.dailyRequestLimit > 0 && t.dailyRequestRatio > 0 {
		activityDate := latestCompletedUTCDate(t.currentTime())
		activity, err := t.apiClient.GetActivity(ctx, activityDate, t.activityAPIKeyHash)
		if err != nil {
			return fmt.Errorf("failed to get activity for %s: %w", activityDate, err)
		}

		dailyRequests := countOpenRouterActivityRequests(activity.Data, t.dailyRequestFreeOnly)
		if activityDate != t.lastKnownDailyRequestDate || dailyRequests != t.lastKnownDailyRequests {
			log.Info().
				Str("activity_date", activityDate).
				Int("requests", dailyRequests).
				Int("request_limit", t.dailyRequestLimit).
				Bool("free_only", t.dailyRequestFreeOnly).
				Msg("Current OpenRouter daily requests for latest completed UTC day")
			t.lastKnownDailyRequestDate = activityDate
			t.lastKnownDailyRequests = dailyRequests
		}

		requestRatio := float64(dailyRequests) / float64(t.dailyRequestLimit)
		if requestRatio >= t.dailyRequestRatio {
			requestScope := "requests"
			if t.dailyRequestFreeOnly {
				requestScope = "free requests"
			}
			alerts = append(alerts, fmt.Sprintf(
				"OpenRouter %s for completed UTC day %s (%d / %d = %.0f%%) reached %.0f%% of the configured daily limit.",
				requestScope, activityDate, dailyRequests, t.dailyRequestLimit, requestRatio*100, t.dailyRequestRatio*100,
			))
		}
	}

	if len(alerts) > 0 {
		now := t.currentTime()
		if !t.lastNotificationTime.IsZero() && now.Sub(t.lastNotificationTime) < t.notificationCooldown {
			log.Info().
				Float64("remaining_credits", remaining).
				Float64("usage", usage).
				Time("last_sent", t.lastNotificationTime).
				Msg("OpenRouter credit alert pending, skipping due to cooldown")
			return nil
		}

		subject := "OpenRouter Credits Alert"
		message := "OpenRouter credit monitoring found the following issues:\n\n"
		for _, alert := range alerts {
			message += "- " + alert + "\n"
		}

		if err := t.notifier.SendNotification(ctx, subject, message); err != nil {
			return fmt.Errorf("failed to send notification: %w", err)
		}

		t.lastNotificationTime = now
	}

	return nil
}

func (t *OpenRouterBalanceCheckTask) currentTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func latestCompletedUTCDate(now time.Time) string {
	return now.UTC().AddDate(0, 0, -1).Format(time.DateOnly)
}

func countOpenRouterActivityRequests(entries []api.OpenRouterActivityEntry, freeOnly bool) int {
	total := 0
	for _, entry := range entries {
		if !freeOnly {
			total += entry.Requests
			continue
		}

		if entry.Usage != 0 || entry.BYOKUsageInference != 0 {
			continue
		}

		freeRequests := entry.Requests - entry.BYOKRequests
		if freeRequests > 0 {
			total += freeRequests
		}
	}
	return total
}
