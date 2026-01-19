package api

import "context"

// GitHubClient defines the interface for GitHub API operations.
// This allows for easy mocking in tests.
type GitHubClient interface {
	GetOpenPullRequests(ctx context.Context, owner, repo string) ([]PullRequest, error)
	GetCommitStatus(ctx context.Context, owner, repo, ref string) (*CommitStatus, error)
	GetCheckSuites(ctx context.Context, owner, repo, ref string) (*CheckSuitesResponse, error)
}

// Ensure GitHubAPI implements GitHubClient interface
var _ GitHubClient = (*GitHubAPI)(nil)
