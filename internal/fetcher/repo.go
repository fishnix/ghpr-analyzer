package fetcher

import (
	"context"
	"fmt"

	"github.com/fishnix/ghpr-analyzer/internal/ghclient"
	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// RepoEnumerator enumerates repositories in a GitHub organization
type RepoEnumerator struct {
	client   *github.Client
	ghClient *ghclient.Client
	org      string
	logger   *zap.Logger
}

// NewRepoEnumerator creates a new repo enumerator
func NewRepoEnumerator(client *github.Client, ghClient *ghclient.Client, org string, logger *zap.Logger) *RepoEnumerator {
	return &RepoEnumerator{
		client:   client,
		ghClient: ghClient,
		org:      org,
		logger:   logger,
	}
}

// EnumerateRepos lists all repositories in the organization
func (r *RepoEnumerator) EnumerateRepos(ctx context.Context) ([]*github.Repository, error) {
	r.logger.Info("Enumerating repositories", zap.String("org", r.org))

	var allRepos []*github.Repository
	var lastResp *github.Response
	opts := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := r.client.Repositories.ListByOrg(ctx, r.org, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}

		lastResp = resp
		allRepos = append(allRepos, repos...)
		r.logger.Debug("Fetched repositories page",
			zap.Int("page", opts.Page),
			zap.Int("count", len(repos)),
			zap.Int("total", len(allRepos)),
		)

		// Check rate limit and sleep if threshold is reached
		if r.ghClient != nil && resp != nil {
			if err := r.ghClient.CheckAndSleepIfNeeded(ctx, resp); err != nil {
				return nil, fmt.Errorf("rate limit check failed: %w", err)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Build info log with rate limit information if available
	logFields := []zap.Field{
		zap.String("org", r.org),
		zap.Int("total_repos", len(allRepos)),
	}

	if lastResp != nil && lastResp.Rate.Limit > 0 {
		logFields = append(logFields,
			zap.Int("rate_limit", lastResp.Rate.Limit),
			zap.Int("rate_remaining", lastResp.Rate.Remaining),
			zap.Time("rate_reset", lastResp.Rate.Reset.Time),
		)
	}

	r.logger.Info("Repository enumeration complete", logFields...)

	return allRepos, nil
}
