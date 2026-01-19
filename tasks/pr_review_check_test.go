package tasks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockGitHubClient mocks the GitHub API client interface
type MockGitHubClient struct {
	mock.Mock
}

func (m *MockGitHubClient) GetOpenPullRequests(ctx context.Context, owner, repo string) ([]api.PullRequest, error) {
	args := m.Called(ctx, owner, repo)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]api.PullRequest), args.Error(1)
}

func (m *MockGitHubClient) GetCommitStatus(ctx context.Context, owner, repo, ref string) (*api.CommitStatus, error) {
	args := m.Called(ctx, owner, repo, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.CommitStatus), args.Error(1)
}

func (m *MockGitHubClient) GetCheckSuites(ctx context.Context, owner, repo, ref string) (*api.CheckSuitesResponse, error) {
	args := m.Called(ctx, owner, repo, ref)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*api.CheckSuitesResponse), args.Error(1)
}

func TestNewPRReviewCheckTask(t *testing.T) {
	cfg := config.GitHubConfig{
		Token:     "ghp_test",
		StaleDays: 5,
		Repositories: []config.RepositoryConfig{
			{Owner: "owner1", Repo: "repo1"},
		},
	}
	notifier := &MockNotifier{}

	task := NewPRReviewCheckTask(cfg, notifier)

	assert.NotNil(t, task)
	assert.Equal(t, cfg, task.config)
	assert.NotNil(t, task.apiClient)
	assert.NotNil(t, task.notifier)
	assert.NotNil(t, task.lastNotificationTime)
	assert.Empty(t, task.lastNotificationTime)
}

func TestPRReviewCheckTask_Run_NoRepositories(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays:    4,
		Repositories: []config.RepositoryConfig{},
	}

	task := NewPRReviewCheckTask(cfg, &MockNotifier{})

	err := task.Run()

	assert.NoError(t, err)
}

