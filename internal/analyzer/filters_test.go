package analyzer

import (
	"testing"

	"github.com/fishnix/ghpr-analyzer/internal/config"
	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

func TestApplyFilters(t *testing.T) {
	cfg := &config.Config{
		Filters: config.FiltersConfig{
			ExcludeAuthors:       []string{"bot", "dependabot"},
			ExcludeTitlePrefixes: []string{"WIP:", "DO NOT MERGE"},
		},
	}

	logger := zap.NewNop()
	analyzer := &Analyzer{
		cfg:    cfg,
		logger: logger,
	}

	prs := []*github.PullRequest{
		{
			Number: github.Int(1),
			Title:  github.String("Normal PR"),
			User: &github.User{
				Login: github.String("user1"),
			},
		},
		{
			Number: github.Int(2),
			Title:  github.String("WIP: Work in progress"),
			User: &github.User{
				Login: github.String("user2"),
			},
		},
		{
			Number: github.Int(3),
			Title:  github.String("Another PR"),
			User: &github.User{
				Login: github.String("bot"),
			},
		},
		{
			Number: github.Int(4),
			Title:  github.String("DO NOT MERGE: Test PR"),
			User: &github.User{
				Login: github.String("user3"),
			},
		},
	}

	filtered := analyzer.applyFilters(prs)

	if len(filtered) != 1 {
		t.Fatalf("Expected 1 PR after filtering, got %d", len(filtered))
	}

	if filtered[0].GetNumber() != 1 {
		t.Errorf("Expected PR #1, got PR #%d", filtered[0].GetNumber())
	}
}
