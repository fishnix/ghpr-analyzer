package analyzer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fishnix/ghpr-analyzer/internal/cache"
	"github.com/fishnix/ghpr-analyzer/internal/config"
	"github.com/fishnix/ghpr-analyzer/internal/exporter"
	"github.com/fishnix/ghpr-analyzer/internal/fetcher"
	"github.com/fishnix/ghpr-analyzer/internal/ghclient"
	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// Analyzer performs the PR analysis
type Analyzer struct {
	cfg               *config.Config
	ghClient          *ghclient.Client
	repoEnum          *fetcher.RepoEnumerator
	prFetcher         *fetcher.PRFetcher
	codeownersFetcher *fetcher.CODEOWNERSFetcher
	jsonExporter      *exporter.JSONExporter
	cache             cache.Cache
	skipAPICalls      bool
	logger            *zap.Logger
}

// NewAnalyzer creates a new analyzer
func NewAnalyzer(cfg *config.Config, ghClient *ghclient.Client, skipAPICalls bool, ignoreTTL bool, logger *zap.Logger) (*Analyzer, error) {
	client := ghClient.GetClient()

	repoEnum := fetcher.NewRepoEnumerator(client, ghClient, cfg.GitHub.Org, logger)
	prFetcher := fetcher.NewPRFetcher(client, ghClient, logger)
	codeownersFetcher := fetcher.NewCODEOWNERSFetcher(client, ghClient, logger)

	jsonExporter := exporter.NewJSONExporter(cfg.Output.OutputDir, logger)

	// Initialize cache
	var cacheInstance cache.Cache
	var err error
	if cfg.Cache.Backend != "" {
		// Convert TTL from minutes to duration
		ttl := time.Duration(cfg.Cache.TTLMinutes) * time.Minute
		if ttl == 0 {
			// Default to 24 hours if not set
			ttl = 24 * time.Hour
		}

		cacheInstance, err = cache.NewCache(
			cfg.Cache.Backend,
			cfg.Cache.SQLitePath,
			cfg.Cache.JSONDir,
			ttl,
			ignoreTTL,
			logger,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %w", err)
		}
	}

	return &Analyzer{
		cfg:               cfg,
		ghClient:          ghClient,
		repoEnum:          repoEnum,
		prFetcher:         prFetcher,
		codeownersFetcher: codeownersFetcher,
		jsonExporter:      jsonExporter,
		cache:             cacheInstance,
		skipAPICalls:      skipAPICalls,
		logger:            logger,
	}, nil
}

