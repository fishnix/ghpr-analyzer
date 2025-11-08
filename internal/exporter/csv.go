package exporter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"go.uber.org/zap"
)

// CSVExporter exports analysis results to CSV format
type CSVExporter struct {
	outputDir string
	logger    *zap.Logger
}

// NewCSVExporter creates a new CSV exporter
func NewCSVExporter(outputDir string, logger *zap.Logger) *CSVExporter {
	return &CSVExporter{
		outputDir: outputDir,
		logger:    logger,
	}
}

// Export exports the analysis results to CSV
func (e *CSVExporter) Export(result *AnalysisResult) error {
	e.logger.Info("Exporting results to CSV", zap.String("output_dir", e.outputDir))

	// Export aggregated results
	if err := e.exportAggregated(result); err != nil {
		return fmt.Errorf("failed to export aggregated results: %w", err)
	}

	// Export by team
	if err := e.exportByTeam(result); err != nil {
		return fmt.Errorf("failed to export by team: %w", err)
	}

	// Export by repo
	if err := e.exportByRepo(result); err != nil {
		return fmt.Errorf("failed to export by repo: %w", err)
	}

	// Export by user
	if err := e.exportByUser(result); err != nil {
		return fmt.Errorf("failed to export by user: %w", err)
	}

	e.logger.Info("CSV export complete")
	return nil
}

// exportAggregated exports aggregated summary
func (e *CSVExporter) exportAggregated(result *AnalysisResult) error {
	outputPath := filepath.Join(e.outputDir, "summary.csv")

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"Metric", "Value"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data
	records := [][]string{
		{"Total PRs Closed", strconv.Itoa(result.TotalPRsClosed)},
		{"Total Repos", strconv.Itoa(len(result.PRsByRepo))},
		{"Total Teams", strconv.Itoa(len(result.PRsByTeam))},
		{"Total Users", strconv.Itoa(len(result.PRsByUser))},
		{"Time Window Start", result.TimeWindow.Since.Format(time.RFC3339)},
		{"Time Window End", result.TimeWindow.Until.Format(time.RFC3339)},
		{"Generated At", result.GeneratedAt.Format(time.RFC3339)},
	}

	for _, record := range records {
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	e.logger.Debug("Exported aggregated summary", zap.String("path", outputPath))
	return nil
}

// exportByTeam exports PRs by team
func (e *CSVExporter) exportByTeam(result *AnalysisResult) error {
	outputPath := filepath.Join(e.outputDir, "prs_by_team.csv")

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"Team", "PR Count"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Sort teams by PR count (descending)
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

	// Write data
	for _, tc := range teams {
		record := []string{tc.team, strconv.Itoa(tc.count)}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	e.logger.Debug("Exported PRs by team", zap.String("path", outputPath))
	return nil
}

// exportByRepo exports PRs by repository
func (e *CSVExporter) exportByRepo(result *AnalysisResult) error {
	outputPath := filepath.Join(e.outputDir, "prs_by_repo.csv")

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"Repository", "PR Count"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Sort repos by PR count (descending)
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

	// Write data
	for _, rc := range repos {
		record := []string{rc.repo, strconv.Itoa(rc.count)}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	e.logger.Debug("Exported PRs by repo", zap.String("path", outputPath))
	return nil
}

// exportByUser exports PRs by user
func (e *CSVExporter) exportByUser(result *AnalysisResult) error {
	outputPath := filepath.Join(e.outputDir, "prs_by_user.csv")

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"User", "PR Count"}); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Sort users by PR count (descending)
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

	// Write data
	for _, uc := range users {
		record := []string{uc.user, strconv.Itoa(uc.count)}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}

	e.logger.Debug("Exported PRs by user", zap.String("path", outputPath))
	return nil
}

