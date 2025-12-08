// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"testing"

	"github.com/example/tfprovidertest/pkg/config"
)

func TestDefaultSettings(t *testing.T) {
	settings := config.DefaultSettings()

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
			settings := config.DefaultSettings()
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
			settings := config.DefaultSettings()
			settings.FuzzyMatchThreshold = tc.threshold

			err := settings.Validate()
			if err == nil {
				t.Errorf("Validate() should return error for invalid threshold %f", tc.threshold)
			}
		})
	}
}

func TestSettingsValidate_InvalidRegex(t *testing.T) {
	settings := config.DefaultSettings()
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
			settings := config.DefaultSettings()
			settings.ResourceNamingPattern = tc.pattern

			err := settings.Validate()
			if err != nil {
				t.Errorf("Validate() returned error for valid regex pattern %q: %v", tc.pattern, err)
			}
		})
	}
}

// TestSettingsValidate_FuzzyMatchingOptional verifies that fuzzy matching is optional.
// Function name matching and file-based matching always run (not configurable).
func TestSettingsValidate_FuzzyMatchingOptional(t *testing.T) {
	tests := []struct {
		name                string
		enableFuzzyMatching bool
	}{
		{"fuzzy matching enabled", true},
		{"fuzzy matching disabled", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			settings := config.DefaultSettings()
			settings.EnableFuzzyMatching = tc.enableFuzzyMatching

			err := settings.Validate()
			if err != nil {
				t.Errorf("Validate() returned unexpected error: %v", err)
			}
		})
	}
}

// TestSettingsValidate_AtLeastOneAnalyzerEnabled verifies that at least one analyzer must be enabled
func TestSettingsValidate_AtLeastOneAnalyzerEnabled(t *testing.T) {
	t.Run("all analyzers disabled should fail", func(t *testing.T) {
		settings := config.DefaultSettings()
		settings.EnableBasicTest = false
		settings.EnableUpdateTest = false
		settings.EnableImportTest = false
		settings.EnableErrorTest = false
		settings.EnableStateCheck = false

		err := settings.Validate()
		if err == nil {
			t.Error("Validate() should return error when all analyzers are disabled")
		}
	})

	t.Run("at least one analyzer enabled should pass", func(t *testing.T) {
		tests := []struct {
			name    string
			enabler func(*config.Settings)
		}{
			{"only basic test", func(s *config.Settings) { s.EnableBasicTest = true }},
			{"only update test", func(s *config.Settings) { s.EnableUpdateTest = true }},
			{"only import test", func(s *config.Settings) { s.EnableImportTest = true }},
			{"only error test", func(s *config.Settings) { s.EnableErrorTest = true }},
			{"only state check", func(s *config.Settings) { s.EnableStateCheck = true }},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				settings := config.DefaultSettings()
				// Disable all
				settings.EnableBasicTest = false
				settings.EnableUpdateTest = false
				settings.EnableImportTest = false
				settings.EnableErrorTest = false
				settings.EnableStateCheck = false
				// Enable specific one
				tc.enabler(&settings)

				err := settings.Validate()
				if err != nil {
					t.Errorf("Validate() returned error when %s is enabled: %v", tc.name, err)
				}
			})
		}
	})
}

// TestSettingsValidate_FuzzyMatchingThreshold verifies cross-field validation
func TestSettingsValidate_FuzzyMatchingThreshold(t *testing.T) {
	t.Run("fuzzy matching enabled with low threshold should fail", func(t *testing.T) {
		settings := config.DefaultSettings()
		settings.EnableFuzzyMatching = true
		settings.FuzzyMatchThreshold = 0.3 // Below 0.5

		err := settings.Validate()
		if err == nil {
			t.Error("Validate() should return error when fuzzy matching enabled with threshold < 0.5")
		}
	})

	t.Run("fuzzy matching enabled with good threshold should pass", func(t *testing.T) {
		settings := config.DefaultSettings()
		settings.EnableFuzzyMatching = true
		settings.FuzzyMatchThreshold = 0.7

		err := settings.Validate()
		if err != nil {
			t.Errorf("Validate() should not return error: %v", err)
		}
	})

	t.Run("fuzzy matching disabled with low threshold should pass", func(t *testing.T) {
		settings := config.DefaultSettings()
		settings.EnableFuzzyMatching = false
		settings.FuzzyMatchThreshold = 0.3 // Low threshold OK when fuzzy matching disabled

		err := settings.Validate()
		if err != nil {
			t.Errorf("Validate() should not return error when fuzzy matching disabled: %v", err)
		}
	})
}
