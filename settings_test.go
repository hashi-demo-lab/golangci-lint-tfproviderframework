// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"testing"
)

func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	// Test analyzer toggles
	if !settings.EnableBasicTest {
		t.Error("EnableBasicTest should be true by default")
	}
	if !settings.EnableUpdateTest {
		t.Error("EnableUpdateTest should be true by default")
	}
	if !settings.EnableImportTest {
		t.Error("EnableImportTest should be true by default")
	}
	if !settings.EnableErrorTest {
		t.Error("EnableErrorTest should be true by default")
	}
	if !settings.EnableStateCheck {
		t.Error("EnableStateCheck should be true by default")
	}

	// Test path patterns
	if settings.ResourcePathPattern != "resource_*.go" {
		t.Errorf("ResourcePathPattern should be 'resource_*.go', got %s", settings.ResourcePathPattern)
	}
	if settings.DataSourcePathPattern != "data_source_*.go" {
		t.Errorf("DataSourcePathPattern should be 'data_source_*.go', got %s", settings.DataSourcePathPattern)
	}
	if settings.TestFilePattern != "*_test.go" {
		t.Errorf("TestFilePattern should be '*_test.go', got %s", settings.TestFilePattern)
	}

	// Test file exclusions
	if !settings.ExcludeBaseClasses {
		t.Error("ExcludeBaseClasses should be true by default")
	}
	if !settings.ExcludeSweeperFiles {
		t.Error("ExcludeSweeperFiles should be true by default")
	}
	if !settings.ExcludeMigrationFiles {
		t.Error("ExcludeMigrationFiles should be true by default")
	}
	if len(settings.ExcludePatterns) != 2 {
		t.Errorf("ExcludePatterns should have 2 elements, got %d", len(settings.ExcludePatterns))
	}
	if len(settings.IncludeHelperPatterns) != 3 {
		t.Errorf("IncludeHelperPatterns should have 3 elements, got %d", len(settings.IncludeHelperPatterns))
	}
	if settings.DiagnosticExclusions {
		t.Error("DiagnosticExclusions should be false by default")
	}

	// Test matching strategies
	if !settings.EnableFunctionMatching {
		t.Error("EnableFunctionMatching should be true by default")
	}
	if !settings.EnableFileBasedMatching {
		t.Error("EnableFileBasedMatching should be true by default")
	}
	if settings.EnableFuzzyMatching {
		t.Error("EnableFuzzyMatching should be false by default")
	}
	if settings.FuzzyMatchThreshold != 0.7 {
		t.Errorf("FuzzyMatchThreshold should be 0.7, got %f", settings.FuzzyMatchThreshold)
	}

	// Test provider configuration
	if settings.ProviderPrefix != "" {
		t.Errorf("ProviderPrefix should be empty by default, got %s", settings.ProviderPrefix)
	}
	if settings.ResourceNamingPattern != "" {
		t.Errorf("ResourceNamingPattern should be empty by default, got %s", settings.ResourceNamingPattern)
	}

	// Test output options
	if settings.Verbose {
		t.Error("Verbose should be false by default")
	}
	if settings.ShowMatchConfidence {
		t.Error("ShowMatchConfidence should be false by default")
	}
	if settings.ShowUnmatchedTests {
		t.Error("ShowUnmatchedTests should be false by default")
	}
	if !settings.ShowOrphanedResources {
		t.Error("ShowOrphanedResources should be true by default")
	}
}

func TestSettingsValidate_ValidThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
	}{
		{"zero threshold", 0.0},
		{"low threshold", 0.3},
		{"mid threshold", 0.5},
		{"high threshold", 0.7},
		{"max threshold", 1.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.FuzzyMatchThreshold = tc.threshold

			err := settings.Validate()
			if err != nil {
				t.Errorf("Validate() returned error for valid threshold %f: %v", tc.threshold, err)
			}
		})
	}
}

func TestSettingsValidate_InvalidThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
	}{
		{"negative threshold", -0.1},
		{"very negative threshold", -1.0},
		{"above max threshold", 1.1},
		{"way above max threshold", 2.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.FuzzyMatchThreshold = tc.threshold

			err := settings.Validate()
			if err == nil {
				t.Errorf("Validate() should return error for invalid threshold %f", tc.threshold)
			}
		})
	}
}

func TestSettingsValidate_InvalidRegex(t *testing.T) {
	settings := DefaultSettings()
	settings.ResourceNamingPattern = "[invalid(regex"

	err := settings.Validate()
	if err == nil {
		t.Error("Validate() should return error for invalid regex pattern")
	}
}

func TestSettingsValidate_ValidRegex(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
	}{
		{"empty pattern", ""},
		{"simple pattern", "^resource_"},
		{"complex pattern", `^(?:resource|data_source)_(\w+)$`},
		{"word match pattern", `\w+_\w+`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.ResourceNamingPattern = tc.pattern

			err := settings.Validate()
			if err != nil {
				t.Errorf("Validate() returned error for valid regex pattern %q: %v", tc.pattern, err)
			}
		})
	}
}

func TestSettingsValidate_NoMatchingStrategy(t *testing.T) {
	settings := DefaultSettings()
	settings.EnableFunctionMatching = false
	settings.EnableFileBasedMatching = false
	settings.EnableFuzzyMatching = false

	err := settings.Validate()
	if err == nil {
		t.Error("Validate() should return error when no matching strategy is enabled")
	}
}

func TestSettingsValidate_AtLeastOneMatchingStrategy(t *testing.T) {
	tests := []struct {
		name        string
		function    bool
		fileBased   bool
		fuzzy       bool
		shouldError bool
	}{
		{"only function matching", true, false, false, false},
		{"only file-based matching", false, true, false, false},
		{"only fuzzy matching", false, false, true, false},
		{"function and file-based", true, true, false, false},
		{"function and fuzzy", true, false, true, false},
		{"file-based and fuzzy", false, true, true, false},
		{"all strategies enabled", true, true, true, false},
		{"no strategies enabled", false, false, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := DefaultSettings()
			settings.EnableFunctionMatching = tc.function
			settings.EnableFileBasedMatching = tc.fileBased
			settings.EnableFuzzyMatching = tc.fuzzy

			err := settings.Validate()
			if tc.shouldError && err == nil {
				t.Error("Validate() should return error when no matching strategy is enabled")
			}
			if !tc.shouldError && err != nil {
				t.Errorf("Validate() returned unexpected error: %v", err)
			}
		})
	}
}