func TestPRReviewCheckTask_Run_NoPullRequests(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "owner1", Repo: "repo1"},
		},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "owner1", "repo1").Return([]api.PullRequest{}, nil)

	task := NewPRReviewCheckTask(cfg, &MockNotifier{})
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_SendsNotification(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays:            4,
		NotificationCooldown: "24h",
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Stale PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour), // 5 days old
		Draft:     false,
		HTMLURL:   "https://github.com/testowner/testrepo/pull/123",
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "Stale PR: Stale PR", mock.MatchedBy(func(msg string) bool {
		// Should NOT contain "Waiting on" or CI status (since passing)
		return assert.Contains(t, msg, "#123") &&
			assert.Contains(t, msg, "testowner/testrepo") &&
			assert.Contains(t, msg, "testuser") &&
			!strings.Contains(msg, "Waiting on:") &&
			!strings.Contains(msg, "CI:")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_WithRequestedReviewers(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Stale PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		RequestedReviewers: []api.User{
			{Login: "alice"},
			{Login: "bob"},
		},
		Draft:   false,
		HTMLURL: "http://github.com/pr/123",
		Head:    api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "Stale PR: Stale PR", mock.MatchedBy(func(msg string) bool {
		// Reviewer "Waiting on" list should be REMOVED
		return !strings.Contains(msg, "Waiting on: alice, bob")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_NoRequestedReviewers(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:             123,
		Title:              "Stale PR",
		User:               api.User{Login: "testuser"},
		UpdatedAt:          time.Now().Add(-5 * 24 * time.Hour),
		RequestedReviewers: []api.User{},
		Draft:              false,
		HTMLURL:            "http://github.com/pr/123",
		Head:               api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "Stale PR: Stale PR", mock.MatchedBy(func(msg string) bool {
		// "No specific reviewers" message should be REMOVED
		return !strings.Contains(msg, "No specific reviewers requested")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_FreshPR_NoNotification(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	freshPR := api.PullRequest{
		Number:    123,
		Title:     "Fresh PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-2 * 24 * time.Hour), // 2 days old
		Draft:     false,
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{freshPR}, nil)
	// No status checks needed for fresh PRs

	mockNotifier := &MockNotifier{}

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestPRReviewCheckTask_Run_DraftPR_Skipped(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	draftPR := api.PullRequest{
		Number:    123,
		Title:     "Draft PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-10 * 24 * time.Hour), // Very old
		Draft:     true,
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{draftPR}, nil)
	// No GetCommitStatus explanation needed because draft PRs are skipped before that check

	mockNotifier := &MockNotifier{}

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestPRReviewCheckTask_Run_AuthorFilter_Matches(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{
				Owner:   "testowner",
				Repo:    "testrepo",
				Authors: []string{"author1", "author2"},
			},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "PR by author1",
		User:      api.User{Login: "author1"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_AuthorFilter_NoMatch(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{
				Owner:   "testowner",
				Repo:    "testrepo",
				Authors: []string{"author1", "author2"},
			},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "PR by other author",
		User:      api.User{Login: "otherauthor"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)

	mockNotifier := &MockNotifier{}

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}

func TestPRReviewCheckTask_Run_AuthorFilter_CaseInsensitive(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{
				Owner:   "testowner",
				Repo:    "testrepo",
				Authors: []string{"Author1"},
			},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "PR",
		User:      api.User{Login: "author1"}, // Different case
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_CIFailure(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Failing PR",
		User:      api.User{Login: "dev"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Head:      api.PRHead{SHA: "badsha"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "badsha").Return(&api.CommitStatus{State: "failure"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "badsha").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "CI: Failing ❌")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_CIPending(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Pending PR",
		User:      api.User{Login: "dev"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Head:      api.PRHead{SHA: "pendingsha"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "pendingsha").Return(&api.CommitStatus{State: "pending"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "pendingsha").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.MatchedBy(func(msg string) bool {
		// Pending is no longer reported, should be clean
		return !strings.Contains(msg, "CI: Pending")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_StalePR_CheckSuiteFailure(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Actions Failing PR",
		User:      api.User{Login: "dev"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Head:      api.PRHead{SHA: "actionfail"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	// Commit status says success (e.g. legacy), but Check Action failed
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "actionfail").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "actionfail").Return(&api.CheckSuitesResponse{
		CheckSuites: []api.CheckSuite{
			{Conclusion: "failure", Status: "completed"},
		},
	}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.MatchedBy(func(msg string) bool {
		return assert.Contains(t, msg, "CI: Failing ❌")
	})).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()
	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_NoAuthorFilter_AllPRsMonitored(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{
				Owner:   "testowner",
				Repo:    "testrepo",
				Authors: []string{}, // Empty = monitor all
			},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "PR by anyone",
		User:      api.User{Login: "anyone"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_RespectsCooldown(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays:            4,
		NotificationCooldown: "1h",
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR := api.PullRequest{
		Number:    123,
		Title:     "Stale PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil).Once()
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil).Once()

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	// First run - should notify
	err := task.Run()
	require.NoError(t, err)

	// Immediate second run - should not notify due to cooldown
	err = task.Run()
	require.NoError(t, err)

	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_APIError_ContinuesWithOtherRepos(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "owner1", Repo: "repo1"},
			{Owner: "owner2", Repo: "repo2"},
		},
	}

	stalePR := api.PullRequest{
		Number:    456,
		Title:     "Stale PR",
		User:      api.User{Login: "testuser"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha456"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "owner1", "repo1").Return(nil, errors.New("API error"))
	mockAPI.On("GetOpenPullRequests", mock.Anything, "owner2", "repo2").Return([]api.PullRequest{stalePR}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "owner2", "repo2", "sha456").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "owner2", "repo2", "sha456").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	// Should not return error, just log and continue
	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_NotificationError_ContinuesWithOtherPRs(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	stalePR1 := api.PullRequest{
		Number:    123,
		Title:     "PR 1",
		User:      api.User{Login: "user1"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	stalePR2 := api.PullRequest{
		Number:    456,
		Title:     "PR 2",
		User:      api.User{Login: "user2"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha456"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{stalePR1, stalePR2}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "testowner", "testrepo", "sha456").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "testowner", "testrepo", "sha456").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, "Stale PR: PR 1", mock.Anything).Return(errors.New("notification failed"))
	mockNotifier.On("SendNotification", mock.Anything, "Stale PR: PR 2", mock.Anything).Return(nil)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_MultipleRepositories(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "owner1", Repo: "repo1"},
			{Owner: "owner2", Repo: "repo2"},
		},
	}

	stalePR1 := api.PullRequest{
		Number:    123,
		Title:     "PR in repo1",
		User:      api.User{Login: "user1"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha123"},
	}

	stalePR2 := api.PullRequest{
		Number:    456,
		Title:     "PR in repo2",
		User:      api.User{Login: "user2"},
		UpdatedAt: time.Now().Add(-5 * 24 * time.Hour),
		Draft:     false,
		Head:      api.PRHead{SHA: "sha456"},
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "owner1", "repo1").Return([]api.PullRequest{stalePR1}, nil)
	mockAPI.On("GetOpenPullRequests", mock.Anything, "owner2", "repo2").Return([]api.PullRequest{stalePR2}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "owner1", "repo1", "sha123").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "owner1", "repo1", "sha123").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)
	mockAPI.On("GetCommitStatus", mock.Anything, "owner2", "repo2", "sha456").Return(&api.CommitStatus{State: "success"}, nil)
	mockAPI.On("GetCheckSuites", mock.Anything, "owner2", "repo2", "sha456").Return(&api.CheckSuitesResponse{TotalCount: 0}, nil)

	mockNotifier := &MockNotifier{}
	mockNotifier.On("SendNotification", mock.Anything, mock.Anything, mock.Anything).Return(nil).Times(2)

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	mockAPI.AssertExpectations(t)
	mockNotifier.AssertExpectations(t)
}

func TestPRReviewCheckTask_Run_CleanupOldNotifications(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays:            4,
		NotificationCooldown: "24h",
		Repositories:         []config.RepositoryConfig{},
	}

	task := NewPRReviewCheckTask(cfg, &MockNotifier{})

	// Add old notification entries
	task.lastNotificationTime["owner/repo#123"] = time.Now().Add(-10 * 24 * time.Hour)
	task.lastNotificationTime["owner/repo#456"] = time.Now().Add(-1 * time.Hour) // Recent

	require.Len(t, task.lastNotificationTime, 2)

	err := task.Run()

	assert.NoError(t, err)
	// Old entry should be cleaned up
	assert.NotContains(t, task.lastNotificationTime, "owner/repo#123")
	// Recent entry should remain
	assert.Contains(t, task.lastNotificationTime, "owner/repo#456")
}

func TestPRReviewCheckTask_Run_ExactlyAtStaleThreshold(t *testing.T) {
	cfg := config.GitHubConfig{
		StaleDays: 4,
		Repositories: []config.RepositoryConfig{
			{Owner: "testowner", Repo: "testrepo"},
		},
	}

	// PR updated exactly 4 days ago
	pr := api.PullRequest{
		Number: 123,
		Title:  "PR at threshold",
		User:   api.User{Login: "testuser"},
		// Use 1 hour buffer to ensure it's definitely less than 4 days
		UpdatedAt: time.Now().Add(-4 * 24 * time.Hour).Add(1 * time.Hour),
		Draft:     false,
	}

	mockAPI := &MockGitHubClient{}
	mockAPI.On("GetOpenPullRequests", mock.Anything, "testowner", "testrepo").Return([]api.PullRequest{pr}, nil)

	mockNotifier := &MockNotifier{}

	task := NewPRReviewCheckTask(cfg, mockNotifier)
	task.apiClient = mockAPI

	err := task.Run()

	assert.NoError(t, err)
	// At exactly 4 days, should not trigger (needs to be > 4 days)
	mockNotifier.AssertNotCalled(t, "SendNotification", mock.Anything, mock.Anything, mock.Anything)
}