// Analyze performs the complete analysis
func (a *Analyzer) Analyze(ctx context.Context) error {
	a.logger.Info("Starting PR analysis",
		zap.String("org", a.cfg.GitHub.Org),
	)

	// Get time window
	since, until, err := a.cfg.GetTimeWindow()
	if err != nil {
		return fmt.Errorf("failed to get time window: %w", err)
	}

	a.logger.Info("Time window",
		zap.String("since", since.Format(time.RFC3339)),
		zap.String("until", until.Format(time.RFC3339)),
	)

	// Enumerate repositories (check cache first)
	var repos []*github.Repository
	if a.cache != nil {
		a.logger.Debug("Cache is configured, checking for cached repositories")

		cachedRepos, err := a.cache.GetRepos(ctx, a.cfg.GitHub.Org)
		if err == nil && len(cachedRepos) > 0 {
			a.logger.Info("Using cached repositories", zap.Int("count", len(cachedRepos)))
			repos = cachedRepos
		}
	}

	a.logger.Info("Repositories", zap.Int("count", len(repos)))
	if len(repos) == 0 {
		return fmt.Errorf("no repositories found")
	}

	// Fetch from API if not cached or cache-only mode
	if len(repos) == 0 {
		if a.skipAPICalls {
			return fmt.Errorf("no cached repositories found and --skip-api-calls is enabled")
		}

		var err error
		repos, err = a.repoEnum.EnumerateRepos(ctx)
		if err != nil {
			return fmt.Errorf("failed to enumerate repositories: %w", err)
		}

		// Cache repositories
		if a.cache != nil {
			if err := a.cache.SetRepos(ctx, a.cfg.GitHub.Org, repos); err != nil {
				a.logger.Warn("Failed to cache repositories", zap.Error(err))
			}
		}
	}

	a.logger.Info("Found repositories", zap.Int("count", len(repos)))

	// Process repositories concurrently
	results := a.processRepos(ctx, repos, since, until)

	// Aggregate results
	a.logger.Info("Aggregating results from processed repositories")
	aggregated := a.aggregateResults(ctx, results, since, until)
	a.logger.Info("Aggregation complete",
		zap.Int("total_prs", aggregated.TotalPRsClosed),
		zap.Int("repos_count", len(aggregated.PRsByRepo)),
		zap.Int("teams_count", len(aggregated.PRsByTeam)),
		zap.Int("users_count", len(aggregated.PRsByUser)),
	)

	// Export results based on format
	a.logger.Info("Starting export", zap.String("format", a.cfg.Output.Format))
	switch a.cfg.Output.Format {
	case "csv":
		csvExporter := exporter.NewCSVExporter(a.cfg.Output.OutputDir, a.logger)
		if err := csvExporter.Export(aggregated); err != nil {
			return fmt.Errorf("failed to export CSV results: %w", err)
		}
		// Also export JSON for compatibility
		if err := a.jsonExporter.Export(aggregated); err != nil {
			return fmt.Errorf("failed to export JSON results: %w", err)
		}
		// Also export human summary
		summaryExporter := exporter.NewSummaryExporter(a.logger)
		if err := summaryExporter.Export(aggregated); err != nil {
			return fmt.Errorf("failed to export summary: %w", err)
		}
	case "json":
		fallthrough
	default:
		if err := a.jsonExporter.Export(aggregated); err != nil {
			return fmt.Errorf("failed to export results: %w", err)
		}
		// Also export human summary
		summaryExporter := exporter.NewSummaryExporter(a.logger)
		if err := summaryExporter.Export(aggregated); err != nil {
			return fmt.Errorf("failed to export summary: %w", err)
		}
	}

	// Export per-repo PRs (JSON only for now)
	a.logger.Info("Preparing per-repo PR export")
	repoPRs := make(map[string][]*github.PullRequest)
	for _, result := range results {
		if result.Repo != nil {
			repoName := fmt.Sprintf("%s/%s", result.Repo.GetOwner().GetLogin(), result.Repo.GetName())
			repoPRs[repoName] = result.PRs
		}
	}
	a.logger.Info("Exporting per-repo PRs to JSON", zap.Int("repo_count", len(repoPRs)))
	if err := a.jsonExporter.ExportPerRepo(repoPRs); err != nil {
		return fmt.Errorf("failed to export per-repo results: %w", err)
	}

	a.logger.Info("Analysis complete",
		zap.Int("total_prs", aggregated.TotalPRsClosed),
		zap.Int("repos_analyzed", len(repos)),
	)

	// Close cache
	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
			a.logger.Warn("Failed to close cache", zap.Error(err))
		}
	}

	return nil
}

// RepoResult holds the results for a single repository
type RepoResult struct {
	Repo       *github.Repository
	PRs        []*github.PullRequest
	CODEOWNERS *fetcher.CODEOWNERSFile
	Err        error
}

// PROwners holds the owners for a PR
type PROwners struct {
	PR     *github.PullRequest
	Owners []string
}

func (a *Analyzer) processRepos(ctx context.Context, repos []*github.Repository, since, until time.Time) []RepoResult {
	// Create worker pool
	numWorkers := a.cfg.Concurrency.RepoWorkers
	if numWorkers <= 0 {
		numWorkers = 8
	}

	results := make([]RepoResult, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, numWorkers)

	for i, repo := range repos {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		go func(idx int, r *github.Repository) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			result := a.processRepo(ctx, r, since, until)
			results[idx] = result
		}(i, repo)
	}

	wg.Wait()
	return results
}

