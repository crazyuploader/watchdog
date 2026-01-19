package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGitHubAPI(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "with token",
			token: "ghp_test123",
		},
		{
			name:  "without token",
			token: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewGitHubAPI(tt.token)
			assert.NotNil(t, api)
			assert.Equal(t, "https://api.github.com", api.BaseURL)
			assert.Equal(t, tt.token, api.Token)
		})
	}
}

func TestGitHubAPI_GetOpenPullRequests_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/repos/testowner/testrepo/pulls", r.URL.Path)
		assert.Equal(t, "open", r.URL.Query().Get("state"))
		assert.Equal(t, "100", r.URL.Query().Get("per_page"))

		// Verify headers
		assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))
		assert.Equal(t, "watchdog-app", r.Header.Get("User-Agent"))

		// Send mock response
		prs := []PullRequest{
			{
				Number: 123,
				Title:  "Test PR",
				User: User{
					Login: "testuser",
				},
				CreatedAt: time.Now().Add(-48 * time.Hour),
				UpdatedAt: time.Now().Add(-24 * time.Hour),
				Draft:     false,
				HTMLURL:   "https://github.com/testowner/testrepo/pull/123",
			},
			{
				Number: 456,
				Title:  "Draft PR",
				User: User{
					Login: "anotheruser",
				},
				CreatedAt: time.Now().Add(-72 * time.Hour),
				UpdatedAt: time.Now().Add(-48 * time.Hour),
				Draft:     true,
				HTMLURL:   "https://github.com/testowner/testrepo/pull/456",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(prs)
	}))
	defer server.Close()

	// Create API client with mock server URL
	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	// Test
	prs, err := api.GetOpenPullRequests("testowner", "testrepo")

	// Assertions
	require.NoError(t, err)
	assert.Len(t, prs, 2)
	assert.Equal(t, 123, prs[0].Number)
	assert.Equal(t, "Test PR", prs[0].Title)
	assert.Equal(t, "testuser", prs[0].User.Login)
	assert.False(t, prs[0].Draft)
	assert.Equal(t, 456, prs[1].Number)
	assert.True(t, prs[1].Draft)
}

func TestGitHubAPI_GetOpenPullRequests_WithToken(t *testing.T) {
	token := "ghp_test123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify token in Authorization header
		assert.Equal(t, "token "+token, r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]PullRequest{})
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   token,
	}

	_, err := api.GetOpenPullRequests("owner", "repo")
	require.NoError(t, err)
}

func TestGitHubAPI_GetOpenPullRequests_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]PullRequest{})
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	prs, err := api.GetOpenPullRequests("owner", "repo")
	require.NoError(t, err)
	assert.Empty(t, prs)
}

func TestGitHubAPI_GetOpenPullRequests_NonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "404 not found",
			statusCode: http.StatusNotFound,
			body:       `{"message": "Not Found"}`,
		},
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       `{"message": "Bad credentials"}`,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       `{"message": "API rate limit exceeded"}`,
		},
		{
			name:       "500 internal server error",
			statusCode: http.StatusInternalServerError,
			body:       `{"message": "Internal Server Error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			api := &GitHubAPI{
				BaseURL: server.URL,
				Token:   "",
			}

			prs, err := api.GetOpenPullRequests("owner", "repo")
			assert.Error(t, err)
			assert.Nil(t, prs)
			assert.Contains(t, err.Error(), "github api request failed")
		})
	}
}

func TestGitHubAPI_GetOpenPullRequests_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`invalid json`))
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	prs, err := api.GetOpenPullRequests("owner", "repo")
	assert.Error(t, err)
	assert.Nil(t, prs)
	assert.Contains(t, err.Error(), "failed to unmarshal response")
}

func TestGitHubAPI_GetOpenPullRequests_ServerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than the 10s timeout
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	prs, err := api.GetOpenPullRequests("owner", "repo")
	assert.Error(t, err)
	assert.Nil(t, prs)
	assert.Contains(t, err.Error(), "failed to fetch pull requests")
}

func TestPullRequestJSON_Marshaling(t *testing.T) {
	now := time.Now()
	pr := PullRequest{
		Number:    123,
		Title:     "Test PR",
		User:      User{Login: "testuser"},
		CreatedAt: now,
		UpdatedAt: now,
		Draft:     false,
		HTMLURL:   "https://github.com/owner/repo/pull/123",
	}

	// Test marshaling
	data, err := json.Marshal(pr)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Test unmarshaling
	var decoded PullRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, pr.Number, decoded.Number)
	assert.Equal(t, pr.Title, decoded.Title)
	assert.Equal(t, pr.User.Login, decoded.User.Login)
	assert.Equal(t, pr.Draft, decoded.Draft)
}
