package exporter

import (
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"
)

// SummaryExporter exports human-readable summary
type SummaryExporter struct {
	logger *zap.Logger
}

// NewSummaryExporter creates a new summary exporter
func NewSummaryExporter(logger *zap.Logger) *SummaryExporter {
	return &SummaryExporter{
		logger: logger,
	}
}

// Export exports a human-readable summary to stdout
func (e *SummaryExporter) Export(result *AnalysisResult) error {
	e.logger.Info("Exporting human-readable summary")

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("GitHub PR Analysis Summary")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("\nTime Window: %s to %s\n", result.TimeWindow.Since.Format("2006-01-02"), result.TimeWindow.Until.Format("2006-01-02"))
	fmt.Printf("Generated At: %s\n", result.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Total PRs
	fmt.Printf("Total PRs Closed: %d\n", result.TotalPRsClosed)
	fmt.Println()

	// Top repositories
	fmt.Println("Top Repositories by PR Count:")
	fmt.Println(strings.Repeat("-", 80))
	type repoCount struct {
		repo  string
		count int
	}
	var repos []repoCount
	for repo, count := range result.PRsByRepo {
		repos = append(repos, repoCount{repo: repo, count: count})
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].count > repos[j].count
	})
	for i, rc := range repos {
		if i >= 10 {
			break
		}
		fmt.Printf("  %-50s %5d\n", rc.repo, rc.count)
	}
	fmt.Println()

	// Top teams
	fmt.Println("Top Teams by PR Count:")
	fmt.Println(strings.Repeat("-", 80))
	type teamCount struct {
		team  string
		count int
	}
	var teams []teamCount
	for team, count := range result.PRsByTeam {
		teams = append(teams, teamCount{team: team, count: count})
	}
	sort.Slice(teams, func(i, j int) bool {
		return teams[i].count > teams[j].count
	})
	for i, tc := range teams {
		if i >= 10 {
			break
		}
		fmt.Printf("  %-50s %5d\n", tc.team, tc.count)
	}
	fmt.Println()

	// Top users
	fmt.Println("Top Users by PR Count:")
	fmt.Println(strings.Repeat("-", 80))
	type userCount struct {
		user  string
		count int
	}
	var users []userCount
	for user, count := range result.PRsByUser {
		users = append(users, userCount{user: user, count: count})
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].count > users[j].count
	})
	for i, uc := range users {
		if i >= 10 {
			break
		}
		fmt.Printf("  %-50s %5d\n", uc.user, uc.count)
	}
	fmt.Println()

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	return nil
}

