// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

// Settings configures which analyzers are enabled and file path patterns to match.
// All analyzers are enabled by default.
type Settings struct {
	EnableBasicTest       bool     `yaml:"enable-basic-test"`
	EnableUpdateTest      bool     `yaml:"enable-update-test"`
	EnableImportTest      bool     `yaml:"enable-import-test"`
	EnableErrorTest       bool     `yaml:"enable-error-test"`
	EnableStateCheck      bool     `yaml:"enable-state-check"`
	ResourcePathPattern   string   `yaml:"resource-path-pattern"`
	DataSourcePathPattern string   `yaml:"data-source-path-pattern"`
	TestFilePattern       string   `yaml:"test-file-pattern"`
	ExcludePaths          []string `yaml:"exclude-paths"`
	// ExcludeBaseClasses excludes files named base_*.go which are typically abstract base classes
	ExcludeBaseClasses bool `yaml:"exclude-base-classes"`
	// ExcludeSweeperFiles excludes files named *_sweeper.go which are test infrastructure
	// for cleaning up resources after acceptance tests, not production resources
	ExcludeSweeperFiles bool `yaml:"exclude-sweeper-files"`
	// ExcludeMigrationFiles excludes files named *_migrate.go, *_migration*.go, and
	// *_state_upgrader.go which are state migration utilities, not production resources
	ExcludeMigrationFiles bool `yaml:"exclude-migration-files"`
	// TestNamePatterns defines additional patterns to match test function names beyond TestAcc*
	// Defaults include: TestAcc*, TestResource*, TestDataSource*, Test*_
	TestNamePatterns []string `yaml:"test-name-patterns"`
	// Verbose enables detailed diagnostic output explaining why issues were flagged.
	// When enabled, diagnostic messages include test files searched, functions found,
	// why they didn't match, and suggested fixes.
	Verbose bool `yaml:"verbose"`
	// EnableFileBasedMatching enables fallback matching where a resource is considered
	// tested if a corresponding test file exists with any Test* functions, even if
	// the function names don't follow the standard naming conventions.
	EnableFileBasedMatching bool `yaml:"enable-file-based-matching"`
	// CustomTestHelpers defines additional test helper function names that wrap resource.Test()
	// By default, only resource.Test() is recognized. Add custom wrappers here.
	// Example: ["testhelper.AccTest", "internal.RunAccTest"]
	CustomTestHelpers []string `yaml:"custom-test-helpers"`
}

// DefaultSettings returns the default configuration with all analyzers enabled.
func DefaultSettings() Settings {
	return Settings{
		EnableBasicTest:         true,
		EnableUpdateTest:        true,
		EnableImportTest:        true,
		EnableErrorTest:         true,
		EnableStateCheck:        true,
		ResourcePathPattern:     "resource_*.go",
		DataSourcePathPattern:   "data_source_*.go",
		TestFilePattern:         "*_test.go",
		ExcludePaths:            []string{},
		ExcludeBaseClasses:      true,  // Exclude base_*.go by default
		ExcludeSweeperFiles:     true,  // Exclude *_sweeper.go by default (test infrastructure)
		ExcludeMigrationFiles:   true,  // Exclude *_migrate.go, *_migration*.go, *_state_upgrader.go by default
		TestNamePatterns:        []string{}, // Empty means use all default patterns
		Verbose:                 false, // Verbose mode disabled by default
		EnableFileBasedMatching: true,  // Enable file-based matching by default
		CustomTestHelpers:       []string{}, // Empty means only resource.Test() is recognized
	}
}

// defaultTestPatterns returns the default test function name patterns
var defaultTestPatterns = []string{
	"TestAcc",        // Standard HashiCorp pattern: TestAccResourceFoo_basic
	"TestResource",   // Alternative: TestResourceFoo
	"TestDataSource", // Alternative: TestDataSourceFoo
}
