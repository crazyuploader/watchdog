package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Draft     bool      `json:"draft"`
	HTMLURL   string    `json:"html_url"`
}

type User struct {
	Login string `json:"login"`
}

// GitHubAPI provides methods to interact with the GitHub API.
type GitHubAPI struct {
	BaseURL string
	Token   string
}

// NewGitHubAPI creates a new GitHubAPI client with the provided token.
func NewGitHubAPI(token string) *GitHubAPI {
	return &GitHubAPI{
		BaseURL: "https://api.github.com",
		Token:   token,
	}
}

// GetOpenPullRequests fetches all open pull requests for the specified repository.
func (g *GitHubAPI) GetOpenPullRequests(owner, repo string) ([]PullRequest, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", g.BaseURL, owner, repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Add("Accept", "application/vnd.github.v3+json")
	if g.Token != "" {
		req.Header.Add("Authorization", "token "+g.Token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pull requests: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github api request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	var prs []PullRequest
	err = json.Unmarshal(body, &prs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return prs, nil
}
