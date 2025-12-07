// Package contracts defines the API contracts for the tfprovidertest linter.
// This is a design document, not executable code.
package contracts

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/analysis"
)

// ==================== Plugin Interface ====================

// LinterPlugin is the interface required by golangci-lint's plugin-module-register.
type LinterPlugin interface {
	// BuildAnalyzers returns the list of analyzers provided by this plugin.
	// Each analyzer represents one linting rule.
	BuildAnalyzers() ([]*analysis.Analyzer, error)

	// GetLoadMode returns the analysis load mode required by this plugin.
	// Returns "syntax" for AST-only analysis (no type information).
	GetLoadMode() string
}

// ==================== Settings Interface ====================

// Settings represents user configuration loaded from .golangci.yml.
type Settings struct {
	// Rule toggles
	EnableBasicTest  bool `yaml:"enable-basic-test" json:"enable-basic-test"`
	EnableUpdateTest bool `yaml:"enable-update-test" json:"enable-update-test"`
	EnableImportTest bool `yaml:"enable-import-test" json:"enable-import-test"`
	EnableErrorTest  bool `yaml:"enable-error-test" json:"enable-error-test"`
	EnableStateCheck bool `yaml:"enable-state-check" json:"enable-state-check"`

	// Path patterns
	ResourcePathPattern   string   `yaml:"resource-path-pattern" json:"resource-path-pattern"`
	DataSourcePathPattern string   `yaml:"data-source-path-pattern" json:"data-source-path-pattern"`
	TestFilePattern       string   `yaml:"test-file-pattern" json:"test-file-pattern"`
	ExcludePaths          []string `yaml:"exclude-paths" json:"exclude-paths"`
}

// DefaultSettings returns the default configuration.
func DefaultSettings() Settings {
	return Settings{
		EnableBasicTest:       true,
		EnableUpdateTest:      true,
		EnableImportTest:      true,
		EnableErrorTest:       true,
		EnableStateCheck:      true,
		ResourcePathPattern:   "resource_*.go",
		DataSourcePathPattern: "data_source_*.go",
		TestFilePattern:       "*_test.go",
		ExcludePaths:          []string{},
	}
}

// ==================== Resource Registry Interface ====================

// ResourceRegistry maintains the mapping between resources and their tests.
type ResourceRegistry interface {
	// RegisterResource adds a resource to the registry.
	RegisterResource(info *ResourceInfo)

	// RegisterTestFile adds a test file to the registry.
	RegisterTestFile(info *TestFileInfo)

	// GetResourceWithTest retrieves a resource and its corresponding test file.
	// Returns (resource, testFile, true) if both found, (resource, nil, false) if only resource found.
	GetResourceWithTest(name string) (*ResourceInfo, *TestFileInfo, bool)

	// GetUntestedResources returns all resources without corresponding test files.
	GetUntestedResources() []*ResourceInfo

	// GetUntestedDataSources returns all data sources without corresponding test files.
	GetUntestedDataSources() []*ResourceInfo
}

// ==================== Entity Interfaces ====================

// ResourceInfo represents a detected Terraform resource or data source.
type ResourceInfo struct {
	Name            string          // Resource type name (e.g., "widget")
	IsDataSource    bool            // true for data sources, false for resources
	FilePath        string          // Absolute path to schema definition file
	SchemaPos       token.Pos       // AST position of Schema() method
	Attributes      []AttributeInfo // Schema attributes
	HasImportState  bool            // Whether resource implements ImportState
	ImportStatePos  token.Pos       // AST position of ImportState() method (if HasImportState)
	PackageImports  []string        // Import paths (for framework vs SDK detection)
	SchemaNode      *ast.FuncDecl   // AST node of Schema() method
	ImportStateNode *ast.FuncDecl   // AST node of ImportState() method
}

// IsFrameworkResource returns true if this resource uses terraform-plugin-framework.
func (r *ResourceInfo) IsFrameworkResource() bool {
	for _, imp := range r.PackageImports {
		if imp == "github.com/hashicorp/terraform-plugin-framework/resource" ||
			imp == "github.com/hashicorp/terraform-plugin-framework/datasource" {
			return true
		}
	}
	return false
}

// HasUpdatableAttributes returns true if any attribute can be updated in-place.
func (r *ResourceInfo) HasUpdatableAttributes() bool {
	for _, attr := range r.Attributes {
		if attr.IsUpdatable {
			return true
		}
	}
	return false
}

// HasValidationRules returns true if any attribute has validators or is required.
func (r *ResourceInfo) HasValidationRules() bool {
	for _, attr := range r.Attributes {
		if attr.HasValidators || attr.Required {
			return true
		}
	}
	return false
}

// AttributeInfo represents a schema attribute.
type AttributeInfo struct {
	Name           string   // Attribute name (e.g., "description")
	Type           string   // Attribute type (e.g., "String", "Int64")
	Required       bool     // Whether attribute is required
	Optional       bool     // Whether attribute is optional
	Computed       bool     // Whether attribute is computed
	IsUpdatable    bool     // Whether attribute can be updated in-place
	HasValidators  bool     // Whether attribute has validators
	ValidatorTypes []string // List of validator type names
}

// TestFileInfo represents a test file containing acceptance tests.
type TestFileInfo struct {
	FilePath      string             // Absolute path to test file
	ResourceName  string             // Resource name this test covers
	IsDataSource  bool               // Whether this tests a data source
	TestFunctions []TestFunctionInfo // Test functions in this file
}

// HasBasicTest returns true if there's at least one acceptance test function.
func (t *TestFileInfo) HasBasicTest() bool {
	for _, fn := range t.TestFunctions {
		if fn.UsesResourceTest {
			return true
		}
	}
	return false
}

