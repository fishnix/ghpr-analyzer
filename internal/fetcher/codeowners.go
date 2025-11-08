package fetcher

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-github/v62/github"
	"github.com/fishnix/golang-template/internal/ghclient"
	"go.uber.org/zap"
)

// CODEOWNERSFetcher fetches CODEOWNERS files from repositories
type CODEOWNERSFetcher struct {
	client   *github.Client
	ghClient *ghclient.Client
	logger   *zap.Logger
}

// NewCODEOWNERSFetcher creates a new CODEOWNERS fetcher
func NewCODEOWNERSFetcher(client *github.Client, ghClient *ghclient.Client, logger *zap.Logger) *CODEOWNERSFetcher {
	return &CODEOWNERSFetcher{
		client:   client,
		ghClient: ghClient,
		logger:   logger,
	}
}

// CODEOWNERSFile represents a parsed CODEOWNERS file
type CODEOWNERSFile struct {
	Rules []CODEOWNERSRule
	Path  string
}

// CODEOWNERSRule represents a single CODEOWNERS rule
type CODEOWNERSRule struct {
	Pattern string
	Owners  []string
	LineNum int
}

// FetchCODEOWNERS fetches and parses CODEOWNERS file from a repository
// It checks both repo root and .github/ directory
// Returns both the parsed file and raw content for caching
func (c *CODEOWNERSFetcher) FetchCODEOWNERS(ctx context.Context, owner, repo string) (*CODEOWNERSFile, []byte, error) {
	// Try common CODEOWNERS locations
	paths := []string{
		"CODEOWNERS",
		".github/CODEOWNERS",
		"docs/CODEOWNERS",
	}

	for _, path := range paths {
		content, err := c.fetchFileContent(ctx, owner, repo, path)
		if err != nil {
			// File not found, try next location
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return nil, nil, fmt.Errorf("failed to fetch CODEOWNERS from %s: %w", path, err)
		}

		if content != nil {
			parsed, err := c.parseCODEOWNERS(content, path)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse CODEOWNERS from %s: %w", path, err)
			}
			c.logger.Debug("Found CODEOWNERS file",
				zap.String("repo", fmt.Sprintf("%s/%s", owner, repo)),
				zap.String("path", path),
				zap.Int("rules", len(parsed.Rules)),
			)
			return parsed, content, nil
		}
	}

	// No CODEOWNERS file found
	c.logger.Debug("No CODEOWNERS file found",
		zap.String("repo", fmt.Sprintf("%s/%s", owner, repo)),
	)
	return nil, nil, nil
}

// fetchFileContent fetches file content from GitHub
func (c *CODEOWNERSFetcher) fetchFileContent(ctx context.Context, owner, repo, path string) ([]byte, error) {
	fileContent, _, resp, err := c.client.Repositories.GetContents(ctx, owner, repo, path, &github.RepositoryContentGetOptions{})
	if err != nil {
		return nil, err
	}

	// Check rate limit and sleep if threshold is reached
	if c.ghClient != nil && resp != nil {
		if err := c.ghClient.CheckAndSleepIfNeeded(ctx, resp); err != nil {
			return nil, fmt.Errorf("rate limit check failed: %w", err)
		}
	}

	// Check if it's a file (not a directory)
	if resp.StatusCode == 200 && fileContent != nil {
		content, err := fileContent.GetContent()
		if err != nil {
			return nil, fmt.Errorf("failed to decode file content: %w", err)
		}
		return []byte(content), nil
	}

	return nil, fmt.Errorf("file not found or is a directory")
}

// ParseCODEOWNERS parses CODEOWNERS file content (public method for cache)
func (c *CODEOWNERSFetcher) ParseCODEOWNERS(content []byte, path string) (*CODEOWNERSFile, error) {
	return c.parseCODEOWNERS(content, path)
}

// parseCODEOWNERS parses CODEOWNERS file content
func (c *CODEOWNERSFetcher) parseCODEOWNERS(content []byte, path string) (*CODEOWNERSFile, error) {
	file := &CODEOWNERSFile{
		Rules: []CODEOWNERSRule{},
		Path:  path,
	}

	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		lineNum := i + 1
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line: pattern owner1 owner2 ...
		parts := strings.Fields(line)
		if len(parts) < 2 {
			// Invalid line, skip
			continue
		}

		pattern := parts[0]
		owners := parts[1:]

		// Normalize pattern (handle leading slash)
		if !strings.HasPrefix(pattern, "/") {
			pattern = "/" + pattern
		}

		file.Rules = append(file.Rules, CODEOWNERSRule{
			Pattern: pattern,
			Owners:  owners,
			LineNum: lineNum,
		})
	}

	return file, nil
}

