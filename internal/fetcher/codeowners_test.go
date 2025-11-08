package fetcher

import (
	"testing"
)

func TestParseCODEOWNERS(t *testing.T) {
	content := []byte(`
# This is a comment
* @team1 @user1
/docs/ @team2
*.go @team3 @user2
`)

	fetcher := NewCODEOWNERSFetcher(nil, nil, nil)
	file, err := fetcher.ParseCODEOWNERS(content, "CODEOWNERS")
	if err != nil {
		t.Fatalf("Failed to parse CODEOWNERS: %v", err)
	}

	if len(file.Rules) != 3 {
		t.Fatalf("Expected 3 rules, got %d", len(file.Rules))
	}

	// Check first rule
	if file.Rules[0].Pattern != "/*" {
		t.Errorf("Expected pattern '/*', got '%s'", file.Rules[0].Pattern)
	}
	if len(file.Rules[0].Owners) != 2 {
		t.Errorf("Expected 2 owners, got %d", len(file.Rules[0].Owners))
	}
}

func TestFindOwners(t *testing.T) {
	content := []byte(`
* @team1
/docs/ @team2
`)

	fetcher := NewCODEOWNERSFetcher(nil, nil, nil)
	file, err := fetcher.ParseCODEOWNERS(content, "CODEOWNERS")
	if err != nil {
		t.Fatalf("Failed to parse CODEOWNERS: %v", err)
	}

	// Test directory match (more specific)
	owners := file.FindOwners("docs/README.md")
	if len(owners) == 0 {
		t.Error("Expected owners for docs/README.md")
	}
	if len(owners) > 0 && owners[0] != "@team2" {
		t.Errorf("Expected @team2, got %s", owners[0])
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		filePath string
		expected bool
	}{
		{"/docs/", "docs/README.md", true},
		{"/docs/", "src/main.go", false},
		{"/docs", "docs/README.md", true},
	}

	for _, tt := range tests {
		result := matchesPattern(tt.pattern, tt.filePath)
		if result != tt.expected {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.filePath, result, tt.expected)
		}
	}
}

