package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Config holds the application configuration
type Config struct {
	GitHub       GitHubConfig       `mapstructure:"github"`
	TimeWindow   TimeWindowConfig   `mapstructure:"time_window"`
	Filters      FiltersConfig      `mapstructure:"filters"`
	Attribution  AttributionConfig  `mapstructure:"attribution"`
	Cache        CacheConfig        `mapstructure:"cache"`
	RateLimiter  RateLimiterConfig  `mapstructure:"rate_limiter"`
	Output       OutputConfig       `mapstructure:"output"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	Concurrency  ConcurrencyConfig  `mapstructure:"concurrency"`
}

// GitHubConfig holds GitHub API configuration
type GitHubConfig struct {
	Org         string `mapstructure:"org"`
	TokenEnvVar string `mapstructure:"token_env_var"`
}

// TimeWindowConfig holds the time window for PR analysis
type TimeWindowConfig struct {
	Since string `mapstructure:"since"`
	Until string `mapstructure:"until"`
}

// FiltersConfig holds filter configuration
type FiltersConfig struct {
	ExcludeAuthors      []string `mapstructure:"exclude_authors"`
	ExcludeTitlePrefixes []string `mapstructure:"exclude_title_prefixes"`
}

// AttributionConfig holds attribution mode configuration
type AttributionConfig struct {
	Mode string `mapstructure:"mode"` // "multi" | "primary" | "first-owner-only"
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Backend     string `mapstructure:"backend"` // "sqlite" | "json"
	SQLitePath  string `mapstructure:"sqlite_path"`
	JSONDir     string `mapstructure:"json_dir"`
	TTLMinutes  int    `mapstructure:"ttl_minutes"`
}

// RateLimiterConfig holds rate limiter configuration
type RateLimiterConfig struct {
	Type        string `mapstructure:"type"` // "token-bucket"
	QPS         int    `mapstructure:"qps"`
	Burst       int    `mapstructure:"burst"`
	Retry       RetryConfig `mapstructure:"retry"`
	Threshold   int    `mapstructure:"threshold"`   // Rate limit threshold to trigger sleep
	SleepMinutes int   `mapstructure:"sleep_minutes"` // Minutes to sleep when threshold is reached
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxAttempts int `mapstructure:"max_attempts"`
	BaseDelayMs int `mapstructure:"base_delay_ms"`
}

// OutputConfig holds output configuration
type OutputConfig struct {
	Format    string `mapstructure:"format"` // "json" | "csv"
	OutputDir string `mapstructure:"output_dir"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level string `mapstructure:"level"` // "debug" | "info" | "warn" | "error"
}

// ConcurrencyConfig holds concurrency configuration
type ConcurrencyConfig struct {
	RepoWorkers int `mapstructure:"repo_workers"`
}

// LoadConfig loads configuration from file and environment
func LoadConfig(configPath string, logger *zap.Logger) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			logger.Warn("Failed to read config file, using defaults", zap.Error(err))
		} else {
			logger.Info("Using config file", zap.String("path", v.ConfigFileUsed()))
		}
	}

	// Bind environment variables
	v.SetEnvPrefix("ANALYZER")
	v.AutomaticEnv()

	// Note: CLI flags are bound in cmd package

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate and set defaults
	if err := validateAndSetDefaults(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// GitHub defaults
	v.SetDefault("github.token_env_var", "GITHUB_TOKEN")

	// Attribution defaults
	v.SetDefault("attribution.mode", "multi")

	// Cache defaults
	v.SetDefault("cache.backend", "sqlite")
	v.SetDefault("cache.sqlite_path", "./cache.db")
	v.SetDefault("cache.json_dir", "./cache")
	v.SetDefault("cache.ttl_minutes", 1440)

	// Rate limiter defaults
	v.SetDefault("rate_limiter.type", "token-bucket")
	v.SetDefault("rate_limiter.qps", 2)
	v.SetDefault("rate_limiter.burst", 20)
	v.SetDefault("rate_limiter.retry.max_attempts", 5)
	v.SetDefault("rate_limiter.retry.base_delay_ms", 500)
	v.SetDefault("rate_limiter.threshold", 0)        // 0 = disabled
	v.SetDefault("rate_limiter.sleep_minutes", 60)   // Default 60 minutes

	// Output defaults
	v.SetDefault("output.format", "json")
	v.SetDefault("output.output_dir", "./out")

	// Logging defaults
	v.SetDefault("logging.level", "info")

	// Concurrency defaults
	v.SetDefault("concurrency.repo_workers", 8)
}

func validateAndSetDefaults(cfg *Config) error {
	// Validate GitHub org
	if cfg.GitHub.Org == "" {
		return fmt.Errorf("github.org is required")
	}

	// Validate time window
	if cfg.TimeWindow.Since == "" {
		return fmt.Errorf("time_window.since is required")
	}
	if cfg.TimeWindow.Until == "" {
		return fmt.Errorf("time_window.until is required")
	}

	// Validate time format
	if _, err := time.Parse(time.RFC3339, cfg.TimeWindow.Since); err != nil {
		return fmt.Errorf("invalid time_window.since format (must be RFC3339): %w", err)
	}
	if _, err := time.Parse(time.RFC3339, cfg.TimeWindow.Until); err != nil {
		return fmt.Errorf("invalid time_window.until format (must be RFC3339): %w", err)
	}

	// Validate attribution mode
	validModes := map[string]bool{"multi": true, "primary": true, "first-owner-only": true}
	if !validModes[cfg.Attribution.Mode] {
		cfg.Attribution.Mode = "multi"
	}

	// Validate output format
	validFormats := map[string]bool{"json": true, "csv": true}
	if !validFormats[cfg.Output.Format] {
		cfg.Output.Format = "json"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(cfg.Output.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	return nil
}

// GetToken retrieves the GitHub token from environment
func (c *Config) GetToken() (string, error) {
	token := os.Getenv(c.GitHub.TokenEnvVar)
	if token == "" {
		return "", fmt.Errorf("GitHub token not found in environment variable %s", c.GitHub.TokenEnvVar)
	}
	return token, nil
}

// GetTimeWindow returns parsed time window
func (c *Config) GetTimeWindow() (time.Time, time.Time, error) {
	since, err := time.Parse(time.RFC3339, c.TimeWindow.Since)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid since time: %w", err)
	}

	until, err := time.Parse(time.RFC3339, c.TimeWindow.Until)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid until time: %w", err)
	}

	return since, until, nil
}

