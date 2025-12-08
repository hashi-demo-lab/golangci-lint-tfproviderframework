// Package config provides configuration settings for the tfprovidertest plugin.
package config

import (
	"fmt"
	"regexp"
	"time"
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

	// TestFilePrefixPatterns defines prefix patterns for extracting resource names from test file paths.
	// Each pattern has the format "prefix:is_datasource" where is_datasource is "true" or "false".
	// The prefix is stripped from the filename to extract the resource name.
	// Examples: ["resource_:false", "data_source_:true", "iam_:false", "ephemeral_:false"]
	// Default patterns cover standard Terraform provider conventions.
	TestFilePrefixPatterns []string `yaml:"test-file-prefix-patterns"`
	// TestFileSuffixPatterns defines suffix patterns for extracting resource names from test file paths.
	// Each pattern has the format "suffix:is_datasource" where is_datasource is "true" or "false".
	// The suffix is stripped from the filename to extract the resource name.
	// Examples: ["_resource:false", "_data_source:true", "_datasource:true"]
	TestFileSuffixPatterns []string `yaml:"test-file-suffix-patterns"`
	// TestFileSuffixStrip defines suffixes to strip from extracted resource names after prefix/suffix removal.
	// This handles generated file naming conventions like "_generated", "_gen".
	// Examples: ["_generated", "_gen"]
	TestFileSuffixStrip []string `yaml:"test-file-suffix-strip"`
	// NestedSchemaPatterns defines patterns that identify nested schema types (false positives).
	// Resources matching these patterns are excluded from coverage reports.
	// Supports suffix patterns (ending with *) and contains patterns (containing *pattern*).
	// Examples: ["*_schema", "*_schema_*"] - filters names ending with "_schema" or containing "_schema_"
	NestedSchemaPatterns []string `yaml:"nested-schema-patterns"`
	// FunctionNameKeywordsToStrip defines CamelCase keywords to remove from test function names
	// before matching. This handles patterns like IAM tests where "IamBinding" should be stripped
	// so "TestAccComputeDiskIamBinding" matches "compute_disk".
	// Keywords are matched in CamelCase form (e.g., "Iam", "Binding", "Member", "Policy").
	// Examples: ["Iam", "IamBinding", "IamMember", "IamPolicy", "Generated"]
	FunctionNameKeywordsToStrip []string `yaml:"function-name-keywords-to-strip"`
	// TestFunctionSuffixes defines snake_case suffixes to strip from test function names
	// before matching to resources. This handles test naming conventions.
	// Default includes: _basic, _update, _import, _withCondition, etc.
	// If empty, uses built-in defaults. To disable suffix stripping, set to ["-"].
	TestFunctionSuffixes []string `yaml:"test-function-suffixes"`

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

	// Cache configuration
	// CacheTTL specifies how long cache entries remain valid before automatic eviction.
	// This prevents memory leaks in long-running processes (e.g., golangci-lint daemon mode).
	// Default is 5 minutes. Set to 0 to disable TTL-based eviction.
	// Format: duration string like "5m", "1h", "30s"
	CacheTTL string `yaml:"cache-ttl"`
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

		// Test file pattern defaults - cover common Terraform provider conventions
		TestFilePrefixPatterns: []string{
			"resource_:false",
			"data_source_:true",
			"ephemeral_:false",
			"action_:false",
			"iam_:false", // IAM test files test parent resource IAM bindings
		},
		TestFileSuffixPatterns: []string{
			"_resource:false",
			"_data_source:true",
			"_datasource:true",
			"_action:false",
		},
		TestFileSuffixStrip: []string{
			"_generated",
			"_gen",
		},
		// Nested schema patterns - filter out helper types that aren't real resources
		NestedSchemaPatterns: []string{
			"*_schema",   // Suffix pattern: names ending with _schema
			"*_schema_*", // Contains pattern: names containing _schema_
		},
		// CamelCase keywords to strip from test function names for better matching
		// These handle IAM tests and generated test patterns
		FunctionNameKeywordsToStrip: []string{
			"IamBinding",  // IAM binding tests
			"IamMember",   // IAM member tests
			"IamPolicy",   // IAM policy tests
			"Iam",         // Generic IAM keyword (must be after specific ones)
			"Generated",   // Generated test suffix
		},
		// Test function suffixes - empty means use built-in defaults
		// Set to ["-"] to disable suffix stripping
		TestFunctionSuffixes: []string{},

		// Provider configuration
		ProviderPrefix:        "",
		ResourceNamingPattern: "",

		// Output options
		Verbose:               false, // Verbose mode disabled by default
		ShowMatchConfidence:   false,
		ShowUnmatchedTests:    false,
		ShowOrphanedResources: true, // Show orphaned resources by default

		// Cache configuration
		CacheTTL: "5m", // 5 minutes default TTL
	}
}

// Validate validates the settings and returns any errors
func (s *Settings) Validate() error {
	// Validate threshold range
	if s.FuzzyMatchThreshold < 0.0 || s.FuzzyMatchThreshold > 1.0 {
		return fmt.Errorf("fuzzy-match-threshold must be between 0.0 and 1.0, got %f", s.FuzzyMatchThreshold)
	}

	// Validate regex pattern (ResourceNamingPattern is a regex, not a glob)
	if s.ResourceNamingPattern != "" {
		if _, err := regexp.Compile(s.ResourceNamingPattern); err != nil {
			return fmt.Errorf("invalid resource-naming-pattern: %w", err)
		}
	}

	// Note: TestFilePattern, ResourcePathPattern, and DataSourcePathPattern are glob patterns
	// (e.g., "*_test.go", "resource_*.go"), not regex patterns. They're used with filepath.Match,
	// so we don't validate them here. Invalid glob patterns will fail at runtime with clear errors.

	// Validate that at least one analyzer is enabled
	if !s.EnableBasicTest && !s.EnableUpdateTest && !s.EnableImportTest &&
		!s.EnableErrorTest && !s.EnableStateCheck {
		return fmt.Errorf("at least one analyzer must be enabled (enable-basic-test, enable-update-test, enable-import-test, enable-error-test, or enable-state-check)")
	}

	// Cross-field validation: if fuzzy matching is enabled, threshold must be reasonable
	if s.EnableFuzzyMatching && s.FuzzyMatchThreshold < 0.5 {
		return fmt.Errorf("fuzzy-match-threshold should be at least 0.5 when fuzzy matching is enabled (got %f)", s.FuzzyMatchThreshold)
	}

	// Validate cache TTL
	if s.CacheTTL != "" {
		if _, err := time.ParseDuration(s.CacheTTL); err != nil {
			return fmt.Errorf("invalid cache-ttl format: %w (expected duration like '5m', '1h', '30s')", err)
		}
	}

	return nil
}

// GetCacheTTLDuration returns the parsed cache TTL duration.
// Returns 5 minutes if CacheTTL is empty or invalid.
// Returns 0 if TTL-based eviction should be disabled.
func (s *Settings) GetCacheTTLDuration() time.Duration {
	if s.CacheTTL == "" {
		return 5 * time.Minute // Default to 5 minutes
	}
	if s.CacheTTL == "0" || s.CacheTTL == "0s" {
		return 0 // Disable TTL-based eviction
	}
	duration, err := time.ParseDuration(s.CacheTTL)
	if err != nil {
		return 5 * time.Minute // Fallback to default
	}
	return duration
}
