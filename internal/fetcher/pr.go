package fetcher

import (
	"context"
	"fmt"
	"time"

	"github.com/fishnix/ghpr-analyzer/internal/ghclient"
	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// PRFetcher fetches pull requests for a repository
type PRFetcher struct {
	client   *github.Client
	ghClient *ghclient.Client
	logger   *zap.Logger
}

// NewPRFetcher creates a new PR fetcher
func NewPRFetcher(client *github.Client, ghClient *ghclient.Client, logger *zap.Logger) *PRFetcher {
	return &PRFetcher{
		client:   client,
		ghClient: ghClient,
		logger:   logger,
	}
}

// FetchClosedPRs fetches closed pull requests for a repository within a time window
func (p *PRFetcher) FetchClosedPRs(ctx context.Context, owner, repo string, since, until time.Time) ([]*github.PullRequest, error) {
	p.logger.Debug("Fetching closed PRs",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.Time("since", since),
		zap.Time("until", until),
	)

	var allPRs []*github.PullRequest
	var lastResp *github.Response
	opts := &github.PullRequestListOptions{
		State:       "closed",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		prs, resp, err := p.client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull requests for %s/%s: %w", owner, repo, err)
		}

		lastResp = resp

		// Filter PRs by closed date within the time window
		for _, pr := range prs {
			if pr.ClosedAt == nil {
				continue
			}

			closedAt := pr.ClosedAt.Time
			if closedAt.Before(since) {
				// Since we're sorting by updated desc, if we hit a PR before since, we can stop
				break
			}

			if closedAt.After(until) {
				continue
			}

			// PR is within the time window
			allPRs = append(allPRs, pr)
		}

		p.logger.Debug("Fetched PRs page",
			zap.String("repo", fmt.Sprintf("%s/%s", owner, repo)),
			zap.Int("page", opts.Page),
			zap.Int("count", len(prs)),
			zap.Int("filtered_count", len(allPRs)),
		)

		// Check rate limit and sleep if threshold is reached
		if p.ghClient != nil && resp != nil {
			if err := p.ghClient.CheckAndSleepIfNeeded(ctx, resp); err != nil {
				return nil, fmt.Errorf("rate limit check failed: %w", err)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage

		// If we've gone past the since date, we can stop
		if len(prs) > 0 && prs[len(prs)-1].ClosedAt != nil {
			if prs[len(prs)-1].ClosedAt.Time.Before(since) {
				break
			}
		}
	}

	// Build info log with rate limit information if available
	logFields := []zap.Field{
		zap.String("repo", fmt.Sprintf("%s/%s", owner, repo)),
		zap.Int("total_prs", len(allPRs)),
	}

	if lastResp != nil && lastResp.Rate.Limit > 0 {
		logFields = append(logFields,
			zap.Int("rate_limit", lastResp.Rate.Limit),
			zap.Int("rate_remaining", lastResp.Rate.Remaining),
			zap.Time("rate_reset", lastResp.Rate.Reset.Time),
		)
	}

	p.logger.Info("PR fetching complete", logFields...)

	return allPRs, nil
}

// FetchPRFiles fetches the list of files changed in a pull request
func (p *PRFetcher) FetchPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]*github.CommitFile, error) {
	var allFiles []*github.CommitFile
	opts := &github.ListOptions{PerPage: 100}

	for {
		files, resp, err := p.client.PullRequests.ListFiles(ctx, owner, repo, prNumber, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list files for PR #%d: %w", prNumber, err)
		}

		allFiles = append(allFiles, files...)

		// Check rate limit and sleep if threshold is reached
		if p.ghClient != nil && resp != nil {
			if err := p.ghClient.CheckAndSleepIfNeeded(ctx, resp); err != nil {
				return nil, fmt.Errorf("rate limit check failed: %w", err)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allFiles, nil
}
