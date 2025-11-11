package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// Cache interface for different cache backends
type Cache interface {
	// GetRepos retrieves cached repositories
	GetRepos(ctx context.Context, org string) ([]*github.Repository, error)
	// SetRepos caches repositories
	SetRepos(ctx context.Context, org string, repos []*github.Repository) error

	// GetCODEOWNERS retrieves cached CODEOWNERS file
	GetCODEOWNERS(ctx context.Context, owner, repo string) ([]byte, error)
	// SetCODEOWNERS caches CODEOWNERS file
	SetCODEOWNERS(ctx context.Context, owner, repo string, content []byte) error

	// GetPRs retrieves cached PRs for a repository, filtered by time window
	GetPRs(ctx context.Context, owner, repo string, since, until time.Time) ([]*github.PullRequest, error)
	// SetPRs caches PRs for a repository (stores individual PRs by ID)
	SetPRs(ctx context.Context, owner, repo string, prs []*github.PullRequest) error

	// GetPRFiles retrieves cached PR files
	GetPRFiles(ctx context.Context, owner, repo string, prNumber int) ([]*github.CommitFile, error)
	// SetPRFiles caches PR files
	SetPRFiles(ctx context.Context, owner, repo string, prNumber int, files []*github.CommitFile) error

	// Invalidate invalidates all cache entries
	Invalidate(ctx context.Context) error
	// InvalidateRepo invalidates cache for a specific repository
	InvalidateRepo(ctx context.Context, owner, repo string) error

	// Close closes the cache
	Close() error
}

// NewCache creates a new cache instance based on backend type
func NewCache(backend, sqlitePath, jsonDir string, ttl time.Duration, ignoreTTL bool, logger *zap.Logger) (Cache, error) {
	switch backend {
	case "sqlite":
		return NewSQLiteCache(sqlitePath, ttl, ignoreTTL, logger)
	case "json":
		return NewJSONCache(jsonDir, ttl, ignoreTTL, logger)
	default:
		return nil, fmt.Errorf("unsupported cache backend: %s", backend)
	}
}

// CacheEntry represents a cached entry with metadata
type CacheEntry struct {
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
	TTL       time.Duration
}

// IsExpired checks if a cache entry is expired
func (e *CacheEntry) IsExpired(ttl time.Duration) bool {
	if ttl == 0 {
		return false // No expiration
	}
	return time.Since(e.Timestamp) > ttl
}