// FindOwners finds owners for a given file path using CODEOWNERS rules
// Returns owners in order of specificity (most specific first)
func (file *CODEOWNERSFile) FindOwners(filePath string) []string {
	if file == nil || len(file.Rules) == 0 {
		return nil
	}

	// Normalize file path
	if !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	filePath = filepath.Clean(filePath)

	var matches []struct {
		owners  []string
		specificity int
	}

	// Find all matching rules
	for _, rule := range file.Rules {
		if matchesPattern(rule.Pattern, filePath) {
			// Calculate specificity (longer pattern = more specific)
			specificity := len(rule.Pattern)
			matches = append(matches, struct {
				owners  []string
				specificity int
			}{
				owners:  rule.Owners,
				specificity: specificity,
			})
		}
	}

	if len(matches) == 0 {
		return nil
	}

	// Sort by specificity (most specific first)
	// Simple bubble sort for small lists
	for i := 0; i < len(matches)-1; i++ {
		for j := i + 1; j < len(matches); j++ {
			if matches[j].specificity > matches[i].specificity {
				matches[i], matches[j] = matches[j], matches[i]
			}
		}
	}

	// Return owners from most specific match
	return matches[0].owners
}

// matchesPattern checks if a file path matches a CODEOWNERS pattern
// Supports gitignore-like patterns
func matchesPattern(pattern, filePath string) bool {
	// Normalize pattern and path
	pattern = filepath.Clean(pattern)
	filePath = filepath.Clean(filePath)

	// Remove leading slash for comparison
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
	}
	if strings.HasPrefix(filePath, "/") {
		filePath = filePath[1:]
	}

	// Handle exact match
	if pattern == filePath {
		return true
	}

	// Handle directory match (pattern ends with /)
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
		return strings.HasPrefix(filePath, pattern+"/") || filePath == pattern
	}

	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		return matchWildcard(pattern, filePath)
	}

	// Handle prefix match (pattern matches directory or file)
	if strings.HasPrefix(filePath, pattern+"/") || filePath == pattern {
		return true
	}

	return false
}

// matchWildcard matches wildcard patterns
func matchWildcard(pattern, filePath string) bool {
	// Handle ** (match any directory)
	if strings.Contains(pattern, "**") {
		return matchDoubleStar(pattern, filePath)
	}

	// Handle single * (match any characters except /)
	return matchSingleStar(pattern, filePath)
}

// matchDoubleStar handles ** patterns (match any directory)
func matchDoubleStar(pattern, filePath string) bool {
	// Replace ** with a placeholder for easier matching
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		// Multiple **, use simple approach
		regexPattern := strings.ReplaceAll(pattern, "**", ".*")
		return matchRegexLike(regexPattern, filePath)
	}

	prefix := parts[0]
	suffix := parts[1]

	// Remove trailing / from prefix if present
	prefix = strings.TrimSuffix(prefix, "/")
	suffix = strings.TrimPrefix(suffix, "/")

	// If prefix is empty, check suffix
	if prefix == "" {
		return strings.HasSuffix(filePath, suffix) || suffix == ""
	}

	// If suffix is empty, check prefix
	if suffix == "" {
		return strings.HasPrefix(filePath, prefix) || prefix == ""
	}

	// Find prefix in path
	prefixIdx := strings.Index(filePath, prefix)
	if prefixIdx == -1 {
		return false
	}

	// Check if suffix exists after prefix
	remaining := filePath[prefixIdx+len(prefix):]
	return strings.Contains(remaining, suffix)
}

// matchSingleStar handles * patterns (match any characters except /)
func matchSingleStar(pattern, filePath string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) < 2 {
		return pattern == filePath
	}

	// Build regex-like pattern
	var regexParts []string
	for i, part := range parts {
		if part != "" {
			regexParts = append(regexParts, regexp.QuoteMeta(part))
		}
		if i < len(parts)-1 {
			// Add [^/]* between parts (match any non-slash characters)
			regexParts = append(regexParts, "[^/]*")
		}
	}

	regexPattern := "^" + strings.Join(regexParts, "") + "$"
	matched, err := regexp.MatchString(regexPattern, filePath)
	return err == nil && matched
}

// matchRegexLike performs simple regex-like matching
func matchRegexLike(pattern, filePath string) bool {
	matched, err := regexp.MatchString("^"+pattern+"$", filePath)
	return err == nil && matched
}

