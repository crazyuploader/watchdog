package config

import (
	"log"
	"strings"
	"time"
)

// Config is the root configuration structure that holds all application settings.
// It's populated from the YAML config file using Viper's mapstructure tags.
// The config file should contain sections for tasks, notifier, and scheduler.
type Config struct {
	Tasks     TasksConfig     `mapstructure:"tasks"`
	Notifier  NotifierConfig  `mapstructure:"notifier"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
}

// parseDurationWithDefault parses a duration string and returns a default if parsing fails.
// Logs a warning when an invalid duration string is encountered to help with debugging.
func parseDurationWithDefault(value string, defaultDuration time.Duration, configPath string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultDuration
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("Warning: Invalid duration '%s' for %s, using default %v (hint: use format like '5m', '1h', '24h')\n",
			value, configPath, defaultDuration)
		return defaultDuration
	}
	return d
}

// TasksConfig groups all task-specific configurations.
// Each task can optionally override the global scheduler interval.
type TasksConfig struct {
	Telnyx TelnyxConfig `mapstructure:"telnyx"`
	GitHub GitHubConfig `mapstructure:"github"`
}

// GitHubConfig holds all settings for GitHub pull request monitoring.
// This feature monitors specified repositories for stale PRs (pending review for too long)
// and sends notifications when PRs exceed the stale threshold.
type GitHubConfig struct {
	// Interval is an optional per-task override for the scheduler interval.
	// If set, this task runs at this interval instead of the global scheduler interval.
	// Format: "60m", "1h", etc. Leave empty to use the global default.
	Interval string `mapstructure:"interval"`

	// Token is an optional GitHub personal access token for higher API rate limits.
	// Without a token, you're limited to 60 requests/hour. With a token: 5000 requests/hour.
	Token string `mapstructure:"token"`

	// Repositories is the list of GitHub repos to monitor for stale PRs.
	Repositories []RepositoryConfig `mapstructure:"repositories"`

	// StaleDays defines how many days a PR can be pending before it's considered stale.
	// Default is 4 days if not specified.
	StaleDays int `mapstructure:"stale_days"`

	// NotificationCooldown prevents spam by limiting how often we notify about the same PR.
	// Format: "24h", "2h30m", etc. Default is 24 hours.
	NotificationCooldown string `mapstructure:"notification_cooldown"`
}

// RepositoryConfig defines a specific GitHub repository to monitor.
// You can optionally filter PRs by author to only track specific team members.
type RepositoryConfig struct {
	// Owner is the GitHub username or organization name (e.g., "signoz")
	Owner string `mapstructure:"owner"`

	// Repo is the repository name (e.g., "signoz-web")
	Repo string `mapstructure:"repo"`

	// Authors is an optional list of GitHub usernames to filter PRs.
	// If empty, all PRs in the repo are monitored. If specified, only PRs by these authors are checked.
	Authors []string `mapstructure:"authors"`
}

// GetNotificationCooldown parses the cooldown string into a time.Duration.
// Returns 24 hours if the value is empty or invalid.
// This prevents sending duplicate notifications for the same PR too frequently.
func (g GitHubConfig) GetNotificationCooldown() time.Duration {
	return parseDurationWithDefault(g.NotificationCooldown, 24*time.Hour, "tasks.github.notification_cooldown")
}

// GetStaleDays returns the number of days before a PR is considered stale.
// Returns 4 days if not configured or set to 0.
// A PR is stale if it hasn't been updated in this many days.
func (g GitHubConfig) GetStaleDays() int {
	if g.StaleDays <= 0 {
		return 4
	}
	return g.StaleDays
}

// GetInterval returns the task-specific interval if configured, otherwise the global default.
// This allows GitHub checks to run less frequently than other tasks (e.g., every 60m to respect rate limits).
func (g GitHubConfig) GetInterval(globalDefault time.Duration) time.Duration {
	return parseDurationWithDefault(g.Interval, globalDefault, "tasks.github.interval")
}

// TelnyxConfig holds settings for monitoring your Telnyx account balance.
// The watchdog will periodically check your balance and alert if it drops below the threshold.
type TelnyxConfig struct {
	// Interval is an optional per-task override for the scheduler interval.
	// If set, this task runs at this interval instead of the global scheduler interval.
	// Format: "5m", "1h", etc. Leave empty to use the global default.
	Interval string `mapstructure:"interval"`

	// APIURL is the Telnyx API endpoint for balance checks (usually https://api.telnyx.com/v2/balance)
	APIURL string `mapstructure:"api_url"`

	// APIKey is your Telnyx API key for authentication (starts with "KEY...")
	APIKey string `mapstructure:"api_key"`

	// Threshold is the minimum balance in dollars. Alerts are sent when balance < threshold.
	Threshold float64 `mapstructure:"threshold"`

	// NotificationCooldown prevents spam by limiting alert frequency for low balance.
	// Format: "6h", "1h30m", etc. Default is 6 hours.
	NotificationCooldown string `mapstructure:"notification_cooldown"`
}

// GetInterval returns the task-specific interval if configured, otherwise the global default.
func (t TelnyxConfig) GetInterval(globalDefault time.Duration) time.Duration {
	return parseDurationWithDefault(t.Interval, globalDefault, "tasks.telnyx.interval")
}

// GetNotificationCooldown parses the cooldown string into a time.Duration.
// Returns 6 hours if the value is empty or invalid.
// This prevents repeatedly sending "low balance" alerts every check interval.
func (t TelnyxConfig) GetNotificationCooldown() time.Duration {
	return parseDurationWithDefault(t.NotificationCooldown, 6*time.Hour, "tasks.telnyx.notification_cooldown")
}

// NotifierConfig holds settings for the Apprise notification system.
// Apprise is a universal notification library that supports 70+ services
// (Telegram, Discord, Slack, email, SMS, etc.)
type NotifierConfig struct {
	// AppriseAPIURL is the endpoint of your Apprise API server.
	// This is where notification requests are sent.
	AppriseAPIURL string `mapstructure:"apprise_api_url"`

	// AppriseServiceURL contains one or more notification service URLs separated by commas.
	// Examples:
	//   - Telegram: "tgram://botToken/chatID"
	//   - Discord: "discord://webhook_id/webhook_token"
	//   - Email: "mailto://user:pass@gmail.com"
	// Multiple services: "tgram://...,discord://...,mailto://..."
	AppriseServiceURL string `mapstructure:"apprise_service_url"`
}

// GetServiceURLs splits the comma-separated service URL string into individual URLs.
// Each URL represents a different notification destination (Telegram, Discord, etc.)
// Returns an empty slice if no services are configured.
// Empty strings (from consecutive commas or trailing commas) are filtered out.
func (n NotifierConfig) GetServiceURLs() []string {
	if n.AppriseServiceURL == "" {
		return []string{}
	}
	parts := strings.Split(n.AppriseServiceURL, ",")
	var urls []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			urls = append(urls, trimmed)
		}
	}
	return urls
}

// SchedulerConfig controls the default frequency for monitoring tasks.
// Tasks can override this interval individually.
type SchedulerConfig struct {
	// Interval defines how often to run checks.
	// Format: "5m" (5 minutes), "1h" (1 hour), "30s" (30 seconds), etc.
	// Default is 5 minutes if not specified or invalid.
	Interval string `mapstructure:"interval"`
}

// GetInterval parses the interval string into a time.Duration.
// Returns 5 minutes if the value is empty or invalid.
// This determines how frequently all monitoring tasks are executed.
func (s SchedulerConfig) GetInterval() time.Duration {
	return parseDurationWithDefault(s.Interval, 5*time.Minute, "scheduler.interval")
}
