package api

import (
	"context"
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
		if err := json.NewEncoder(w).Encode(prs); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	// Create API client with mock server URL
	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	// Test
	ctx := context.Background()
	prs, err := api.GetOpenPullRequests(ctx, "testowner", "testrepo")

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
		if err := json.NewEncoder(w).Encode([]PullRequest{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   token,
	}

	ctx := context.Background()
	_, err := api.GetOpenPullRequests(ctx, "owner", "repo")
	require.NoError(t, err)
}

func TestGitHubAPI_GetOpenPullRequests_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode([]PullRequest{}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	ctx := context.Background()
	prs, err := api.GetOpenPullRequests(ctx, "owner", "repo")
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if _, err := w.Write([]byte(tt.body)); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			}))
			defer server.Close()

			api := &GitHubAPI{
				BaseURL: server.URL,
				Token:   "",
			}

			ctx := context.Background()
			prs, err := api.GetOpenPullRequests(ctx, "owner", "repo")
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
		if _, err := w.Write([]byte(`invalid json`)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	ctx := context.Background()
	prs, err := api.GetOpenPullRequests(ctx, "owner", "repo")
	assert.Error(t, err)
	assert.Nil(t, prs)
	assert.Contains(t, err.Error(), "failed to unmarshal response")
}

func TestGitHubAPI_GetOpenPullRequests_ServerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(15 * time.Second) // Longer than the timeout
	}))
	defer server.Close()

	api := &GitHubAPI{
		BaseURL: server.URL,
		Token:   "",
	}

	// Use a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	prs, err := api.GetOpenPullRequests(ctx, "owner", "repo")
	assert.Error(t, err)
	assert.Nil(t, prs)
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

func TestGitHubAPI_GetPullRequest_Success(t *testing.T) {
	tests := []struct {
		name          string
		mergeable     *bool
		mergeState    string
		merged        bool
		wantMergeable bool
		wantState     string
	}{
		{name: "clean mergeable", mergeable: boolPtr(true), mergeState: "clean", wantMergeable: true, wantState: "clean"},
		{name: "dirty conflict", mergeable: boolPtr(false), mergeState: "dirty", wantMergeable: false, wantState: "dirty"},
		{name: "blocked", mergeable: boolPtr(false), mergeState: "blocked", wantMergeable: false, wantState: "blocked"},
		{name: "already merged", mergeable: boolPtr(false), mergeState: "unknown", merged: true, wantMergeable: false, wantState: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/repos/testowner/testrepo/pulls/123", r.URL.Path)

				pr := PullRequest{
					Number:         123,
					Title:          "Test PR",
					User:           User{Login: "testuser"},
					HTMLURL:        "https://github.com/testowner/testrepo/pull/123",
					Merged:         tt.merged,
					Mergeable:      tt.mergeable,
					MergeableState: tt.mergeState,
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(pr)
			}))
			defer server.Close()

			api := &GitHubAPI{BaseURL: server.URL, Token: "ghp_test"}

			pr, err := api.GetPullRequest(context.Background(), "testowner", "testrepo", 123)
			require.NoError(t, err)
			require.NotNil(t, pr)
			require.NotNil(t, pr.Mergeable)
			assert.Equal(t, tt.wantMergeable, *pr.Mergeable)
			assert.Equal(t, tt.wantState, pr.MergeableState)
			assert.Equal(t, tt.merged, pr.Merged)
		})
	}
}

func TestGitHubAPI_GetPullRequest_BackgroundComputation(t *testing.T) {
	// GitHub returns a null mergeable while it computes mergeability in the background.
	// The client should re-request until a non-null value appears.
	original := mergeabilityComputeDelay
	mergeabilityComputeDelay = 10 * time.Millisecond
	defer func() { mergeabilityComputeDelay = original }()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		pr := PullRequest{Number: 7, MergeableState: "unknown"}
		if calls >= 2 {
			// Second call: computation finished, conflict detected.
			pr.Mergeable = boolPtr(false)
			pr.MergeableState = "dirty"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(pr)
	}))
	defer server.Close()

	api := &GitHubAPI{BaseURL: server.URL}

	pr, err := api.GetPullRequest(context.Background(), "owner", "repo", 7)
	require.NoError(t, err)
	require.NotNil(t, pr.Mergeable)
	assert.False(t, *pr.Mergeable)
	assert.Equal(t, "dirty", pr.MergeableState)
	assert.GreaterOrEqual(t, calls, 2)
}

func TestGitHubAPI_GetPullRequest_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	api := &GitHubAPI{BaseURL: server.URL}

	pr, err := api.GetPullRequest(context.Background(), "owner", "repo", 999)
	assert.Error(t, err)
	assert.Nil(t, pr)
}

func TestGitHubAPI_GetPullRequest_ContextCancelledWhileComputing(t *testing.T) {
	// Always returns null mergeable; the client should give up when the context expires
	// during the wait between re-computation polls.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr := PullRequest{Number: 1, MergeableState: "unknown"}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(pr)
	}))
	defer server.Close()

	api := &GitHubAPI{BaseURL: server.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	pr, err := api.GetPullRequest(ctx, "owner", "repo", 1)
	assert.Error(t, err)
	assert.Nil(t, pr)
}

func boolPtr(b bool) *bool { return &b }
