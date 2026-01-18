package tasks

import (
	"fmt"
	"strings"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/config"
	"watchdog/internal/notifier"
)

// PRReviewCheckTask checks for stale pull requests and notifies via the configured notifier.
type PRReviewCheckTask struct {
	config               config.GitHubConfig
	apiClient            *api.GitHubAPI
	notifier             notifier.Notifier
	lastNotificationTime map[string]time.Time
}

// NewPRReviewCheckTask creates a new instance of PRReviewCheckTask.
func NewPRReviewCheckTask(cfg config.GitHubConfig, notifier notifier.Notifier) *PRReviewCheckTask {
	return &PRReviewCheckTask{
		config:               cfg,
		apiClient:            api.NewGitHubAPI(cfg.Token),
		notifier:             notifier,
		lastNotificationTime: make(map[string]time.Time),
	}
}

func (t *PRReviewCheckTask) Run() error {
	staleDays := t.config.GetStaleDays()

	for _, repoConfig := range t.config.Repositories {
		prs, err := t.apiClient.GetOpenPullRequests(repoConfig.Owner, repoConfig.Repo)
		if err != nil {
			fmt.Printf("Failed to fetch PRs for %s/%s: %v\n", repoConfig.Owner, repoConfig.Repo, err)
			continue
		}

		for _, pr := range prs {
			// Skip drafts
			if pr.Draft {
				continue
			}

			// Filter by author if configured
			if len(repoConfig.Authors) > 0 {
				isAuthorMatch := false
				for _, author := range repoConfig.Authors {
					if strings.EqualFold(pr.User.Login, author) {
						isAuthorMatch = true
						break
					}
				}
				if !isAuthorMatch {
					continue
				}
			}

			// Check age
			if time.Since(pr.UpdatedAt) < time.Duration(staleDays)*24*time.Hour {
				continue
			}

			// Check notification cooldown
			prID := fmt.Sprintf("%s/%s#%d", repoConfig.Owner, repoConfig.Repo, pr.Number)
			if lastTime, ok := t.lastNotificationTime[prID]; ok {
				if time.Since(lastTime) < t.config.GetNotificationCooldown() {
					continue
				}
			}

			// Send notification
			subject := fmt.Sprintf("Stale PR: %s", pr.Title)
			message := fmt.Sprintf("PR #%d in %s/%s by %s is pending review.\nLast updated: %s\nLink: %s",
				pr.Number, repoConfig.Owner, repoConfig.Repo, pr.User.Login,
				pr.UpdatedAt.Format(time.RFC1123), pr.HTMLURL)

			fmt.Printf("Sending notification for stale PR: %s\n", prID)
			err := t.notifier.SendNotification(subject, message)
			if err != nil {
				fmt.Printf("Failed to send notification for %s: %v\n", prID, err)
			} else {
				t.lastNotificationTime[prID] = time.Now()
			}
		}
	}

	return nil
}
