// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"regexp"
)

// Settings configures which analyzers are enabled and file path patterns to match.
// All analyzers are enabled by default.
type Settings struct {
	// Analyzer toggles
	EnableBasicTest  bool `yaml:"enable-basic-test"`
	EnableUpdateTest bool `yaml:"enable-update-test"`
	EnableImportTest bool `yaml:"enable-import-test"`
	EnableErrorTest  bool `yaml:"enable-error-test"`
	EnableStateCheck bool `yaml:"enable-state-check"`

	// Path patterns
	ResourcePathPattern   string   `yaml:"resource-path-pattern"`
	DataSourcePathPattern string   `yaml:"data-source-path-pattern"`
	TestFilePattern       string   `yaml:"test-file-pattern"`
	ExcludePaths          []string `yaml:"exclude-paths"`

	// File exclusions
	// ExcludeBaseClasses excludes files named base_*.go which are typically abstract base classes
	ExcludeBaseClasses bool `yaml:"exclude-base-classes"`
	// ExcludeSweeperFiles excludes files named *_sweeper.go which are test infrastructure
	// for cleaning up resources after acceptance tests, not production resources
	ExcludeSweeperFiles bool `yaml:"exclude-sweeper-files"`
	// ExcludeMigrationFiles excludes files named *_migrate.go, *_migration*.go, and
	// *_state_upgrader.go which are state migration utilities, not production resources
	ExcludeMigrationFiles bool `yaml:"exclude-migration-files"`
	// ExcludePatterns defines glob patterns for files to exclude from analysis
	// Examples: ["*_sweeper.go", "*_test_helpers.go"]
	ExcludePatterns []string `yaml:"exclude-patterns"`
	// IncludeHelperPatterns defines patterns to identify helper functions
	// Examples: ["*Helper*", "*Wrapper*", "AccTest*"]
	IncludeHelperPatterns []string `yaml:"include-helper-patterns"`
	// DiagnosticExclusions when true, outputs diagnostic information about excluded files
	DiagnosticExclusions bool `yaml:"diagnostic-exclusions"`

	// Test detection
	// TestNamePatterns defines additional patterns to match test function names beyond TestAcc*
	// Defaults include: TestAcc*, TestResource*, TestDataSource*, Test*_
	TestNamePatterns []string `yaml:"test-name-patterns"`
	// CustomTestHelpers defines additional test helper function names that wrap resource.Test()
	// By default, only resource.Test() is recognized. Add custom wrappers here.
	// Example: ["testhelper.AccTest", "internal.RunAccTest"]
	CustomTestHelpers []string `yaml:"custom-test-helpers"`

	// Matching strategies
	// EnableFuzzyMatching enables fuzzy string matching for resource-to-test associations.
	// This is disabled by default as it can be expensive and may produce false positives.
	// Function name matching and file-based matching always run as they are fast and accurate.
	EnableFuzzyMatching bool `yaml:"enable-fuzzy-matching"`
	// FuzzyMatchThreshold sets the minimum similarity score (0.0-1.0) for fuzzy matches
	FuzzyMatchThreshold float64 `yaml:"fuzzy-match-threshold"`

	// Provider configuration
	// ProviderPrefix specifies the provider prefix for function name matching (e.g., "AWS", "Google")
	ProviderPrefix string `yaml:"provider-prefix"`
	// ResourceNamingPattern is a regex pattern for extracting resource names from identifiers
	ResourceNamingPattern string `yaml:"resource-naming-pattern"`

	// Output options
	// Verbose enables detailed diagnostic output explaining why issues were flagged.
	// When enabled, diagnostic messages include test files searched, functions found,
	// why they didn't match, and suggested fixes.
	Verbose bool `yaml:"verbose"`
	// ShowMatchConfidence when true, displays confidence scores for test-to-resource matches
	ShowMatchConfidence bool `yaml:"show-match-confidence"`
	// ShowUnmatchedTests when true, reports test functions that couldn't be associated with any resource
	ShowUnmatchedTests bool `yaml:"show-unmatched-tests"`
	// ShowOrphanedResources when true, reports resources without any test coverage
	ShowOrphanedResources bool `yaml:"show-orphaned-resources"`
}

// DefaultSettings returns the default configuration with all analyzers enabled.
func DefaultSettings() Settings {
	return Settings{
		// Analyzer toggles
		EnableBasicTest:  true,
		EnableUpdateTest: true,
		EnableImportTest: true,
		EnableErrorTest:  true,
		EnableStateCheck: true,

		// Path patterns
		ResourcePathPattern:   "resource_*.go",
		DataSourcePathPattern: "data_source_*.go",
		TestFilePattern:       "*_test.go",
		ExcludePaths:          []string{},

		// File exclusions
		ExcludeBaseClasses:    true, // Exclude base_*.go by default
		ExcludeSweeperFiles:   true, // Exclude *_sweeper.go by default (test infrastructure)
		ExcludeMigrationFiles: true, // Exclude *_migrate.go, *_migration*.go, *_state_upgrader.go by default
		ExcludePatterns:       []string{"*_sweeper.go", "*_test_helpers.go"},
		IncludeHelperPatterns: []string{"*Helper*", "*Wrapper*", "AccTest*"},
		DiagnosticExclusions:  false,

		// Test detection
		TestNamePatterns:  []string{}, // Empty means use all default patterns
		CustomTestHelpers: []string{}, // Empty means only resource.Test() is recognized

		// Matching strategies
		// Function name matching and file-based matching always run (fast and accurate)
		EnableFuzzyMatching: false, // Fuzzy matching disabled by default (expensive, false positives)
		FuzzyMatchThreshold: 0.7,   // 70% similarity threshold for fuzzy matches

		// Provider configuration
		ProviderPrefix:        "",
		ResourceNamingPattern: "",

		// Output options
		Verbose:               false, // Verbose mode disabled by default
		ShowMatchConfidence:   false,
		ShowUnmatchedTests:    false,
		ShowOrphanedResources: true, // Show orphaned resources by default
	}
}

// Validate validates the settings and returns any errors
func (s *Settings) Validate() error {
	// Validate threshold range
	if s.FuzzyMatchThreshold < 0.0 || s.FuzzyMatchThreshold > 1.0 {
		return fmt.Errorf("fuzzy-match-threshold must be between 0.0 and 1.0, got %f", s.FuzzyMatchThreshold)
	}

	// Validate regex patterns
	if s.ResourceNamingPattern != "" {
		if _, err := regexp.Compile(s.ResourceNamingPattern); err != nil {
			return fmt.Errorf("invalid resource-naming-pattern: %w", err)
		}
	}

	return nil
}

// defaultTestPatterns returns the default test function name patterns
var defaultTestPatterns = []string{
	"TestAcc",        // Standard HashiCorp pattern: TestAccResourceFoo_basic
	"TestResource",   // Alternative: TestResourceFoo
	"TestDataSource", // Alternative: TestDataSourceFoo
}
