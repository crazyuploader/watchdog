package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PullRequest represents a GitHub pull request with the fields we care about for monitoring.
// This struct is populated by unmarshaling the JSON response from GitHub's API.
// We only include fields relevant to determining if a PR is stale (needs review attention).
type PullRequest struct {
	// Number is the PR number (e.g., #123)
	Number int `json:"number"`

	// Title is the PR title/description
	Title string `json:"title"`

	// User contains information about who created the PR
	User User `json:"user"`

	// CreatedAt is when the PR was first opened
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last time the PR was modified (new commits, comments, reviews, etc.)
	// We use this to determine if a PR is stale
	UpdatedAt time.Time `json:"updated_at"`

	// Draft indicates if this is a draft PR (not ready for review)
	// We skip draft PRs in our monitoring
	Draft bool `json:"draft"`

	// HTMLURL is the web URL to view the PR (e.g., https://github.com/owner/repo/pull/123)
	// We include this in notifications so users can click through
	HTMLURL string `json:"html_url"`
}

// User represents the GitHub user who created a pull request.
// We only need the login (username) for filtering PRs by author.
type User struct {
	// Login is the GitHub username (e.g., "crazyuploader")
	Login string `json:"login"`
}

// GitHubAPI is a client for interacting with the GitHub REST API.
// It handles authentication via personal access tokens and provides methods
// for fetching pull request data.
type GitHubAPI struct {
	// BaseURL is the GitHub API base URL (https://api.github.com)
	BaseURL string

	// Token is an optional personal access token for authentication.
	// Without a token: 60 requests/hour rate limit
	// With a token: 5000 requests/hour rate limit
	// Leave empty for public repos if you don't need high rate limits
	Token string
}

// NewGitHubAPI creates a new GitHub API client.
// The token parameter is optional - pass an empty string if you don't have one.
// NewGitHubAPI creates a GitHubAPI client with BaseURL set to "https://api.github.com" and the provided personal access token.
// If token is empty the client will make unauthenticated requests.
func NewGitHubAPI(token string) *GitHubAPI {
	return &GitHubAPI{
		BaseURL: "https://api.github.com",
		Token:   token,
	}
}

// GetOpenPullRequests fetches all open pull requests for a specific repository.
// It returns up to 100 PRs (GitHub's max per_page limit).
//
// Parameters:
//   - owner: The GitHub username or organization (e.g., "signoz")
//   - repo: The repository name (e.g., "signoz-web")
//
// Returns:
//   - A slice of PullRequest objects containing PR metadata
//   - An error if the API request fails or returns a non-200 status
//
// The function automatically adds authentication headers if a token is configured.
func (g *GitHubAPI) GetOpenPullRequests(owner, repo string) ([]PullRequest, error) {
	// Create HTTP client with a 10-second timeout to prevent hanging
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Build the API URL - we request open PRs with a limit of 100 per page
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", g.BaseURL, owner, repo)

	// Create the HTTP request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set required headers
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("User-Agent", "watchdog-app") // GitHub requires a User-Agent header

	// Add authentication if we have a token
	// This increases rate limits from 60/hour to 5000/hour
	if g.Token != "" {
		req.Header.Add("Authorization", "token "+g.Token)
	}

	// Execute the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull requests: %v", err)
	}

	// Ensure the response body is closed when we're done
	// We explicitly ignore the error since there's nothing we can do about it
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	// Parse the JSON response into our PullRequest struct
	var prs []PullRequest
	err = json.Unmarshal(body, &prs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return prs, nil
}