// HasUpdateTest returns true if any test has multiple steps with different configs.
func (t *TestFileInfo) HasUpdateTest() bool {
	for _, fn := range t.TestFunctions {
		if len(fn.TestSteps) > 1 {
			// Check if configs differ between steps
			for i := 1; i < len(fn.TestSteps); i++ {
				if fn.TestSteps[i].Config != fn.TestSteps[0].Config {
					return true
				}
			}
		}
	}
	return false
}

// HasImportTest returns true if any test step uses ImportState with verification.
func (t *TestFileInfo) HasImportTest() bool {
	for _, fn := range t.TestFunctions {
		if fn.HasImportStep {
			// Verify at least one step has ImportStateVerify
			for _, step := range fn.TestSteps {
				if step.ImportState && step.ImportStateVerify {
					return true
				}
			}
		}
	}
	return false
}

// HasErrorTest returns true if any test step uses ExpectError.
func (t *TestFileInfo) HasErrorTest() bool {
	for _, fn := range t.TestFunctions {
		if fn.HasErrorCase {
			return true
		}
	}
	return false
}

// HasStateChecks returns true if all test steps include Check functions.
func (t *TestFileInfo) HasStateChecks() bool {
	for _, fn := range t.TestFunctions {
		for _, step := range fn.TestSteps {
			// Skip error steps (checks not required)
			if step.ExpectError {
				continue
			}
			// Skip import steps (state verify handles this)
			if step.ImportState && step.ImportStateVerify {
				continue
			}
			// Regular steps must have checks
			if !step.HasCheck || len(step.CheckFunctions) == 0 {
				return false
			}
		}
	}
	return true
}

// TestFunctionInfo represents a test function.
type TestFunctionInfo struct {
	Name             string         // Function name
	FunctionPos      token.Pos      // AST position of function
	UsesResourceTest bool           // Whether function calls resource.Test()
	TestSteps        []TestStepInfo // Test steps in this function
	HasErrorCase     bool           // Whether any step uses ExpectError
	HasImportStep    bool           // Whether any step uses ImportState
}

// TestStepInfo represents a resource.TestStep{}.
type TestStepInfo struct {
	StepNumber         int      // Sequential step number (0-based)
	Config             string   // Terraform configuration
	HasCheck           bool     // Whether step includes Check field
	CheckFunctions     []string // Check function names
	ImportState        bool     // Whether this is an import test step
	ImportStateVerify  bool     // Whether import verification is enabled
	ExpectError        bool     // Whether step expects an error
	ExpectErrorPattern string   // Regex pattern for expected error (if applicable)
}

// IsUpdateStep returns true if this is not the first step (implies update).
func (s *TestStepInfo) IsUpdateStep() bool {
	return s.StepNumber > 0
}

// IsValidImportStep returns true if this step properly tests import.
func (s *TestStepInfo) IsValidImportStep() bool {
	return s.ImportState && s.ImportStateVerify
}

// ==================== Analyzer Factory ====================

// AnalyzerFactory creates analyzer instances based on settings.
type AnalyzerFactory interface {
	// CreateBasicTestAnalyzer creates the basic acceptance test analyzer.
	CreateBasicTestAnalyzer(settings Settings, registry ResourceRegistry) *analysis.Analyzer

	// CreateUpdateTestAnalyzer creates the update test analyzer.
	CreateUpdateTestAnalyzer(settings Settings, registry ResourceRegistry) *analysis.Analyzer

	// CreateImportTestAnalyzer creates the import test analyzer.
	CreateImportTestAnalyzer(settings Settings, registry ResourceRegistry) *analysis.Analyzer

	// CreateErrorTestAnalyzer creates the error test analyzer.
	CreateErrorTestAnalyzer(settings Settings, registry ResourceRegistry) *analysis.Analyzer

	// CreateStateCheckAnalyzer creates the state check analyzer.
	CreateStateCheckAnalyzer(settings Settings, registry ResourceRegistry) *analysis.Analyzer
}

// ==================== AST Utilities ====================

// ASTParser provides utility functions for AST traversal and pattern matching.
type ASTParser interface {
	// DetectResource finds resource.Resource implementations in a file.
	DetectResource(file *ast.File) []*ResourceInfo

	// DetectDataSource finds datasource.DataSource implementations in a file.
	DetectDataSource(file *ast.File) []*ResourceInfo

	// ExtractAttributes parses schema.Schema{} composite literals.
	ExtractAttributes(schemaNode *ast.FuncDecl) []AttributeInfo

	// DetectImportState checks if a type implements ImportState method.
	DetectImportState(file *ast.File, resourceType string) (bool, token.Pos)

	// ParseTestFile extracts test function information from a test file.
	ParseTestFile(file *ast.File) *TestFileInfo

	// MatchTestToResource determines which resource a test file corresponds to.
	MatchTestToResource(testFilePath string, settings Settings) (string, bool)
}

// ==================== Diagnostic Builder ====================

// DiagnosticBuilder constructs actionable error messages.
type DiagnosticBuilder interface {
	// BuildBasicTestMissing creates diagnostic for missing basic test.
	BuildBasicTestMissing(resource *ResourceInfo) analysis.Diagnostic

	// BuildUpdateTestMissing creates diagnostic for missing update test.
	BuildUpdateTestMissing(resource *ResourceInfo) analysis.Diagnostic

	// BuildImportTestMissing creates diagnostic for missing import test.
	BuildImportTestMissing(resource *ResourceInfo) analysis.Diagnostic

	// BuildErrorTestMissing creates diagnostic for missing error test.
	BuildErrorTestMissing(resource *ResourceInfo) analysis.Diagnostic

	// BuildStateCheckMissing creates diagnostic for missing state checks.
	BuildStateCheckMissing(testFile *TestFileInfo, stepNumber int) analysis.Diagnostic
}
