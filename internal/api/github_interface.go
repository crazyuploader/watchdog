package api

// GitHubClient defines the interface for GitHub API operations.
// This allows for easy mocking in tests.
type GitHubClient interface {
	GetOpenPullRequests(owner, repo string) ([]PullRequest, error)
}

// Ensure GitHubAPI implements GitHubClient interface
var _ GitHubClient = (*GitHubAPI)(nil)