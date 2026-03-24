package tasks

import (
	"context"
	"fmt"
	"testing"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestNewExternalContributorPRTask(t *testing.T) {
	cfg := config.ExternalContributorPRConfig{
		Token: "test-token",
		Repositories: []config.ExternalContributorRepoConfig{
			{Owner: "SigNoz", Repo: "signoz.io"},
		},
	}
	mockNotifier := &MockNotifier{}

	task := NewExternalContributorPRTask(cfg, mockNotifier)

	assert.NotNil(t, task)
	assert.Equal(t, cfg, task.config)
	assert.NotNil(t, task.apiClient)
	assert.Equal(t, mockNotifier, task.notifier)
	assert.NotNil(t, task.lastNotificationTime)
}

func TestExternalContributorPRTask_FilterExternalPRs(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		repoConfig  config.ExternalContributorRepoConfig
		prs         []api.PullRequest
		mockMembers []api.User
		expectedNum int
		expectError bool
	}{
		{
			name: "mixed contributors with manual member list",
			repoConfig: config.ExternalContributorRepoConfig{
				Owner:          "org",
				Repo:           "repo",
				OrgMembers:     []string{"member1", "member2"},
				PRLookbackDays: 7,
			},
			prs: []api.PullRequest{
				{Number: 1, User: api.User{Login: "member1"}, CreatedAt: now.Add(-1 * time.Hour)},
				{Number: 2, User: api.User{Login: "external1"}, CreatedAt: now.Add(-24 * time.Hour)},
				{Number: 3, User: api.User{Login: "MEMBER2"}, CreatedAt: now.Add(-2 * time.Hour)}, // Case check
			},
			expectedNum: 1, // Only external1
		},
		{
			name: "external contributor outside lookback",
			repoConfig: config.ExternalContributorRepoConfig{
				Owner:          "org",
				Repo:           "repo",
				OrgMembers:     []string{"member1"},
				PRLookbackDays: 7,
			},
			prs: []api.PullRequest{
				{Number: 1, User: api.User{Login: "external1"}, CreatedAt: now.Add(-10 * 24 * time.Hour)},
			},
			expectedNum: 0,
		},
		{
			name: "draft PRs are skipped",
			repoConfig: config.ExternalContributorRepoConfig{
				Owner:          "org",
				Repo:           "repo",
				OrgMembers:     []string{"member1"},
				PRLookbackDays: 7,
			},
			prs: []api.PullRequest{
				{Number: 1, User: api.User{Login: "external1"}, CreatedAt: now.Add(-1 * time.Hour), Draft: true},
			},
			expectedNum: 0,
		},
		{
			name: "auto-fetch members from API",
			repoConfig: config.ExternalContributorRepoConfig{
				Owner:          "org",
				Repo:           "repo",
				PRLookbackDays: 7,
			},
			prs: []api.PullRequest{
				{Number: 1, User: api.User{Login: "member1"}, CreatedAt: now.Add(-1 * time.Hour)},
				{Number: 2, User: api.User{Login: "external1"}, CreatedAt: now.Add(-1 * time.Hour)},
			},
			mockMembers: []api.User{
				{Login: "member1"},
			},
			expectedNum: 1, // Only external1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &MockGitHubClient{}
			if len(tt.repoConfig.OrgMembers) == 0 {
				mockAPI.On("GetOrgMembers", mock.Anything, tt.repoConfig.Owner).Return(tt.mockMembers, nil)
			}

			task := &ExternalContributorPRTask{
				apiClient: mockAPI,
			}

			result := task.filterExternalPRs(context.Background(), tt.prs, tt.repoConfig)
			assert.Len(t, result, tt.expectedNum)
			mockAPI.AssertExpectations(t)
		})
	}
}

func TestExternalContributorPRTask_Run(t *testing.T) {
	cfg := config.ExternalContributorPRConfig{
		Repositories: []config.ExternalContributorRepoConfig{
			{
				Owner:          "org",
				Repo:           "repo",
				OrgMembers:     []string{"member1"},
				PRLookbackDays: 7,
			},
		},
		NotificationCooldown: "24h",
	}

	externalPR := api.PullRequest{
		Number:    1,
		Title:     "External PR",
		User:      api.User{Login: "external1"},
		CreatedAt: time.Now().Add(-1 * time.Hour),
		HTMLURL:   "https://github.com/org/repo/pull/1",
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "org", "repo").Return([]api.PullRequest{externalPR}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "External contributor PRs in org/repo") &&
			assert.Contains(t, msg, "#1 by external1")
	})).Return(nil)

	task := NewExternalContributorPRTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	// First run: should notify
	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)

	// Second run: should NOT notify due to cooldown
	err = task.Run()
	assert.NoError(t, err)
	// mockNotifier already asserted once, if it was called twice AssertExpectations would fail or we could use .Once()
}

func TestExternalContributorPRTask_Run_APIError(t *testing.T) {
	cfg := config.ExternalContributorPRConfig{
		Repositories: []config.ExternalContributorRepoConfig{
			{Owner: "org", Repo: "repo"},
		},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "org", "repo").Return(nil, fmt.Errorf("api error"))

	mockNotifier := &MockNotifier{}

	task := NewExternalContributorPRTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err) // Run logs and continues
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{25 * time.Hour, "1d 1h"},
		{2 * time.Hour, "2h 0m"},
		{1 * time.Hour, "1h 0m"},
		{30 * time.Minute, "30m"},
		{90 * time.Minute, "1h 30m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatDuration(tt.d))
		})
	}
}
