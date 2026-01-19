package tasks

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/config"
	"watchdog/internal/notifier"

	"github.com/rs/zerolog/log"
)

// PRReviewCheckTask monitors GitHub repositories for stale pull requests.
// A PR is considered "stale" if it hasn't been updated in X days (configured via stale_days).
//
// The task:
//  1. Fetches all open PRs from configured repositories
//  2. Filters PRs by author (if configured)
//  3. Checks if PRs are older than the stale threshold
//  4. Sends notifications for stale PRs (with cooldown to prevent spam)
//
// This implements the scheduler.Task interface via the Run() method.
type PRReviewCheckTask struct {
	// config holds the GitHub monitoring configuration (repos, stale days, cooldown, etc.)
	config config.GitHubConfig

	// apiClient is used to fetch PR data from GitHub
	apiClient api.GitHubClient

	// notifier is used to send alerts (via Apprise/Telegram/Discord/etc.)
	notifier notifier.Notifier

	// lastNotificationTime tracks when we last notified about each PR
	// Key format: "owner/repo#123" (e.g., "signoz/signoz-web#456")
	// This prevents spamming notifications for the same PR
	lastNotificationTime map[string]time.Time

	// mu guards access to lastNotificationTime to prevent data races
	mu sync.Mutex
}

// NewPRReviewCheckTask creates a new PR monitoring task.
// Parameters:
//   - cfg: GitHub configuration (repos to monitor, stale threshold, etc.)
//   - notifier: Where to send notifications (Apprise webhook, Telegram, etc.)
//
// The task will use the GitHub token from cfg for API authentication (if provided).
func NewPRReviewCheckTask(cfg config.GitHubConfig, notifier notifier.Notifier) *PRReviewCheckTask {
	return &PRReviewCheckTask{
		config:               cfg,
		apiClient:            api.NewGitHubAPI(cfg.Token),
		notifier:             notifier,
		lastNotificationTime: make(map[string]time.Time),
	}
}

