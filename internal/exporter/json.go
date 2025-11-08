package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/go-github/v62/github"
	"go.uber.org/zap"
)

// AnalysisResult represents the aggregated analysis results
type AnalysisResult struct {
	TotalPRsClosed int                    `json:"total_prs_closed"`
	PRsByRepo      map[string]int         `json:"prs_by_repo"`
	PRsByTeam      map[string]int         `json:"prs_by_team"`
	PRsByUser      map[string]int         `json:"prs_by_user"`
	TimeWindow     TimeWindow             `json:"time_window"`
	GeneratedAt    time.Time              `json:"generated_at"`
}

// TimeWindow represents the analysis time window
type TimeWindow struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// JSONExporter exports analysis results to JSON format
type JSONExporter struct {
	outputDir string
	logger    *zap.Logger
}

// NewJSONExporter creates a new JSON exporter
func NewJSONExporter(outputDir string, logger *zap.Logger) *JSONExporter {
	return &JSONExporter{
		outputDir: outputDir,
		logger:    logger,
	}
}

// Export exports the analysis results to JSON
func (e *JSONExporter) Export(result *AnalysisResult) error {
	e.logger.Info("Exporting results to JSON", zap.String("output_dir", e.outputDir))

	// Create output file path
	outputPath := filepath.Join(e.outputDir, "analysis_results.json")

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	e.logger.Info("JSON export complete", zap.String("path", outputPath))
	return nil
}

// RepoPR represents a PR for per-repo export
type RepoPR struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	ClosedAt  time.Time `json:"closed_at"`
	URL       string    `json:"url"`
}

// ExportPerRepo exports PRs grouped by repository
func (e *JSONExporter) ExportPerRepo(repoPRs map[string][]*github.PullRequest) error {
	e.logger.Info("Exporting per-repo PRs to JSON")

	// Convert to exportable format
	exportData := make(map[string][]RepoPR)
	for repo, prs := range repoPRs {
		exportData[repo] = make([]RepoPR, 0, len(prs))
		for _, pr := range prs {
			author := ""
			if pr.User != nil {
				author = pr.User.GetLogin()
			}
			exportData[repo] = append(exportData[repo], RepoPR{
				Number:    pr.GetNumber(),
				Title:     pr.GetTitle(),
				Author:    author,
				State:     pr.GetState(),
				CreatedAt: pr.GetCreatedAt().Time,
				ClosedAt:  pr.GetClosedAt().Time,
				URL:       pr.GetHTMLURL(),
			})
		}
	}

	// Create output file path
	outputPath := filepath.Join(e.outputDir, "prs_by_repo.json")

	// Marshal to JSON with indentation
	jsonData, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	e.logger.Info("Per-repo JSON export complete", zap.String("path", outputPath))
	return nil
}