func (a *Analyzer) processRepo(ctx context.Context, repo *github.Repository, since, until time.Time) RepoResult {
	owner := repo.GetOwner().GetLogin()
	name := repo.GetName()

	a.logger.Debug("Processing repository",
		zap.String("owner", owner),
		zap.String("repo", name),
	)

	// Wait for rate limiter before fetching
	if err := a.ghClient.WaitForRateLimit(ctx); err != nil {
		return RepoResult{
			Repo: repo,
			Err:  fmt.Errorf("rate limiter wait failed: %w", err),
		}
	}

	// Fetch CODEOWNERS file (check cache first)
	var codeowners *fetcher.CODEOWNERSFile
	if a.cache != nil {
		cachedContent, err := a.cache.GetCODEOWNERS(ctx, owner, name)
		if err == nil && len(cachedContent) > 0 {
			// Parse cached CODEOWNERS
			// Create a temporary fetcher for parsing (no client needed for parsing)
			tempFetcher := fetcher.NewCODEOWNERSFetcher(nil, nil, a.logger)
			codeowners, err = tempFetcher.ParseCODEOWNERS(cachedContent, "")
			if err != nil {
				a.logger.Warn("Failed to parse cached CODEOWNERS", zap.Error(err))
			}
		}
	}

	// Fetch from API if not cached
	if codeowners == nil {
		if !a.skipAPICalls {
			var err error
			var rawContent []byte
			codeowners, rawContent, err = a.codeownersFetcher.FetchCODEOWNERS(ctx, owner, name)
			if err != nil {
				a.logger.Warn("Failed to fetch CODEOWNERS",
					zap.String("repo", fmt.Sprintf("%s/%s", owner, name)),
					zap.Error(err),
				)
				// Continue without CODEOWNERS
				codeowners = nil
			} else if codeowners != nil && a.cache != nil && len(rawContent) > 0 {
				// Cache CODEOWNERS raw content
				if err := a.cache.SetCODEOWNERS(ctx, owner, name, rawContent); err != nil {
					a.logger.Warn("Failed to cache CODEOWNERS", zap.Error(err))
				}
			}
		} else {
			a.logger.Debug("Skipping CODEOWNERS fetch (cache-only mode)",
				zap.String("repo", fmt.Sprintf("%s/%s", owner, name)),
			)
		}
	}

	// Fetch PRs (check cache first)
	var prs []*github.PullRequest
	if a.cache != nil {
		cachedPRs, err := a.cache.GetPRs(ctx, owner, name, since, until)
		if err == nil && len(cachedPRs) > 0 {
			a.logger.Debug("Using cached PRs",
				zap.String("repo", fmt.Sprintf("%s/%s", owner, name)),
				zap.Int("count", len(cachedPRs)),
			)
			prs = cachedPRs
		}
	}

	// Fetch from API if not cached
	if len(prs) == 0 {
		if a.skipAPICalls {
			return RepoResult{
				Repo:       repo,
				CODEOWNERS: codeowners,
				Err:        fmt.Errorf("no cached PRs found and --skip-api-calls is enabled"),
			}
		}

		var err error
		prs, err = a.prFetcher.FetchClosedPRs(ctx, owner, name, since, until)
		if err != nil {
			return RepoResult{
				Repo:       repo,
				CODEOWNERS: codeowners,
				Err:        fmt.Errorf("failed to fetch PRs: %w", err),
			}
		}

		// Cache PRs
		if a.cache != nil {
			if err := a.cache.SetPRs(ctx, owner, name, since, until, prs); err != nil {
				a.logger.Warn("Failed to cache PRs", zap.Error(err))
			}
		}
	}

	// Apply filters
	filteredPRs := a.applyFilters(prs)

	return RepoResult{
		Repo:       repo,
		PRs:        filteredPRs,
		CODEOWNERS: codeowners,
	}
}

