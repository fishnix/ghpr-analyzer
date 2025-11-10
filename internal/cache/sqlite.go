package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// SQLiteCache implements cache using SQLite
type SQLiteCache struct {
	db     *sql.DB
	logger *zap.Logger
	ttl    time.Duration
}

// NewSQLiteCache creates a new SQLite cache
func NewSQLiteCache(dbPath string, logger *zap.Logger) (*SQLiteCache, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	cache := &SQLiteCache{
		db:     db,
		logger: logger,
		ttl:    24 * time.Hour, // Default TTL
	}

	// Initialize schema
	if err := cache.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return cache, nil
}

// initSchema initializes the database schema
func (c *SQLiteCache) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS repos (
		org TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (org)
	);
	
	CREATE TABLE IF NOT EXISTS codeowners (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo)
	);
	
	CREATE TABLE IF NOT EXISTS prs (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		since TEXT NOT NULL,
		until TEXT NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo, since, until)
	);
	
	CREATE TABLE IF NOT EXISTS pr_files (
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		data BLOB NOT NULL,
		timestamp DATETIME NOT NULL,
		PRIMARY KEY (owner, repo, pr_number)
	);
	`

	_, err := c.db.Exec(schema)
	return err
}

// GetRepos retrieves cached repositories
func (c *SQLiteCache) GetRepos(ctx context.Context, org string) ([]*github.Repository, error) {
	var data []byte
	var timestamp time.Time

	c.logger.Debug("Getting cached repositories", zap.String("org", org))

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM repos WHERE org = ?",
		org,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		c.logger.Debug("Cache entry not found", zap.String("org", org))
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration
	entry := CacheEntry{Timestamp: timestamp}
	if entry.IsExpired(c.ttl) {
		c.logger.Debug("Cache entry expired", zap.String("org", org))
		return nil, fmt.Errorf("cache entry expired")
	}

	// Unmarshal
	var repos []*github.Repository
	if err := json.Unmarshal(data, &repos); err != nil {
		c.logger.Debug("Failed to unmarshal data", zap.String("org", org), zap.Error(err))
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return repos, nil
}

// SetRepos caches repositories
func (c *SQLiteCache) SetRepos(ctx context.Context, org string, repos []*github.Repository) error {
	data, err := json.Marshal(repos)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO repos (org, data, timestamp) VALUES (?, ?, ?)`,
		org, data, time.Now(),
	)

	return err
}

// GetCODEOWNERS retrieves cached CODEOWNERS file
func (c *SQLiteCache) GetCODEOWNERS(ctx context.Context, owner, repo string) ([]byte, error) {
	var data []byte
	var timestamp time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM codeowners WHERE owner = ? AND repo = ?",
		owner, repo,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration
	entry := CacheEntry{Timestamp: timestamp}
	if entry.IsExpired(c.ttl) {
		return nil, fmt.Errorf("cache entry expired")
	}

	return data, nil
}

// SetCODEOWNERS caches CODEOWNERS file
func (c *SQLiteCache) SetCODEOWNERS(ctx context.Context, owner, repo string, content []byte) error {
	_, err := c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO codeowners (owner, repo, data, timestamp) VALUES (?, ?, ?, ?)`,
		owner, repo, content, time.Now(),
	)

	return err
}

// GetPRs retrieves cached PRs for a repository
func (c *SQLiteCache) GetPRs(ctx context.Context, owner, repo string, since, until time.Time) ([]*github.PullRequest, error) {
	sinceStr := since.Format(time.RFC3339)
	untilStr := until.Format(time.RFC3339)

	var data []byte
	var timestamp time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM prs WHERE owner = ? AND repo = ? AND since = ? AND until = ?",
		owner, repo, sinceStr, untilStr,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration
	entry := CacheEntry{Timestamp: timestamp}
	if entry.IsExpired(c.ttl) {
		return nil, fmt.Errorf("cache entry expired")
	}

	// Unmarshal
	var prs []*github.PullRequest
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return prs, nil
}

// SetPRs caches PRs for a repository
func (c *SQLiteCache) SetPRs(ctx context.Context, owner, repo string, since, until time.Time, prs []*github.PullRequest) error {
	data, err := json.Marshal(prs)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	sinceStr := since.Format(time.RFC3339)
	untilStr := until.Format(time.RFC3339)

	_, err = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO prs (owner, repo, since, until, data, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
		owner, repo, sinceStr, untilStr, data, time.Now(),
	)

	return err
}

// GetPRFiles retrieves cached PR files
func (c *SQLiteCache) GetPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]*github.CommitFile, error) {
	var data []byte
	var timestamp time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT data, timestamp FROM pr_files WHERE owner = ? AND repo = ? AND pr_number = ?",
		owner, repo, prNumber,
	).Scan(&data, &timestamp)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cache entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query cache: %w", err)
	}

	// Check expiration
	entry := CacheEntry{Timestamp: timestamp}
	if entry.IsExpired(c.ttl) {
		return nil, fmt.Errorf("cache entry expired")
	}

	// Unmarshal
	var files []*github.CommitFile
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return files, nil
}

// SetPRFiles caches PR files
func (c *SQLiteCache) SetPRFiles(ctx context.Context, owner, repo string, prNumber int, files []*github.CommitFile) error {
	data, err := json.Marshal(files)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO pr_files (owner, repo, pr_number, data, timestamp) VALUES (?, ?, ?, ?, ?)`,
		owner, repo, prNumber, data, time.Now(),
	)

	return err
}

// Invalidate invalidates all cache entries
func (c *SQLiteCache) Invalidate(ctx context.Context) error {
	tables := []string{"repos", "codeowners", "prs", "pr_files"}
	for _, table := range tables {
		if _, err := c.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to invalidate %s: %w", table, err)
		}
	}
	return nil
}

// InvalidateRepo invalidates cache for a specific repository
func (c *SQLiteCache) InvalidateRepo(ctx context.Context, owner, repo string) error {
	_, err := c.db.ExecContext(ctx,
		"DELETE FROM codeowners WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate codeowners: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		"DELETE FROM prs WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate prs: %w", err)
	}

	_, err = c.db.ExecContext(ctx,
		"DELETE FROM pr_files WHERE owner = ? AND repo = ?",
		owner, repo,
	)
	if err != nil {
		return fmt.Errorf("failed to invalidate pr_files: %w", err)
	}

	return nil
}

// Close closes the cache
func (c *SQLiteCache) Close() error {
	return c.db.Close()
}
