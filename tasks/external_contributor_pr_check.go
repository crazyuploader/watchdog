package tasks

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"watchdog/internal/api"
	"watchdog/internal/config"
	"watchdog/internal/notifier"

	"github.com/rs/zerolog/log"
)

type ExternalContributorPRTask struct {
	config               config.ExternalContributorPRConfig
	apiClient            api.GitHubClient
	notifier             notifier.Notifier
	lastNotificationTime map[string]time.Time
	mu                   sync.Mutex
}

func NewExternalContributorPRTask(cfg config.ExternalContributorPRConfig, notifier notifier.Notifier) *ExternalContributorPRTask {
	return &ExternalContributorPRTask{
		config:               cfg,
		apiClient:            api.NewGitHubAPI(cfg.Token),
		notifier:             notifier,
		lastNotificationTime: make(map[string]time.Time),
	}
}

func (t *ExternalContributorPRTask) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, repoConfig := range t.config.Repositories {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		prs, err := t.apiClient.GetOpenPullRequests(ctx, repoConfig.Owner, repoConfig.Repo)
		if err != nil {
			log.Error().
				Err(err).
				Str("owner", repoConfig.Owner).
				Str("repo", repoConfig.Repo).
				Msg("Failed to fetch PRs for external contributor check")
			continue
		}

		externalPRs := t.filterExternalPRs(ctx, prs, repoConfig)
		if len(externalPRs) > 0 {
			t.notifyExternalPRs(ctx, externalPRs, repoConfig)
		}
	}

	t.cleanupOldNotifications()

	return nil
}

func (t *ExternalContributorPRTask) filterExternalPRs(ctx context.Context, prs []api.PullRequest, repoConfig config.ExternalContributorRepoConfig) []api.PullRequest {
	memberSet := make(map[string]bool)
	for _, member := range repoConfig.OrgMembers {
		memberSet[strings.ToLower(member)] = true
	}

	if len(memberSet) == 0 {
		members, err := t.apiClient.GetOrgMembers(ctx, repoConfig.Owner)
		if err != nil {
			log.Warn().
				Err(err).
				Str("org", repoConfig.Owner).
				Msg("Failed to fetch org members, skipping repo")
			return nil
		}

		for _, m := range members {
			memberSet[strings.ToLower(m.Login)] = true
		}

		if len(memberSet) == 0 {
			log.Warn().
				Str("org", repoConfig.Owner).
				Msg("No org members found, skipping repo")
			return nil
		}
	}

	lookbackDays := repoConfig.GetPRLookbackDays()
	cutoff := time.Now().AddDate(0, 0, -lookbackDays)

	var externalPRs []api.PullRequest
	for _, pr := range prs {
		if pr.Draft {
			continue
		}

		if !memberSet[strings.ToLower(pr.User.Login)] {
			if pr.CreatedAt.After(cutoff) {
				externalPRs = append(externalPRs, pr)
			}
		}
	}

	return externalPRs
}

func (t *ExternalContributorPRTask) notifyExternalPRs(ctx context.Context, prs []api.PullRequest, repoConfig config.ExternalContributorRepoConfig) {
	prID := fmt.Sprintf("%s/%s", repoConfig.Owner, repoConfig.Repo)

	t.mu.Lock()
	lastTime, ok := t.lastNotificationTime[prID]
	t.mu.Unlock()

	if ok {
		if time.Since(lastTime) < t.config.GetNotificationCooldown() {
			return
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "External contributor PRs in %s/%s:\n\n", repoConfig.Owner, repoConfig.Repo)

	for _, pr := range prs {
		age := time.Since(pr.CreatedAt)
		ageStr := formatDuration(age)
		fmt.Fprintf(&sb, "• #%d by %s - %s (%s old)\n  %s\n\n",
			pr.Number, pr.User.Login, pr.Title, ageStr, pr.HTMLURL)
	}

	subject := fmt.Sprintf("External PR Alert: %d new PR(s) in %s/%s", len(prs), repoConfig.Owner, repoConfig.Repo)
	message := sb.String()

	log.Info().
		Str("repo", prID).
		Int("count", len(prs)).
		Msg("Sending notification for external contributor PRs")

	err := t.notifier.SendNotification(ctx, subject, message)
	if err != nil {
		log.Error().Err(err).Str("repo", prID).Msg("Failed to send external PR notification")
	} else {
		t.mu.Lock()
		t.lastNotificationTime[prID] = time.Now()
		t.mu.Unlock()
	}
}

func (t *ExternalContributorPRTask) cleanupOldNotifications() {
	minCleanupAge := 7 * 24 * time.Hour
	cooldown := t.config.GetNotificationCooldown()

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
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
