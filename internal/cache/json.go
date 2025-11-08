package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// JSONCache implements cache using JSON files
type JSONCache struct {
	baseDir string
	logger  *zap.Logger
	ttl     time.Duration
}

// NewJSONCache creates a new JSON file cache
func NewJSONCache(baseDir string, logger *zap.Logger) (*JSONCache, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &JSONCache{
		baseDir: baseDir,
		logger:  logger,
		ttl:     24 * time.Hour, // Default TTL
	}, nil
}

// GetRepos retrieves cached repositories
func (c *JSONCache) GetRepos(ctx context.Context, org string) ([]*github.Repository, error) {
	path := filepath.Join(c.baseDir, "orgs", org, "repos.json")
	var repos []*github.Repository
	err := c.getJSON(path, &repos)
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// SetRepos caches repositories
func (c *JSONCache) SetRepos(ctx context.Context, org string, repos []*github.Repository) error {
	path := filepath.Join(c.baseDir, "orgs", org, "repos.json")
	return c.setJSON(path, repos)
}

// GetCODEOWNERS retrieves cached CODEOWNERS file
func (c *JSONCache) GetCODEOWNERS(ctx context.Context, owner, repo string) ([]byte, error) {
	path := filepath.Join(c.baseDir, "repos", owner, repo, "codeowners.json")
	var content []byte
	err := c.getJSON(path, &content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

// SetCODEOWNERS caches CODEOWNERS file
func (c *JSONCache) SetCODEOWNERS(ctx context.Context, owner, repo string, content []byte) error {
	path := filepath.Join(c.baseDir, "repos", owner, repo, "codeowners.json")
	return c.setJSON(path, content)
}

// GetPRs retrieves cached PRs for a repository
func (c *JSONCache) GetPRs(ctx context.Context, owner, repo string, since, until time.Time) ([]*github.PullRequest, error) {
	// Create a cache key based on time window
	key := fmt.Sprintf("prs_%s_%s.json", since.Format("20060102"), until.Format("20060102"))
	path := filepath.Join(c.baseDir, "repos", owner, repo, key)
	var prs []*github.PullRequest
	err := c.getJSON(path, &prs)
	if err != nil {
		return nil, err
	}
	return prs, nil
}

// SetPRs caches PRs for a repository
func (c *JSONCache) SetPRs(ctx context.Context, owner, repo string, since, until time.Time, prs []*github.PullRequest) error {
	key := fmt.Sprintf("prs_%s_%s.json", since.Format("20060102"), until.Format("20060102"))
	path := filepath.Join(c.baseDir, "repos", owner, repo, key)
	return c.setJSON(path, prs)
}

// GetPRFiles retrieves cached PR files
func (c *JSONCache) GetPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]*github.CommitFile, error) {
	path := filepath.Join(c.baseDir, "repos", owner, repo, "prs", fmt.Sprintf("%d_files.json", prNumber))
	var files []*github.CommitFile
	err := c.getJSON(path, &files)
	if err != nil {
		return nil, err
	}
	return files, nil
}

// SetPRFiles caches PR files
func (c *JSONCache) SetPRFiles(ctx context.Context, owner, repo string, prNumber int, files []*github.CommitFile) error {
	path := filepath.Join(c.baseDir, "repos", owner, repo, "prs", fmt.Sprintf("%d_files.json", prNumber))
	return c.setJSON(path, files)
}

// Invalidate invalidates all cache entries
func (c *JSONCache) Invalidate(ctx context.Context) error {
	return os.RemoveAll(c.baseDir)
}

// InvalidateRepo invalidates cache for a specific repository
func (c *JSONCache) InvalidateRepo(ctx context.Context, owner, repo string) error {
	path := filepath.Join(c.baseDir, "repos", owner, repo)
	return os.RemoveAll(path)
}

// Close closes the cache
func (c *JSONCache) Close() error {
	return nil
}

// getJSON retrieves JSON data from cache
func (c *JSONCache) getJSON(path string, result interface{}) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("cache entry not found")
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	// Unmarshal entry
	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return fmt.Errorf("failed to unmarshal cache entry: %w", err)
	}

	// Check expiration
	if entry.IsExpired(c.ttl) {
		c.logger.Debug("Cache entry expired", zap.String("path", path))
		return fmt.Errorf("cache entry expired")
	}

	// Unmarshal data
	dataBytes, err := json.Marshal(entry.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	if err := json.Unmarshal(dataBytes, result); err != nil {
		return fmt.Errorf("failed to unmarshal cache data: %w", err)
	}

	return nil
}

// setJSON stores JSON data in cache
func (c *JSONCache) setJSON(path string, data interface{}) error {
	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create cache entry
	entry := CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
	}

	// Marshal entry
	jsonData, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

