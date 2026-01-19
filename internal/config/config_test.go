package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseDurationWithDefault(t *testing.T) {
	tests := []struct {
		name            string
		value           string
		defaultDuration time.Duration
		expected        time.Duration
	}{
		{
			name:            "valid duration - minutes",
			value:           "5m",
			defaultDuration: 10 * time.Minute,
			expected:        5 * time.Minute,
		},
		{
			name:            "valid duration - hours",
			value:           "2h",
			defaultDuration: 1 * time.Hour,
			expected:        2 * time.Hour,
		},
		{
			name:            "valid duration - seconds",
			value:           "30s",
			defaultDuration: 60 * time.Second,
			expected:        30 * time.Second,
		},
		{
			name:            "valid duration - complex",
			value:           "1h30m45s",
			defaultDuration: 1 * time.Hour,
			expected:        1*time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:            "empty string",
			value:           "",
			defaultDuration: 5 * time.Minute,
			expected:        5 * time.Minute,
		},
		{
			name:            "whitespace only",
			value:           "   ",
			defaultDuration: 10 * time.Minute,
			expected:        10 * time.Minute,
		},
		{
			name:            "invalid duration",
			value:           "invalid",
			defaultDuration: 15 * time.Minute,
			expected:        15 * time.Minute,
		},
		{
			name:            "negative duration",
			value:           "-5m",
			defaultDuration: 5 * time.Minute,
			expected:        -5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDurationWithDefault(tt.value, tt.defaultDuration, "test.config")
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubConfig_GetNotificationCooldown(t *testing.T) {
	tests := []struct {
		name     string
		cooldown string
		expected time.Duration
	}{
		{
			name:     "valid cooldown",
			cooldown: "12h",
			expected: 12 * time.Hour,
		},
		{
			name:     "empty cooldown - use default",
			cooldown: "",
			expected: 24 * time.Hour,
		},
		{
			name:     "invalid cooldown - use default",
			cooldown: "invalid",
			expected: 24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GitHubConfig{
				NotificationCooldown: tt.cooldown,
			}
			result := cfg.GetNotificationCooldown()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubConfig_GetStaleDays(t *testing.T) {
	tests := []struct {
		name      string
		staleDays int
		expected  int
	}{
		{
			name:      "configured stale days",
			staleDays: 7,
			expected:  7,
		},
		{
			name:      "zero stale days - use default",
			staleDays: 0,
			expected:  4,
		},
		{
			name:      "negative stale days - use default",
			staleDays: -5,
			expected:  4,
		},
		{
			name:      "large stale days",
			staleDays: 365,
			expected:  365,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GitHubConfig{
				StaleDays: tt.staleDays,
			}
			result := cfg.GetStaleDays()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGitHubConfig_GetInterval(t *testing.T) {
	tests := []struct {
		name          string
		interval      string
		globalDefault time.Duration
		expected      time.Duration
	}{
		{
			name:          "task-specific interval",
			interval:      "60m",
			globalDefault: 5 * time.Minute,
			expected:      60 * time.Minute,
		},
		{
			name:          "empty interval - use global default",
			interval:      "",
			globalDefault: 10 * time.Minute,
			expected:      10 * time.Minute,
		},
		{
			name:          "invalid interval - use global default",
			interval:      "invalid",
			globalDefault: 15 * time.Minute,
			expected:      15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := GitHubConfig{
				Interval: tt.interval,
			}
			result := cfg.GetInterval(tt.globalDefault)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTelnyxConfig_GetInterval(t *testing.T) {
	tests := []struct {
		name          string
		interval      string
		globalDefault time.Duration
		expected      time.Duration
	}{
		{
			name:          "task-specific interval",
			interval:      "10m",
			globalDefault: 5 * time.Minute,
			expected:      10 * time.Minute,
		},
		{
			name:          "empty interval - use global default",
			interval:      "",
			globalDefault: 7 * time.Minute,
			expected:      7 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TelnyxConfig{
				Interval: tt.interval,
			}
			result := cfg.GetInterval(tt.globalDefault)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTelnyxConfig_GetNotificationCooldown(t *testing.T) {
	tests := []struct {
		name     string
		cooldown string
		expected time.Duration
	}{
		{
			name:     "valid cooldown",
			cooldown: "12h",
			expected: 12 * time.Hour,
		},
		{
			name:     "empty cooldown - use default",
			cooldown: "",
			expected: 6 * time.Hour,
		},
		{
			name:     "invalid cooldown - use default",
			cooldown: "bad-value",
			expected: 6 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := TelnyxConfig{
				NotificationCooldown: tt.cooldown,
			}
			result := cfg.GetNotificationCooldown()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNotifierConfig_GetServiceURLs(t *testing.T) {
	tests := []struct {
		name       string
		serviceURL string
		expected   []string
	}{
		{
			name:       "single service URL",
			serviceURL: "tgram://botToken/chatID",
			expected:   []string{"tgram://botToken/chatID"},
		},
		{
			name:       "multiple service URLs",
			serviceURL: "tgram://token/id,discord://webhook/token,mailto://user:pass@gmail.com",
			expected:   []string{"tgram://token/id", "discord://webhook/token", "mailto://user:pass@gmail.com"},
		},
		{
			name:       "URLs with spaces",
			serviceURL: "tgram://token/id , discord://webhook/token , mailto://user@mail.com",
			expected:   []string{"tgram://token/id", "discord://webhook/token", "mailto://user@mail.com"},
		},
		{
			name:       "empty string",
			serviceURL: "",
			expected:   []string{},
		},
		{
			name:       "trailing comma",
			serviceURL: "tgram://token/id,",
			expected:   []string{"tgram://token/id"},
		},
		{
			name:       "leading comma",
			serviceURL: ",tgram://token/id",
			expected:   []string{"tgram://token/id"},
		},
		{
			name:       "consecutive commas",
			serviceURL: "tgram://token/id,,discord://webhook/token",
			expected:   []string{"tgram://token/id", "discord://webhook/token"},
		},
		{
			name:       "only commas",
			serviceURL: ",,,",
			expected:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NotifierConfig{
				AppriseServiceURL: tt.serviceURL,
			}
			result := cfg.GetServiceURLs()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSchedulerConfig_GetInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		expected time.Duration
	}{
		{
			name:     "valid interval - minutes",
			interval: "10m",
			expected: 10 * time.Minute,
		},
		{
			name:     "valid interval - hours",
			interval: "2h",
			expected: 2 * time.Hour,
		},
		{
			name:     "valid interval - seconds",
			interval: "30s",
			expected: 30 * time.Second,
		},
		{
			name:     "empty interval - use default",
			interval: "",
			expected: 5 * time.Minute,
		},
		{
			name:     "invalid interval - use default",
			interval: "not-a-duration",
			expected: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SchedulerConfig{
				Interval: tt.interval,
			}
			result := cfg.GetInterval()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRepositoryConfig_Fields(t *testing.T) {
	repo := RepositoryConfig{
		Owner:   "testowner",
		Repo:    "testrepo",
		Authors: []string{"author1", "author2"},
	}

	assert.Equal(t, "testowner", repo.Owner)
	assert.Equal(t, "testrepo", repo.Repo)
	assert.Len(t, repo.Authors, 2)
	assert.Contains(t, repo.Authors, "author1")
	assert.Contains(t, repo.Authors, "author2")
}

func TestConfig_Structure(t *testing.T) {
	cfg := Config{
		Tasks: TasksConfig{
			Telnyx: TelnyxConfig{
				APIURL:    "https://api.telnyx.com/v2/balance",
				APIKey:    "KEY123",
				Threshold: 10.0,
			},
			GitHub: GitHubConfig{
				Token:     "ghp_token",
				StaleDays: 5,
				Repositories: []RepositoryConfig{
					{Owner: "owner1", Repo: "repo1"},
				},
			},
		},
		Notifier: NotifierConfig{
			AppriseAPIURL:     "https://apprise.example.com/notify",
			AppriseServiceURL: "tgram://token/id",
		},
		Scheduler: SchedulerConfig{
			Interval: "5m",
		},
	}

	assert.Equal(t, "https://api.telnyx.com/v2/balance", cfg.Tasks.Telnyx.APIURL)
	assert.Equal(t, "KEY123", cfg.Tasks.Telnyx.APIKey)
	assert.Equal(t, 10.0, cfg.Tasks.Telnyx.Threshold)
	assert.Equal(t, "ghp_token", cfg.Tasks.GitHub.Token)
	assert.Equal(t, 5, cfg.Tasks.GitHub.StaleDays)
	assert.Len(t, cfg.Tasks.GitHub.Repositories, 1)
	assert.Equal(t, "https://apprise.example.com/notify", cfg.Notifier.AppriseAPIURL)
	assert.Equal(t, "5m", cfg.Scheduler.Interval)
}
