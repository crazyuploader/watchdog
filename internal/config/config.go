package config

import (
	"strings"
	"time"
)

type Config struct {
	Telnyx    TelnyxConfig    `mapstructure:"telnyx"`
	Notifier  NotifierConfig  `mapstructure:"notifier"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
}

type TelnyxConfig struct {
	APIURL    string  `mapstructure:"api_url"`
	APIKey    string  `mapstructure:"api_key"`
	Threshold float64 `mapstructure:"threshold"`
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
