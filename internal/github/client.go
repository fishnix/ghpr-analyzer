package github

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

// Client wraps the GitHub API client with rate limiting and retries
type Client struct {
	client     *github.Client
	limiter    *rate.Limiter
	logger     *zap.Logger
	maxRetries int
	baseDelay  time.Duration
}

// NewClient creates a new GitHub client with rate limiting
func NewClient(token string, qps int, burst int, maxRetries int, baseDelayMs int, logger *zap.Logger) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	// Create rate limiter
	// qps is requests per second, so we need to convert to rate.Limit
	limiter := rate.NewLimiter(rate.Limit(qps), burst)

	client := github.NewClient(tc)

	return &Client{
		client:     client,
		limiter:    limiter,
		logger:     logger,
		maxRetries: maxRetries,
		baseDelay:  time.Duration(baseDelayMs) * time.Millisecond,
	}, nil
}

// GetClient returns the underlying GitHub client
func (c *Client) GetClient() *github.Client {
	return c.client
}

// WaitForRateLimit waits for the rate limiter
func (c *Client) WaitForRateLimit(ctx context.Context) error {
	return c.limiter.Wait(ctx)
}

// CheckRateLimit checks the current rate limit status
func (c *Client) CheckRateLimit(ctx context.Context) (*github.RateLimits, *github.Response, error) {
	limits, resp, err := c.client.RateLimits(ctx)
	if err != nil {
		return nil, resp, err
	}
	return limits, resp, nil
}

// RetryWithBackoff executes a function with exponential backoff retry
func (c *Client) RetryWithBackoff(ctx context.Context, fn func() (*github.Response, error)) (*github.Response, error) {
	var lastErr error
	var lastResp *github.Response

	for attempt := 0; attempt < c.maxRetries; attempt++ {
		// Wait for rate limiter
		if err := c.WaitForRateLimit(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter wait failed: %w", err)
		}

		resp, err := fn()
		if err == nil {
			// Check rate limit headers
			if resp != nil {
				if resp.Rate.Remaining == 0 {
					resetTime := resp.Rate.Reset.Time
					waitTime := time.Until(resetTime)
					if waitTime > 0 {
						c.logger.Warn("Rate limit exhausted, waiting",
							zap.Time("reset_time", resetTime),
							zap.Duration("wait_time", waitTime),
						)
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-time.After(waitTime):
						}
					}
				}
			}
			return resp, nil
		}

		lastErr = err
		lastResp = resp

		// Check if it's a retryable error
		if resp != nil {
			statusCode := resp.StatusCode
			if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
				// Calculate backoff delay with jitter
				delay := c.calculateBackoff(attempt)
				c.logger.Warn("Retryable error, backing off",
					zap.Int("attempt", attempt+1),
					zap.Int("status_code", statusCode),
					zap.Duration("delay", delay),
					zap.Error(err),
				)

				// If rate limited, wait for reset time
				if statusCode == http.StatusTooManyRequests {
					if resetTime := resp.Rate.Reset.Time; !resetTime.IsZero() {
						waitTime := time.Until(resetTime)
						if waitTime > 0 {
							delay = waitTime
						}
					}
				}

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
		}

		// Non-retryable error
		return resp, err
	}

	return lastResp, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	delay := float64(c.baseDelay) * math.Pow(2, float64(attempt))
	jitter := time.Duration(float64(delay) * 0.1) // 10% jitter
	return time.Duration(delay) + jitter
}