// Run executes the PR monitoring logic.
// This method is called periodically by the scheduler (e.g., every 5 minutes).
//
// For each configured repository, it:
//  1. Fetches all open PRs from GitHub
//  2. Filters out draft PRs (not ready for review)
//  3. Filters by author if configured (only watch specific team members)
//  4. Checks if the PR is stale (not updated in X days)
//  5. Sends a notification if stale (respecting cooldown period)
//
// Returns:
//   - Always returns nil (errors are logged but don't stop the scheduler)
//   - Individual repo/PR failures are logged and skipped
func (t *PRReviewCheckTask) Run() error {
	staleDays := t.config.GetStaleDays()

	// Iterate through all configured repositories
	for _, repoConfig := range t.config.Repositories {
		// Fetch open PRs from GitHub
		prs, err := t.apiClient.GetOpenPullRequests(repoConfig.Owner, repoConfig.Repo)
		if err != nil {
			// Log the error but continue with other repos
			log.Error().
				Err(err).
				Str("owner", repoConfig.Owner).
				Str("repo", repoConfig.Repo).
				Msg("Failed to fetch PRs")
			continue
		}

		// Check each PR for staleness
		for _, pr := range prs {
			// Skip draft PRs - they're not ready for review yet
			if pr.Draft {
				continue
			}

			// Filter by author if configured
			// If authors list is empty, we monitor all PRs
			// If authors list is specified, only monitor PRs by those users
			if len(repoConfig.Authors) > 0 {
				isAuthorMatch := false
				for _, author := range repoConfig.Authors {
					// Case-insensitive comparison
					if strings.EqualFold(pr.User.Login, author) {
						isAuthorMatch = true
						break
					}
				}
				// Skip this PR if author doesn't match our filter
				if !isAuthorMatch {
					continue
				}
			}

			// Check if PR is stale
			// We use UpdatedAt (last activity time) rather than CreatedAt
			// This way, PRs with recent comments/commits won't trigger alerts
			if time.Since(pr.UpdatedAt) < time.Duration(staleDays)*24*time.Hour {
				continue // PR is still fresh, skip it
			}

			// Check notification cooldown
			// We don't want to spam notifications for the same PR every 5 minutes
			// The cooldown (default 24h) ensures we only notify once per day per PR
			prID := fmt.Sprintf("%s/%s#%d", repoConfig.Owner, repoConfig.Repo, pr.Number)

			t.mu.Lock()
			lastTime, ok := t.lastNotificationTime[prID]
			t.mu.Unlock()

			if ok {
				if time.Since(lastTime) < t.config.GetNotificationCooldown() {
					continue // We notified about this PR recently, skip it
				}
			}

			// PR is stale and we haven't notified recently - send notification
			subject := fmt.Sprintf("Stale PR: %s", pr.Title)

			// Check CI status (Commit Status + Check Suites)
			var ciMsg string

			// 1. Get Commit Status (Legacy / CircleCI / Jenkins)
			commitStatus, errStatus := t.apiClient.GetCommitStatus(repoConfig.Owner, repoConfig.Repo, pr.Head.SHA)
			if errStatus != nil {
				log.Error().Err(errStatus).Str("pr", prID).Msg("Failed to check commit status")
			}

			// 2. Get Check Suites (GitHub Actions)
			checkSuites, errChecks := t.apiClient.GetCheckSuites(repoConfig.Owner, repoConfig.Repo, pr.Head.SHA)
			if errChecks != nil {
				log.Error().Err(errChecks).Str("pr", prID).Msg("Failed to check suites")
			}

			// 3. Combine Logic
			// Priority: Failure only. We assume success/pending unless we find a failure.
			isFailure := false

			// Check Commit Status
			if commitStatus != nil {
				switch commitStatus.State {
				case "failure", "error":
					isFailure = true
				}
			}

			// Check Suites
			if checkSuites != nil {
				for _, suite := range checkSuites.CheckSuites {
					if suite.Conclusion == "failure" || suite.Conclusion == "timed_out" || suite.Conclusion == "cancelled" {
						isFailure = true
						break
					}
				}
			}

			if isFailure {
				ciMsg = " (CI: Failing ❌)"
			}

			message := fmt.Sprintf("PR #%d in %s/%s by %s is pending review.%s\nLast updated: %s\nLink: %s",
				pr.Number, repoConfig.Owner, repoConfig.Repo, pr.User.Login,
				ciMsg,
				pr.UpdatedAt.Format(time.RFC1123), pr.HTMLURL)

			log.Info().Str("pr", prID).Msg("Sending notification for stale PR")
			err = t.notifier.SendNotification(subject, message)
			if err != nil {
				// Log the error but continue with other PRs
				log.Error().Err(err).Str("pr", prID).Msg("Failed to send notification")
			} else {
				// Record that we sent a notification for this PR
				// This starts the cooldown period
				t.mu.Lock()
				t.lastNotificationTime[prID] = time.Now()
				t.mu.Unlock()
			}
		}
	}

	// Cleanup old entries from lastNotificationTime map to prevent memory leak
	// Remove entries older than 7 days (or configured cooldown if longer)
	// This ensures we respect the cooldown while eventually cleaning up closed/merged PRs
	minCleanupAge := 7 * 24 * time.Hour
	cooldown := t.config.GetNotificationCooldown()

	// Use the larger of the two to avoid cleaning up before cooldown expires
	cleanupThreshold := minCleanupAge
	if cooldown > minCleanupAge {
		cleanupThreshold = cooldown
	}

	t.mu.Lock()
	for prID, lastTime := range t.lastNotificationTime {
		if time.Since(lastTime) > cleanupThreshold {
			delete(t.lastNotificationTime, prID)
		}
	}
	t.mu.Unlock()

	// Always return nil - we don't want task errors to stop the scheduler
	return nil
}
