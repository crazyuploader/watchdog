package config

import (
	"strings"
	"time"
)

type Config struct {
	Telnyx    TelnyxConfig    `mapstructure:"telnyx"`
	Notifier  NotifierConfig  `mapstructure:"notifier"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	GitHub    GitHubConfig    `mapstructure:"github"`
}

// GitHubConfig holds configuration for GitHub PR monitoring.
type GitHubConfig struct {
	Token                string             `mapstructure:"token"`
	Repositories         []RepositoryConfig `mapstructure:"repositories"`
	StaleDays            int                `mapstructure:"stale_days"`            // Default: 4
	NotificationCooldown string             `mapstructure:"notification_cooldown"` // Default: 24h
}

// RepositoryConfig defines a specific repository to monitor.
type RepositoryConfig struct {
	Owner   string   `mapstructure:"owner"`
	Repo    string   `mapstructure:"repo"`
	Authors []string `mapstructure:"authors"`
}

// GetNotificationCooldown returns the configured cooldown duration or a default of 24 hours.
func (g GitHubConfig) GetNotificationCooldown() time.Duration {
	if g.NotificationCooldown == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(g.NotificationCooldown)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// GetStaleDays returns the configured stale days threshold or a default of 4.
func (g GitHubConfig) GetStaleDays() int {
	if g.StaleDays == 0 {
		return 4
	}
	return g.StaleDays
}

type TelnyxConfig struct {
	APIURL               string  `mapstructure:"api_url"`
	APIKey               string  `mapstructure:"api_key"`
	Threshold            float64 `mapstructure:"threshold"`
	NotificationCooldown string  `mapstructure:"notification_cooldown"`
}

func (t TelnyxConfig) GetNotificationCooldown() time.Duration {
	if t.NotificationCooldown == "" {
		return 6 * time.Hour // default
	}
	d, err := time.ParseDuration(t.NotificationCooldown)
	if err != nil {
		return 6 * time.Hour // default
	}
	return d
}

type NotifierConfig struct {
	AppriseAPIURL     string `mapstructure:"apprise_api_url"`
	AppriseServiceURL string `mapstructure:"apprise_service_url"`
}

func (n NotifierConfig) GetServiceURLs() []string {
	if n.AppriseServiceURL == "" {
		return []string{}
	}
	parts := strings.Split(n.AppriseServiceURL, ",")
	var urls []string
	for _, p := range parts {
		urls = append(urls, strings.TrimSpace(p))
	}
	return urls
}

type SchedulerConfig struct {
	Interval string `mapstructure:"interval"` // parsed as duration
}

func (s SchedulerConfig) GetInterval() time.Duration {
	d, err := time.ParseDuration(s.Interval)
	if err != nil {
		return 5 * time.Minute // default
	}
	return d
}
