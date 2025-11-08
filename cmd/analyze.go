package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/fishnix/golang-template/internal/analyzer"
	"github.com/fishnix/golang-template/internal/cache"
	"github.com/fishnix/golang-template/internal/config"
	"github.com/fishnix/golang-template/internal/ghclient"
	"go.uber.org/zap"
)

var (
	orgFlag              string
	sinceFlag            string
	untilFlag            string
	excludeAuthorFlags   []string
	excludeTitlePrefixes []string
	outputFormatFlag     string
	outputDirFlag        string
	skipAPICallsFlag     bool
	invalidateCacheFlag  bool
	dryRunFlag           bool
)

// analyzeCmd starts analysis
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: fmt.Sprintf("starts %s", appName),
	Run: func(c *cobra.Command, _ []string) {
		defer mustSync()
		if err := analyze(c.Context()); err != nil {
			logger.Error("Analysis failed", zap.Error(err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)

	// Bind flags to viper
	analyzeCmd.Flags().StringVar(&orgFlag, "org", "", "GitHub organization name")
	analyzeCmd.Flags().StringVar(&sinceFlag, "since", "", "Start time for analysis (RFC3339 format)")
	analyzeCmd.Flags().StringVar(&untilFlag, "until", "", "End time for analysis (RFC3339 format)")
	analyzeCmd.Flags().StringArrayVar(&excludeAuthorFlags, "exclude-author", []string{}, "Exclude PRs by author (can be specified multiple times)")
	analyzeCmd.Flags().StringArrayVar(&excludeTitlePrefixes, "exclude-title-prefix", []string{}, "Exclude PRs by title prefix (can be specified multiple times)")
	analyzeCmd.Flags().StringVar(&outputFormatFlag, "output-format", "", "Output format (json, csv)")
	analyzeCmd.Flags().StringVar(&outputDirFlag, "output-dir", "", "Output directory")
	analyzeCmd.Flags().BoolVar(&skipAPICallsFlag, "skip-api-calls", false, "Skip API calls and use cache only")
	analyzeCmd.Flags().BoolVar(&invalidateCacheFlag, "invalidate-cache", false, "Invalidate cache before analysis")
	analyzeCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Dry run mode (don't make API calls)")

	// Bind flags to viper
	viper.BindPFlag("github.org", analyzeCmd.Flags().Lookup("org"))
	viper.BindPFlag("time_window.since", analyzeCmd.Flags().Lookup("since"))
	viper.BindPFlag("time_window.until", analyzeCmd.Flags().Lookup("until"))
	viper.BindPFlag("filters.exclude_authors", analyzeCmd.Flags().Lookup("exclude-author"))
	viper.BindPFlag("filters.exclude_title_prefixes", analyzeCmd.Flags().Lookup("exclude-title-prefix"))
	viper.BindPFlag("output.format", analyzeCmd.Flags().Lookup("output-format"))
	viper.BindPFlag("output.output_dir", analyzeCmd.Flags().Lookup("output-dir"))
}

func analyze(cmdCtx context.Context) error {
	logger.Info("Starting PR analysis")

	// Load configuration
	cfg, err := config.LoadConfig(cfgFile, logger)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override with CLI flags if provided
	if orgFlag != "" {
		cfg.GitHub.Org = orgFlag
	}
	if sinceFlag != "" {
		cfg.TimeWindow.Since = sinceFlag
	}
	if untilFlag != "" {
		cfg.TimeWindow.Until = untilFlag
	}
	if len(excludeAuthorFlags) > 0 {
		cfg.Filters.ExcludeAuthors = excludeAuthorFlags
	}
	if len(excludeTitlePrefixes) > 0 {
		cfg.Filters.ExcludeTitlePrefixes = excludeTitlePrefixes
	}
	if outputFormatFlag != "" {
		cfg.Output.Format = outputFormatFlag
	}
	if outputDirFlag != "" {
		cfg.Output.OutputDir = outputDirFlag
	}

	// Get GitHub token
	token, err := cfg.GetToken()
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Create GitHub client
	ghClient, err := ghclient.NewClient(
		token,
		cfg.RateLimiter.QPS,
		cfg.RateLimiter.Burst,
		cfg.RateLimiter.Retry.MaxAttempts,
		cfg.RateLimiter.Retry.BaseDelayMs,
		cfg.RateLimiter.Threshold,
		cfg.RateLimiter.SleepMinutes,
		logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create GitHub client: %w", err)
	}

	// Handle cache invalidation
	if invalidateCacheFlag {
		if cfg.Cache.Backend == "" {
			return fmt.Errorf("cache backend not configured, cannot invalidate")
		}
		cacheInstance, err := cache.NewCache(
			cfg.Cache.Backend,
			cfg.Cache.SQLitePath,
			cfg.Cache.JSONDir,
			logger,
		)
		if err != nil {
			return fmt.Errorf("failed to initialize cache: %w", err)
		}
		defer cacheInstance.Close()

		if err := cacheInstance.Invalidate(context.Background()); err != nil {
			return fmt.Errorf("failed to invalidate cache: %w", err)
		}
		logger.Info("Cache invalidated successfully")
		return nil
	}

	// Create analyzer
	analyzer, err := analyzer.NewAnalyzer(cfg, ghClient, skipAPICallsFlag, logger)
	if err != nil {
		return fmt.Errorf("failed to create analyzer: %w", err)
	}

	// Handle dry run
	if dryRunFlag {
		logger.Info("Dry run mode - skipping analysis")
		return nil
	}

	// Run analysis
	if err := analyzer.Analyze(cmdCtx); err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	logger.Info("Analysis complete")
	return nil
}