func (a *Analyzer) applyFilters(prs []*github.PullRequest) []*github.PullRequest {
	var filtered []*github.PullRequest

	excludeAuthors := make(map[string]bool)
	for _, author := range a.cfg.Filters.ExcludeAuthors {
		excludeAuthors[author] = true
	}

	excludePrefixes := a.cfg.Filters.ExcludeTitlePrefixes

	for _, pr := range prs {
		// Check author exclusion
		if pr.User != nil {
			author := pr.User.GetLogin()
			if excludeAuthors[author] {
				a.logger.Debug("Excluding PR by author",
					zap.Int("pr_number", pr.GetNumber()),
					zap.String("author", author),
				)
				continue
			}
		}

		// Check title prefix exclusion
		title := pr.GetTitle()
		excluded := false
		for _, prefix := range excludePrefixes {
			if len(title) >= len(prefix) && title[:len(prefix)] == prefix {
				a.logger.Debug("Excluding PR by title prefix",
					zap.Int("pr_number", pr.GetNumber()),
					zap.String("prefix", prefix),
				)
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		filtered = append(filtered, pr)
	}

	return filtered
}

// mapPROwners maps PR changed files to CODEOWNERS owners
func (a *Analyzer) mapPROwners(ctx context.Context, pr *github.PullRequest, codeowners *fetcher.CODEOWNERSFile, owner, repo string) []string {
	if codeowners == nil {
		return nil
	}

	// Fetch PR changed files (check cache first)
	var prFiles []*github.CommitFile
	if a.cache != nil {
		cachedFiles, err := a.cache.GetPRFiles(ctx, owner, repo, pr.GetNumber())
		if err == nil && len(cachedFiles) > 0 {
			prFiles = cachedFiles
		}
	}

	// Fetch from API if not cached
	if len(prFiles) == 0 {
		if !a.skipAPICalls {
			var err error
			prFiles, err = a.prFetcher.FetchPRFiles(ctx, owner, repo, pr.GetNumber())
			if err != nil {
				a.logger.Debug("Failed to fetch PR files",
					zap.Int("pr_number", pr.GetNumber()),
					zap.Error(err),
				)
				return nil
			}

			// Cache PR files
			if a.cache != nil {
				if err := a.cache.SetPRFiles(ctx, owner, repo, pr.GetNumber(), prFiles); err != nil {
					a.logger.Warn("Failed to cache PR files", zap.Error(err))
				}
			}
		} else {
			a.logger.Debug("Skipping PR files fetch (cache-only mode)",
				zap.Int("pr_number", pr.GetNumber()),
			)
			return nil
		}
	}

	// Collect all owners from all changed files
	allOwners := make(map[string]bool)
	for _, file := range prFiles {
		filePath := file.GetFilename()
		owners := codeowners.FindOwners(filePath)
		for _, owner := range owners {
			allOwners[owner] = true
		}
	}

	// Convert to slice
	var owners []string
	for owner := range allOwners {
		owners = append(owners, owner)
	}

	return owners
}

// applyAttributionMode applies the attribution mode to owners
func (a *Analyzer) applyAttributionMode(owners []string) []string {
	if len(owners) == 0 {
		return owners
	}

	mode := a.cfg.Attribution.Mode
	switch mode {
	case "first-owner-only":
		// Return only the first owner
		return []string{owners[0]}
	case "primary":
		// For now, treat primary as first owner
		// In a full implementation, this might consider team hierarchy
		return []string{owners[0]}
	case "multi":
		// Return all owners
		return owners
	default:
		// Default to multi
		return owners
	}
}

// normalizeOwner normalizes owner name (handles @ prefix, team format)
func normalizeOwner(owner string) string {
	// Remove @ prefix if present
	return strings.TrimPrefix(owner, "@")
}

// getRollupTeams returns the rollup team names for a given team
func (a *Analyzer) getRollupTeams(team string) []string {
	var rollupTeams []string
	normalizedTeam := normalizeOwner(team)

	for _, rollup := range a.cfg.TeamRollup {
		for _, rollupTeam := range rollup.Teams {
			if normalizeOwner(rollupTeam) == normalizedTeam {
				rollupTeams = append(rollupTeams, rollup.Name)
				break // Team can be in multiple rollups, but we only add each rollup name once
			}
		}
	}

	return rollupTeams
}

// isTeamInRollup checks if a team is part of any rollup configuration
func (a *Analyzer) isTeamInRollup(team string) bool {
	normalizedTeam := normalizeOwner(team)

	for _, rollup := range a.cfg.TeamRollup {
		for _, rollupTeam := range rollup.Teams {
			if normalizeOwner(rollupTeam) == normalizedTeam {
				return true
			}
		}
	}

	return false
}

func (a *Analyzer) aggregateResults(ctx context.Context, results []RepoResult, since, until time.Time) *exporter.AnalysisResult {
	aggregated := &exporter.AnalysisResult{
		PRsByRepo: make(map[string]int),
		PRsByTeam: make(map[string]int),
		PRsByUser: make(map[string]int),
		TimeWindow: exporter.TimeWindow{
			Since: since,
			Until: until,
		},
		GeneratedAt: time.Now(),
	}

	totalPRs := 0
	for _, result := range results {
		if result.PRs != nil {
			totalPRs += len(result.PRs)
		}
	}

	a.logger.Info("Processing aggregation",
		zap.Int("total_repos", len(results)),
		zap.Int("total_prs_to_process", totalPRs),
	)

	processedCount := 0
	for _, result := range results {
		if result.Err != nil {
			a.logger.Warn("Repository processing error",
				zap.String("repo", fmt.Sprintf("%s/%s", result.Repo.GetOwner().GetLogin(), result.Repo.GetName())),
				zap.Error(result.Err),
			)
			continue
		}

		repoName := fmt.Sprintf("%s/%s", result.Repo.GetOwner().GetLogin(), result.Repo.GetName())
		prCount := len(result.PRs)
		aggregated.PRsByRepo[repoName] = prCount
		aggregated.TotalPRsClosed += prCount

		// Count by user (author)
		for _, pr := range result.PRs {
			if pr.User != nil {
				user := pr.User.GetLogin()
				aggregated.PRsByUser[user]++
			}
		}

		// Count by team (CODEOWNERS)
		owner := result.Repo.GetOwner().GetLogin()
		name := result.Repo.GetName()
		hasCodeowners := result.CODEOWNERS != nil

		if hasCodeowners && len(result.PRs) > 0 {
			a.logger.Debug("Mapping PRs to CODEOWNERS owners",
				zap.String("repo", fmt.Sprintf("%s/%s", owner, name)),
				zap.Int("pr_count", len(result.PRs)),
			)
		}

		for _, pr := range result.PRs {
			var owners []string
			if hasCodeowners {
				// Map PR files to owners
				prOwners := a.mapPROwners(ctx, pr, result.CODEOWNERS, owner, name)
				// Apply attribution mode
				owners = a.applyAttributionMode(prOwners)
			}

			if len(owners) == 0 {
				// No owners found, use "no_codeowners"
				aggregated.PRsByTeam["no_codeowners"]++
			} else {
				// Track which rollup teams this PR should be counted under (to avoid double-counting)
				rollupTeamsSet := make(map[string]bool)
				nonRollupTeams := make(map[string]bool)

				// Process each owner
				for _, owner := range owners {
					normalized := normalizeOwner(owner)

					// Check if this team is part of a rollup
					if a.isTeamInRollup(owner) {
						// Team is in a rollup, add to rollup teams set
						rollupTeams := a.getRollupTeams(owner)
						for _, rollupTeam := range rollupTeams {
							rollupTeamsSet[rollupTeam] = true
						}
					} else {
						// Team is not in a rollup, count under individual team name
						nonRollupTeams[normalized] = true
					}
				}

				// Count each rollup team once per PR
				for rollupTeam := range rollupTeamsSet {
					aggregated.PRsByTeam[rollupTeam]++
				}

				// Count each non-rollup team once per PR
				for team := range nonRollupTeams {
					aggregated.PRsByTeam[team]++
				}
			}
		}

		processedCount++
		if processedCount%10 == 0 || processedCount == len(results) {
			a.logger.Debug("Aggregation progress",
				zap.Int("processed", processedCount),
				zap.Int("total", len(results)),
				zap.Int("prs_processed_so_far", aggregated.TotalPRsClosed),
			)
		}
	}

	return aggregated
}
