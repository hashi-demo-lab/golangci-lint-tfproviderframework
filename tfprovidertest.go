// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
//
// The plugin provides five analyzers that enforce HashiCorp's testing best practices:
//   - Basic Test Coverage: Detects resources without acceptance tests
//   - Update Test Coverage: Validates multi-step tests for updatable attributes
//   - Import Test Coverage: Ensures ImportState methods have import tests
//   - Error Test Coverage: Verifies validation rules have error case tests
//   - State Check Validation: Confirms test steps include state validation functions
package tfprovidertest

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

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
		EnableBasicTest:       true,
		EnableUpdateTest:      true,
		EnableImportTest:      true,
		EnableErrorTest:       true,
		EnableStateCheck:      true,
		ResourcePathPattern:   "resource_*.go",
		DataSourcePathPattern: "data_source_*.go",
		TestFilePattern:       "*_test.go",
		ExcludePaths:          []string{},
		ExcludeBaseClasses:    true,  // Exclude base_*.go by default
		ExcludeSweeperFiles:   true,  // Exclude *_sweeper.go by default (test infrastructure)
		ExcludeMigrationFiles: true,  // Exclude *_migrate.go, *_migration*.go, *_state_upgrader.go by default
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

// isTestFunction checks if a function name matches any of the test naming patterns
func isTestFunction(funcName string, customPatterns []string) bool {
	// Always require "Test" prefix (capital T for exported tests)
	if !strings.HasPrefix(funcName, "Test") {
		return false
	}

	// Use custom patterns if provided, otherwise use defaults
	patterns := customPatterns
	if len(patterns) == 0 {
		patterns = defaultTestPatterns
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(funcName, pattern) {
			return true
		}
	}

	// Also match any Test*_ pattern (e.g., TestPrivateKey_RSA)
	// This catches test functions like TestPrivateKeyRSA or TestDataSource_200
	if strings.Contains(funcName, "_") {
		return true
	}

	// Match any Test function that starts with uppercase after Test
	// This handles patterns like TestPrivateKeyRSA, TestWidgetSomething
	if len(funcName) > 4 && unicode.IsUpper(rune(funcName[4])) {
		return true
	}

	return false
}

// IsTestFunctionExported is an exported version of isTestFunction for testing
func IsTestFunctionExported(funcName string, customPatterns []string) bool {
	return isTestFunction(funcName, customPatterns)
}

// CamelCaseToSnakeCaseExported is an exported version of toSnakeCase for testing
func CamelCaseToSnakeCaseExported(s string) string {
	return toSnakeCase(s)
}

// ExtractResourceNameFromTestFunc is an exported version of extractResourceNameFromTestFunc for testing
func ExtractResourceNameFromTestFunc(funcName string) string {
	return extractResourceNameFromTestFunc(funcName)
}

// ParseResources is an exported version of parseResources for testing
func ParseResources(file *ast.File, fset *token.FileSet, filePath string) []*ResourceInfo {
	return parseResources(file, fset, filePath)
}

// ParseTestFile is an exported version of parseTestFile for testing
func ParseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	return parseTestFile(file, fset, filePath)
}

// ParseTestFileWithHelpers is an exported version of parseTestFileWithHelpers for testing
func ParseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
	return parseTestFileWithHelpers(file, fset, filePath, customHelpers)
}

// CheckUsesResourceTestWithHelpers is an exported version for testing
func CheckUsesResourceTestWithHelpers(body *ast.BlockStmt, customHelpers []string) bool {
	return checkUsesResourceTestWithHelpers(body, customHelpers)
}

// HasMatchingTestFile checks if a resource has a matching test file with Test* functions.
// This provides file-based test matching fallback for providers with non-standard naming.
func HasMatchingTestFile(resourceName string, isDataSource bool, registry *ResourceRegistry) bool {
	testFile := registry.GetTestFile(resourceName)
	if testFile == nil {
		return false
	}
	return len(testFile.TestFunctions) > 0
}

// isBaseClassFile checks if a file is a base class file that should be excluded
func isBaseClassFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasPrefix(base, "base_") || strings.HasPrefix(base, "base.")
}

// IsSweeperFile checks if a file is a sweeper file that should be excluded.
// Sweeper files are test infrastructure files for cleaning up resources after
// acceptance tests. They follow the naming pattern *_sweeper.go.
func IsSweeperFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_sweeper.go")
}

// IsMigrationFile checks if a file is a state migration file that should be excluded.
// Migration files are state migration utilities, not production resources. They follow
// naming patterns: *_migrate.go, *_migration*.go, *_state_upgrader.go
func IsMigrationFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_migrate.go") ||
		strings.Contains(base, "_migration") ||
		strings.HasSuffix(base, "_state_upgrader.go")
}

// BuildExpectedTestPath constructs the expected test file path for a given resource.
// For example, /path/to/resource_widget.go -> /path/to/resource_widget_test.go
func BuildExpectedTestPath(resource *ResourceInfo) string {
	filePath := resource.FilePath
	if strings.HasSuffix(filePath, ".go") {
		return strings.TrimSuffix(filePath, ".go") + "_test.go"
	}
	return filePath + "_test.go"
}

// BuildExpectedTestFunc constructs the expected test function name for a given resource.
// For example, widget -> TestAccWidget_basic, http (data source) -> TestAccDataSourceHttp_basic
func BuildExpectedTestFunc(resource *ResourceInfo) string {
	titleName := toTitleCase(resource.Name)
	if resource.IsDataSource {
		return fmt.Sprintf("TestAccDataSource%s_basic", titleName)
	}
	return fmt.Sprintf("TestAcc%s_basic", titleName)
}

// ResourceRegistry maintains thread-safe mappings of resources, data sources,
// and their associated test files discovered during AST analysis.
type ResourceRegistry struct {
	mu          sync.RWMutex
	resources   map[string]*ResourceInfo
	dataSources map[string]*ResourceInfo
	testFiles   map[string]*TestFileInfo
}

// NewResourceRegistry creates a new empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		resources:   make(map[string]*ResourceInfo),
		dataSources: make(map[string]*ResourceInfo),
		testFiles:   make(map[string]*TestFileInfo),
	}
}

// RegisterResource adds a resource or data source to the registry.
func (r *ResourceRegistry) RegisterResource(info *ResourceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info.IsDataSource {
		r.dataSources[info.Name] = info
	} else {
		r.resources[info.Name] = info
	}
}

// RegisterTestFile adds a test file to the registry, associating it with a resource.
func (r *ResourceRegistry) RegisterTestFile(info *TestFileInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.testFiles[info.ResourceName] = info
}

// GetResource retrieves a resource by name from the registry.
func (r *ResourceRegistry) GetResource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resources[name]
}

// GetDataSource retrieves a data source by name from the registry.
func (r *ResourceRegistry) GetDataSource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dataSources[name]
}

// GetTestFile retrieves test file information for a given resource name.
func (r *ResourceRegistry) GetTestFile(resourceName string) *TestFileInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.testFiles[resourceName]
}

// GetUntestedResources returns all resources and data sources that lack test coverage.
func (r *ResourceRegistry) GetUntestedResources() []*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var untested []*ResourceInfo
	for name, resource := range r.resources {
		if _, hasTest := r.testFiles[name]; !hasTest {
			untested = append(untested, resource)
		}
	}
	for name, dataSource := range r.dataSources {
		if _, hasTest := r.testFiles[name]; !hasTest {
			untested = append(untested, dataSource)
		}
	}
	return untested
}

// GetAllResources returns a copy of all resources (thread-safe).
func (r *ResourceRegistry) GetAllResources() map[string]*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := make(map[string]*ResourceInfo, len(r.resources))
	for k, v := range r.resources {
		copy[k] = v
	}
	return copy
}

// GetAllDataSources returns a copy of all data sources (thread-safe).
func (r *ResourceRegistry) GetAllDataSources() map[string]*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := make(map[string]*ResourceInfo, len(r.dataSources))
	for k, v := range r.dataSources {
		copy[k] = v
	}
	return copy
}

// GetAllTestFiles returns a copy of all test files (thread-safe).
func (r *ResourceRegistry) GetAllTestFiles() map[string]*TestFileInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	copy := make(map[string]*TestFileInfo, len(r.testFiles))
	for k, v := range r.testFiles {
		copy[k] = v
	}
	return copy
}

// ResourceInfo holds metadata about a Terraform resource or data source
// discovered through AST parsing of provider code.
type ResourceInfo struct {
	Name           string
	IsDataSource   bool
	FilePath       string
	SchemaPos      token.Pos
	Attributes     []AttributeInfo
	HasImportState bool
	ImportStatePos token.Pos
}

// AttributeInfo represents a single attribute from a resource schema.
type AttributeInfo struct {
	Name           string
	Type           string
	Required       bool
	Optional       bool
	Computed       bool
	IsUpdatable    bool
	HasValidators  bool
	ValidatorTypes []string
}

// NeedsUpdateTest returns true if the attribute is optional and updatable.
func (a *AttributeInfo) NeedsUpdateTest() bool {
	return a.Optional && a.IsUpdatable
}

// NeedsValidationTest returns true if the attribute has validators or is required.
func (a *AttributeInfo) NeedsValidationTest() bool {
	return a.HasValidators || a.Required
}

// TestFileInfo represents parsed information from a test file.
type TestFileInfo struct {
	FilePath      string
	ResourceName  string
	IsDataSource  bool
	TestFunctions []TestFunctionInfo
}

// TestFunctionInfo represents a single TestAcc function and its test steps.
type TestFunctionInfo struct {
	Name             string
	FunctionPos      token.Pos
	UsesResourceTest bool
	TestSteps        []TestStepInfo
	HasErrorCase     bool
	HasImportStep    bool
}

// TestStepInfo represents a single step within a resource.TestCase.
type TestStepInfo struct {
	StepNumber        int
	StepPos           token.Pos // Position of this step in the source code
	Config            string
	HasConfig         bool // True if Config field exists (even if not a literal)
	HasCheck          bool
	CheckFunctions    []string
	ImportState       bool
	ImportStateVerify bool
	ExpectError       bool
}

// IsUpdateStep returns true if this is not the first step and has a config.
func (t *TestStepInfo) IsUpdateStep() bool {
	return t.StepNumber > 0 && t.HasConfig
}

// IsValidImportStep returns true if this step properly tests ImportState.
func (t *TestStepInfo) IsValidImportStep() bool {
	return t.ImportState && t.ImportStateVerify
}

// TestFileSearchResult represents a test file that was searched for a resource.
type TestFileSearchResult struct {
	FilePath string
	Found    bool
}

// TestFunctionMatchInfo represents a test function found during analysis with its match status.
type TestFunctionMatchInfo struct {
	Name        string
	Line        int
	MatchStatus string // "matched" or "not_matched"
	MatchReason string // Why it didn't match (empty if matched)
}

// VerboseDiagnosticInfo holds detailed diagnostic information for verbose output.
type VerboseDiagnosticInfo struct {
	ResourceName       string
	ResourceType       string // "resource" or "data source"
	ResourceFile       string
	ResourceLine       int
	TestFilesSearched  []TestFileSearchResult
	TestFunctionsFound []TestFunctionMatchInfo
	ExpectedPatterns   []string
	FoundPattern       string
	SuggestedFixes     []string
}

// ClassifyTestFunctionMatch determines if a test function matches a resource and provides
// a reason if it doesn't match.
func ClassifyTestFunctionMatch(funcName string, resourceName string) (status string, reason string) {
	titleName := toTitleCase(resourceName)

	// Check for exact match patterns
	matchPatterns := []string{
		"TestAcc" + titleName,
		"TestAccResource" + titleName,
		"TestAccDataSource" + titleName,
		"TestResource" + titleName,
		"TestDataSource" + titleName,
	}

	for _, pattern := range matchPatterns {
		if strings.HasPrefix(funcName, pattern) {
			return "matched", ""
		}
	}

	// Check if it has TestAcc prefix but wrong resource name
	if strings.HasPrefix(funcName, "TestAcc") {
		return "not_matched", "does not match resource '" + resourceName + "'"
	}

	// Check if it has Test prefix but missing Acc
	if strings.HasPrefix(funcName, "Test") && !strings.HasPrefix(funcName, "TestAcc") {
		// Check if it looks like it's for this resource
		lowerFunc := strings.ToLower(funcName)
		lowerResource := strings.ReplaceAll(resourceName, "_", "")
		if strings.Contains(lowerFunc, lowerResource) {
			return "not_matched", "missing 'Acc' prefix"
		}
		return "not_matched", "does not match resource '" + resourceName + "'"
	}

	return "not_matched", "does not follow test naming convention"
}

// BuildVerboseDiagnosticInfo creates a VerboseDiagnosticInfo for a resource.
func BuildVerboseDiagnosticInfo(resource *ResourceInfo, registry *ResourceRegistry) VerboseDiagnosticInfo {
	resourceType := "resource"
	if resource.IsDataSource {
		resourceType = "data source"
	}

	info := VerboseDiagnosticInfo{
		ResourceName: resource.Name,
		ResourceType: resourceType,
		ResourceFile: resource.FilePath,
		ResourceLine: 0, // Will be set by caller with actual position
	}

	// Build expected test file path
	expectedTestPath := BuildExpectedTestPath(resource)

	// Check if test file exists in registry
	testFile := registry.GetTestFile(resource.Name)
	if testFile != nil {
		info.TestFilesSearched = []TestFileSearchResult{
			{FilePath: testFile.FilePath, Found: true},
		}

		// Analyze test functions
		for _, testFunc := range testFile.TestFunctions {
			status, reason := ClassifyTestFunctionMatch(testFunc.Name, resource.Name)
			info.TestFunctionsFound = append(info.TestFunctionsFound, TestFunctionMatchInfo{
				Name:        testFunc.Name,
				Line:        0, // Line info not available in TestFunctionInfo currently
				MatchStatus: status,
				MatchReason: reason,
			})
		}
	} else {
		info.TestFilesSearched = []TestFileSearchResult{
			{FilePath: expectedTestPath, Found: false},
		}
	}

	// Build expected patterns
	titleName := toTitleCase(resource.Name)
	if resource.IsDataSource {
		info.ExpectedPatterns = []string{
			"TestAccDataSource" + titleName + "*",
			"TestDataSource" + titleName + "*",
		}
	} else {
		info.ExpectedPatterns = []string{
			"TestAcc" + titleName + "*",
			"TestAccResource" + titleName + "*",
			"TestResource" + titleName + "*",
		}
	}

	// Build suggested fixes
	info.SuggestedFixes = buildSuggestedFixes(resource, testFile)

	return info
}

// buildSuggestedFixes creates suggested fix messages for a resource missing tests.
func buildSuggestedFixes(resource *ResourceInfo, testFile *TestFileInfo) []string {
	var fixes []string
	expectedFunc := BuildExpectedTestFunc(resource)

	if testFile == nil {
		// No test file exists
		expectedPath := BuildExpectedTestPath(resource)
		fixes = append(fixes, fmt.Sprintf("Create test file %s with function %s", filepath.Base(expectedPath), expectedFunc))
	} else if len(testFile.TestFunctions) == 0 {
		// Test file exists but no test functions
		fixes = append(fixes, fmt.Sprintf("Add acceptance test function %s to %s", expectedFunc, filepath.Base(testFile.FilePath)))
	} else {
		// Test functions exist but don't match naming convention
		fixes = append(fixes, fmt.Sprintf("Option 1: Rename tests to follow convention (%s)", expectedFunc))
		fixes = append(fixes, "Option 2: Configure custom test patterns in .golangci.yml:\n      test-name-patterns:\n        - \"Test"+toTitleCase(resource.Name)+"\"")
	}

	return fixes
}

// FormatVerboseDiagnostic formats a VerboseDiagnosticInfo into a human-readable string.
func FormatVerboseDiagnostic(info VerboseDiagnosticInfo) string {
	var sb strings.Builder

	// Resource Location section
	sb.WriteString("\n  Resource Location:\n")
	sb.WriteString(fmt.Sprintf("    %s: %s:%d\n", info.ResourceType, info.ResourceFile, info.ResourceLine))

	// Test Files Searched section
	sb.WriteString("\n  Test Files Searched:\n")
	for _, tf := range info.TestFilesSearched {
		status := "not found"
		if tf.Found {
			status = "found"
		}
		sb.WriteString(fmt.Sprintf("    - %s (%s)\n", filepath.Base(tf.FilePath), status))
	}

	// Test Functions Found section (only if there are any)
	if len(info.TestFunctionsFound) > 0 {
		sb.WriteString("\n  Test Functions Found:\n")
		for _, tf := range info.TestFunctionsFound {
			matchStatus := "MATCHED"
			if tf.MatchStatus == "not_matched" {
				matchStatus = fmt.Sprintf("NOT MATCHED (%s)", tf.MatchReason)
			}
			sb.WriteString(fmt.Sprintf("    - %s (line %d) - %s\n", tf.Name, tf.Line, matchStatus))
		}
	}

	// Why Not Matched section (only if there are expected patterns)
	if len(info.ExpectedPatterns) > 0 {
		sb.WriteString("\n  Why Not Matched:\n")
		sb.WriteString(fmt.Sprintf("    Expected pattern: %s\n", strings.Join(info.ExpectedPatterns, " or ")))
		if info.FoundPattern != "" {
			sb.WriteString(fmt.Sprintf("    Found pattern: %s\n", info.FoundPattern))
		}
	}

	// Suggested Fix section
	if len(info.SuggestedFixes) > 0 {
		sb.WriteString("\n  Suggested Fix:\n")
		for _, fix := range info.SuggestedFixes {
			sb.WriteString(fmt.Sprintf("    %s\n", fix))
		}
	}

	return sb.String()
}

// parseResources extracts all resources and data sources from a Go source file
// by analyzing Schema() method implementations.
func parseResources(file *ast.File, fset *token.FileSet, filePath string) []*ResourceInfo {
	var resources []*ResourceInfo

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for Schema() method implementations
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Schema" {
			return true
		}

		// Extract receiver type name (e.g., *WidgetResource -> WidgetResource)
		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType == "" {
			return true
		}

		// Check if it's a framework resource or data source
		isDataSource := strings.HasSuffix(recvType, "DataSource")
		isResource := strings.HasSuffix(recvType, "Resource")

		if !isResource && !isDataSource {
			return true
		}

		// Extract resource name from type name
		name := extractResourceName(recvType)
		if name == "" {
			return true
		}

		resource := &ResourceInfo{
			Name:         name,
			IsDataSource: isDataSource,
			FilePath:     filePath,
			SchemaPos:    funcDecl.Pos(),
			Attributes:   extractAttributes(funcDecl.Body),
		}

		resources = append(resources, resource)
		return true
	})

	// Check for ImportState method
	for _, resource := range resources {
		if !resource.IsDataSource {
			resource.HasImportState = hasImportStateMethod(file, resource.Name)
		}
	}

	return resources
}

func getReceiverTypeName(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}

	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func extractResourceName(typeName string) string {
	// Remove "Resource" or "DataSource" suffix
	name := strings.TrimSuffix(typeName, "Resource")
	name = strings.TrimSuffix(name, "DataSource")

	// Convert CamelCase to snake_case
	return toSnakeCase(name)
}

func toSnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			// Add underscore before uppercase if:
			// 1. Previous char is lowercase, OR
			// 2. Next char exists and is lowercase (handles acronyms like "HTTPServer" -> "http_server")
			prev := runes[i-1]
			if unicode.IsLower(prev) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// toTitleCase converts snake_case to TitleCase (e.g., "my_resource" -> "MyResource")
func toTitleCase(s string) string {
	var result strings.Builder
	capitalizeNext := true
	for _, r := range s {
		if r == '_' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// formatResourceLocation formats the resource location for enhanced issue reporting.
// Returns a string like "Resource: /path/to/file.go:45"
func formatResourceLocation(pass *analysis.Pass, resource *ResourceInfo) string {
	pos := pass.Fset.Position(resource.SchemaPos)
	return fmt.Sprintf("Resource: %s:%d", pos.Filename, pos.Line)
}

func extractAttributes(body *ast.BlockStmt) []AttributeInfo {
	var attributes []AttributeInfo
	if body == nil {
		return attributes
	}

	// Find the schema.Schema composite literal
	ast.Inspect(body, func(n ast.Node) bool {
		// Look for CompositeLit that represents schema.Schema{}
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if this is schema.Schema type
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if sel.Sel.Name != "Schema" {
				return true
			}
		}

		// Find the Attributes field in schema.Schema
		for _, elt := range compLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// Check if this is the Attributes field
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Attributes" {
				// Parse the attributes map
				if mapLit, ok := kv.Value.(*ast.CompositeLit); ok {
					attributes = parseAttributesMap(mapLit)
				}
			}
		}

		return false // Don't recurse into nested schemas
	})

	return attributes
}

func parseAttributesMap(mapLit *ast.CompositeLit) []AttributeInfo {
	var attributes []AttributeInfo

	for _, elt := range mapLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		// Get attribute name
		var attrName string
		if lit, ok := kv.Key.(*ast.BasicLit); ok {
			attrName = strings.Trim(lit.Value, `"`)
		}

		if attrName == "" {
			continue
		}

		// Parse attribute properties
		attr := AttributeInfo{
			Name:        attrName,
			IsUpdatable: true, // Default to updatable unless RequiresReplace found
		}

		// Parse the attribute composite literal
		if attrLit, ok := kv.Value.(*ast.CompositeLit); ok {
			for _, attrElt := range attrLit.Elts {
				attrKV, ok := attrElt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}

				fieldName := ""
				if ident, ok := attrKV.Key.(*ast.Ident); ok {
					fieldName = ident.Name
				}

				switch fieldName {
				case "Required":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Required = ident.Name == "true"
					}
				case "Optional":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Optional = ident.Name == "true"
					}
				case "Computed":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Computed = ident.Name == "true"
					}
				case "PlanModifiers":
					// Check if RequiresReplace is present
					attr.IsUpdatable = !hasRequiresReplace(attrKV.Value)
				case "Validators":
					// Check for validators
					if compLit, ok := attrKV.Value.(*ast.CompositeLit); ok {
						attr.HasValidators = len(compLit.Elts) > 0
					}
				}
			}

			// Determine attribute type from the composite literal type
			if sel, ok := attrLit.Type.(*ast.SelectorExpr); ok {
				attr.Type = sel.Sel.Name
			}
		}

		attributes = append(attributes, attr)
	}

	return attributes
}

func hasRequiresReplace(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		// Look for function calls like stringplanmodifier.RequiresReplace()
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if the function name contains "RequiresReplace"
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if strings.Contains(sel.Sel.Name, "RequiresReplace") {
				found = true
				return false
			}
		}

		return true
	})
	return found
}

func hasImportStateMethod(file *ast.File, resourceName string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "ImportState" {
			return true
		}

		if funcDecl.Recv != nil {
			recvType := getReceiverTypeName(funcDecl.Recv)
			// Use toTitleCase instead of deprecated strings.Title
			// toTitleCase properly converts snake_case to TitleCase
			expectedType := toTitleCase(resourceName) + "Resource"
			if recvType == expectedType || recvType == "*"+expectedType {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// T016: Test file parser - now supports multiple naming conventions
// parseTestFile parses a test file and extracts test function information.
// It delegates to parseTestFileWithHelpers with no custom helpers.
func parseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	return parseTestFileWithHelpers(file, fset, filePath, nil)
}

// parseTestFileWithHelpers parses a test file with support for custom test helpers.
// customHelpers specifies additional test helper function names that wrap resource.Test().
func parseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
	var testFuncs []TestFunctionInfo
	var resourceName string
	isDataSource := false

	// Try to extract resource name from filename first (more reliable)
	// e.g., resource_widget_test.go -> widget, data_source_http_test.go -> http
	baseName := filepath.Base(filePath)
	if strings.HasSuffix(baseName, "_test.go") {
		nameWithoutTest := strings.TrimSuffix(baseName, "_test.go")
		if strings.HasPrefix(nameWithoutTest, "resource_") {
			resourceName = strings.TrimPrefix(nameWithoutTest, "resource_")
		} else if strings.HasPrefix(nameWithoutTest, "data_source_") {
			resourceName = strings.TrimPrefix(nameWithoutTest, "data_source_")
			isDataSource = true
		} else if strings.HasPrefix(nameWithoutTest, "ephemeral_") {
			// Handle ephemeral resources (e.g., ephemeral_private_key)
			resourceName = strings.TrimPrefix(nameWithoutTest, "ephemeral_")
		} else if strings.HasSuffix(nameWithoutTest, "_resource") {
			// Handle reversed naming: group_resource_test.go -> group
			resourceName = strings.TrimSuffix(nameWithoutTest, "_resource")
		} else if strings.HasSuffix(nameWithoutTest, "_data_source") {
			// Handle reversed naming: inventory_data_source_test.go -> inventory
			resourceName = strings.TrimSuffix(nameWithoutTest, "_data_source")
			isDataSource = true
		} else if strings.HasSuffix(nameWithoutTest, "_datasource") {
			// Handle reversed naming: group_datasource_test.go -> group
			resourceName = strings.TrimSuffix(nameWithoutTest, "_datasource")
			isDataSource = true
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		name := funcDecl.Name.Name

		// Check if this is a test function using flexible matching
		if !isTestFunction(name, nil) {
			return true
		}

		// Try to extract resource name from function name if not found from filename
		if resourceName == "" {
			resourceName = extractResourceNameFromTestFunc(name)
		}

		// Determine if data source test
		if strings.Contains(strings.ToLower(name), "datasource") ||
			strings.Contains(name, "DataSource") {
			isDataSource = true
		}

		// Check if test uses resource.Test() or custom test helpers
		usesResourceTest := checkUsesResourceTestWithHelpers(funcDecl.Body, customHelpers)
		if !usesResourceTest {
			return true
		}

		testFunc := TestFunctionInfo{
			Name:             funcDecl.Name.Name,
			FunctionPos:      funcDecl.Pos(),
			UsesResourceTest: true,
			TestSteps:        extractTestSteps(funcDecl.Body),
		}

		// Check for error cases and import steps
		for _, step := range testFunc.TestSteps {
			if step.ExpectError {
				testFunc.HasErrorCase = true
			}
			if step.ImportState {
				testFunc.HasImportStep = true
			}
		}

		testFuncs = append(testFuncs, testFunc)
		return true
	})

	if len(testFuncs) == 0 {
		return nil
	}

	return &TestFileInfo{
		FilePath:      filePath,
		ResourceName:  resourceName,
		IsDataSource:  isDataSource,
		TestFunctions: testFuncs,
	}
}

// extractResourceNameFromTestFunc extracts the resource name from various test function naming patterns.
// Returns empty string if no resource name can be extracted - caller should rely on filename in this case.
func extractResourceNameFromTestFunc(funcName string) string {
	// Try various patterns in order of specificity
	patterns := []struct {
		prefix string
	}{
		{"TestAccDataSource"},
		{"TestAccResource"},
		{"TestAcc"},
		{"TestDataSource"},
		{"TestResource"},
		{"Test"},
	}

	for _, p := range patterns {
		if strings.HasPrefix(funcName, p.prefix) {
			name := strings.TrimPrefix(funcName, p.prefix)
			if name == "" {
				continue
			}

			// Skip if the remaining name starts with underscore (e.g., TestDataSource_200)
			// This means the resource name is not in the function name
			if strings.HasPrefix(name, "_") {
				// For patterns like TestDataSource_200, there's no resource name in the function
				// Fall through to try shorter prefixes, but we should not extract from generic "Test" prefix
				if p.prefix == "Test" {
					// We've tried all specific patterns, give up
					return ""
				}
				continue
			}

			// Split on underscore to get just the resource name part
			parts := strings.SplitN(name, "_", 2)
			if len(parts) > 0 && parts[0] != "" {
				resourcePart := parts[0]

				// Skip if the resource part is a type identifier (DataSource, Resource)
				// This happens when we fall through from a more specific pattern to "Test"
				// e.g., TestDataSource_200 -> after removing "Test" we get "DataSource_200"
				if resourcePart == "DataSource" || resourcePart == "Resource" {
					return ""
				}

				// Handle patterns like "PrivateKeyRSA" -> extract "PrivateKey"
				// We need to split CamelCase and take just the first meaningful part
				extracted := extractResourceFromCamelCase(resourcePart)
				if extracted != "" {
					return toSnakeCase(extracted)
				}
			}
		}
	}

	// Fallback: If the function starts with Test and has CamelCase parts after,
	// try to extract the first CamelCase word as a potential resource name.
	// This handles non-standard patterns like TestMyCustomResource_something
	if strings.HasPrefix(funcName, "Test") && len(funcName) > 4 {
		remainder := funcName[4:] // Remove "Test"
		if len(remainder) > 0 && unicode.IsUpper(rune(remainder[0])) {
			// Extract first CamelCase word (up to first underscore or lowercase transition)
			firstWord := extractFirstCamelCaseWord(remainder)
			if firstWord != "" && firstWord != "Acc" && firstWord != "Resource" && firstWord != "DataSource" {
				return toSnakeCase(firstWord)
			}
		}
	}

	return ""
}

// extractFirstCamelCaseWord extracts the first CamelCase word from a string.
// For "PrivateKeyRSA_basic" returns "PrivateKey", for "Widget_test" returns "Widget".
func extractFirstCamelCaseWord(s string) string {
	if s == "" {
		return ""
	}

	var result strings.Builder
	runes := []rune(s)

	for i, r := range runes {
		// Stop at underscore
		if r == '_' {
			break
		}

		// Stop at second uppercase letter that starts a new word
		// (but not if we're at the start or if previous was also uppercase - handles acronyms)
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			// If previous was lowercase, this starts a new word
			if unicode.IsLower(prev) {
				break
			}
			// If previous was uppercase and next is lowercase, this starts a new word (e.g., RSAKey)
			if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				break
			}
		}

		result.WriteRune(r)
	}

	word := result.String()
	// Return empty if we only got an acronym (all uppercase, 3 or fewer chars)
	if len(word) <= 3 && word == strings.ToUpper(word) {
		return ""
	}

	return word
}

// extractResourceFromCamelCase extracts the resource name from a CamelCase string,
// removing known suffixes like RSA, ECDSA, ED25519, etc.
func extractResourceFromCamelCase(s string) string {
	if s == "" {
		return ""
	}

	// Known algorithm/type suffixes to strip
	suffixes := []string{
		"RSA",
		"ECDSA",
		"ED25519",
		"SHA256",
		"SHA384",
		"SHA512",
		"V1",
		"V2",
		"V3",
	}

	result := s
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = strings.TrimSuffix(result, suffix)
			break
		}
	}

	// If we stripped everything or nothing remains, return original
	if result == "" {
		return s
	}

	return result
}

// checkUsesResourceTest checks if a function body contains a call to resource.Test()
// or any of the custom test helper functions.
func checkUsesResourceTest(body *ast.BlockStmt) bool {
	return checkUsesResourceTestWithHelpers(body, nil)
}

// checkUsesResourceTestWithHelpers checks if a function body contains a call to resource.Test()
// or any of the custom test helper functions specified in customHelpers.
// customHelpers format: ["package.Function", "otherpackage.OtherFunc"]
func checkUsesResourceTestWithHelpers(body *ast.BlockStmt, customHelpers []string) bool {
	if body == nil {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for resource.Test() call
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				// Check standard resource.Test() or resource.ParallelTest()
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					found = true
					return false
				}

				// Check custom test helpers
				for _, helper := range customHelpers {
					parts := strings.SplitN(helper, ".", 2)
					if len(parts) == 2 {
						if ident.Name == parts[0] && sel.Sel.Name == parts[1] {
							found = true
							return false
						}
					}
				}
			}
		}
		return true
	})
	return found
}

func extractTestSteps(body *ast.BlockStmt) []TestStepInfo {
	var steps []TestStepInfo
	if body == nil {
		return steps
	}
	stepNumber := 0

	// Find resource.Test() call and extract Steps
	ast.Inspect(body, func(n ast.Node) bool {
		// Look for resource.Test() call
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if it's resource.Test
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && sel.Sel.Name == "Test" {
					// Found resource.Test() - parse TestCase argument
					if len(call.Args) >= 2 {
						// Second argument is the TestCase
						if testCase, ok := call.Args[1].(*ast.CompositeLit); ok {
							steps = parseTestCase(testCase, &stepNumber)
						}
					}
					return false
				}
			}
		}

		return true
	})

	return steps
}

func parseTestCase(testCase *ast.CompositeLit, stepNumber *int) []TestStepInfo {
	var steps []TestStepInfo

	// Find the Steps field
	for _, elt := range testCase.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Steps" {
			// Parse the Steps slice
			if stepsLit, ok := kv.Value.(*ast.CompositeLit); ok {
				for _, stepElt := range stepsLit.Elts {
					if stepLit, ok := stepElt.(*ast.CompositeLit); ok {
						step := parseTestStep(stepLit, *stepNumber)
						steps = append(steps, step)
						*stepNumber++
					}
				}
			}
		}
	}

	return steps
}

func parseTestStep(stepLit *ast.CompositeLit, stepNumber int) TestStepInfo {
	step := TestStepInfo{
		StepNumber: stepNumber,
		StepPos:    stepLit.Pos(),
	}

	for _, elt := range stepLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		fieldName := ""
		if ident, ok := kv.Key.(*ast.Ident); ok {
			fieldName = ident.Name
		}

		switch fieldName {
		case "Config":
			// Set HasConfig regardless of whether value is a literal or function call
			step.HasConfig = true
			// Extract config string if it's a literal
			if lit, ok := kv.Value.(*ast.BasicLit); ok {
				step.Config = strings.Trim(lit.Value, "`\"")
			}
		case "Check":
			step.HasCheck = true
			// Extract check function names
			step.CheckFunctions = extractCheckFunctions(kv.Value)
		case "ImportState":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.ImportState = ident.Name == "true"
			}
		case "ImportStateVerify":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.ImportStateVerify = ident.Name == "true"
			}
		case "ExpectError":
			// Can be bool or regex
			step.ExpectError = true
		}
	}

	return step
}

func extractCheckFunctions(node ast.Node) []string {
	var funcs []string

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Look for TestCheck* functions
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if strings.HasPrefix(sel.Sel.Name, "TestCheck") {
				funcs = append(funcs, sel.Sel.Name)
			}
		}

		return true
	})

	return funcs
}

// Plugin implements the golangci-lint plugin interface.
type Plugin struct {
	settings Settings
}

// New creates a new plugin instance with the given settings.
func New(settings any) (register.LinterPlugin, error) {
	s := DefaultSettings()
	if settings != nil {
		decoded, err := register.DecodeSettings[Settings](settings)
		if err != nil {
			return nil, fmt.Errorf("failed to decode settings: %w", err)
		}
		s = decoded
	}
	return &Plugin{settings: s}, nil
}

// BuildAnalyzers returns the list of enabled analyzers based on settings.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	var analyzers []*analysis.Analyzer

	if p.settings.EnableBasicTest {
		analyzers = append(analyzers, BasicTestAnalyzer)
	}
	if p.settings.EnableUpdateTest {
		analyzers = append(analyzers, UpdateTestAnalyzer)
	}
	if p.settings.EnableImportTest {
		analyzers = append(analyzers, ImportTestAnalyzer)
	}
	if p.settings.EnableErrorTest {
		analyzers = append(analyzers, ErrorTestAnalyzer)
	}
	if p.settings.EnableStateCheck {
		analyzers = append(analyzers, StateCheckAnalyzer)
	}

	return analyzers, nil
}

// GetLoadMode returns the AST load mode required by the analyzers.
func (p *Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func init() {
	register.Plugin("tfprovidertest", New)
}

// BasicTestAnalyzer detects resources and data sources lacking acceptance tests.
var BasicTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-basic-test",
	Doc:  "Checks that every resource and data source has at least one acceptance test.",
	Run:  runBasicTestAnalyzer,
}

// UpdateTestAnalyzer validates that resources with updatable attributes have multi-step tests.
var UpdateTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-update-test",
	Doc:  "Checks that resources with updatable attributes have multi-step update tests.",
	Run:  runUpdateTestAnalyzer,
}

// ImportTestAnalyzer ensures resources with ImportState methods have import tests.
var ImportTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-import-test",
	Doc:  "Checks that resources implementing ImportState have import tests.",
	Run:  runImportTestAnalyzer,
}

// ErrorTestAnalyzer checks that resources with validators have error case tests.
var ErrorTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-error-cases",
	Doc:  "Checks that resources with validation rules have error case tests.",
	Run:  runErrorTestAnalyzer,
}

// StateCheckAnalyzer validates that test steps include state validation check functions.
var StateCheckAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-check-functions",
	Doc:  "Checks that test steps include state validation check functions.",
	Run:  runStateCheckAnalyzer,
}

// buildRegistry creates a ResourceRegistry populated with all resources and test files
// from the analysis pass. This helper reduces code duplication across analyzers.
func buildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
	registry := NewResourceRegistry()

	// First pass: collect all resources and data sources
	for _, file := range pass.Files {
		filePath := pass.Fset.Position(file.Pos()).Filename

		// Skip test files
		if strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		// Skip base class files if configured
		if settings.ExcludeBaseClasses && isBaseClassFile(filePath) {
			continue
		}

		// Skip sweeper files if configured (test infrastructure for cleanup)
		if settings.ExcludeSweeperFiles && IsSweeperFile(filePath) {
			continue
		}

		// Skip migration files if configured (state migration utilities)
		if settings.ExcludeMigrationFiles && IsMigrationFile(filePath) {
			continue
		}

		// Parse resources from this file
		resources := parseResources(file, pass.Fset, filePath)
		for _, res := range resources {
			registry.RegisterResource(res)
		}
	}

	// Second pass: collect all test files
	for _, file := range pass.Files {
		filePath := pass.Fset.Position(file.Pos()).Filename

		// Only process test files
		if !strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		// Parse test file with custom test helpers support
		testFile := parseTestFileWithHelpers(file, pass.Fset, filePath, settings.CustomTestHelpers)
		if testFile != nil && testFile.ResourceName != "" {
			registry.RegisterTestFile(testFile)
		}
	}

	return registry
}

// runBasicTestAnalyzer implements User Story 1: Basic Test Coverage
// Detects resources and data sources that lack basic acceptance tests
func runBasicTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := buildRegistry(pass, settings)

	// Third pass: report untested resources with enhanced location information
	untested := registry.GetUntestedResources()
	for _, resource := range untested {
		resourceType := "resource"
		resourceTypeTitle := "Resource"
		if resource.IsDataSource {
			resourceType = "data source"
			resourceTypeTitle = "Data source"
		}

		// Build enhanced message with location details
		pos := pass.Fset.Position(resource.SchemaPos)
		expectedTestPath := BuildExpectedTestPath(resource)
		expectedTestFunc := BuildExpectedTestFunc(resource)

		msg := fmt.Sprintf("%s '%s' has no acceptance test\n  %s: %s:%d\n  Expected test file: %s\n  Expected test function: %s",
			resourceType, resource.Name,
			resourceTypeTitle, pos.Filename, pos.Line,
			expectedTestPath, expectedTestFunc)

		pass.Reportf(resource.SchemaPos, "%s", msg)
	}

	// Also check for resources with test files but no TestAcc functions
	allResources := registry.GetAllResources()
	allTestFiles := registry.GetAllTestFiles()
	for name, resource := range allResources {
		if testFile, exists := allTestFiles[name]; exists {
			if len(testFile.TestFunctions) == 0 {
				pos := pass.Fset.Position(resource.SchemaPos)
				expectedTestFunc := BuildExpectedTestFunc(resource)

				msg := fmt.Sprintf("resource '%s' has test file but no TestAcc functions\n  Resource: %s:%d\n  Test file: %s\n  Expected test function: %s",
					name, pos.Filename, pos.Line,
					testFile.FilePath, expectedTestFunc)

				pass.Reportf(resource.SchemaPos, "%s", msg)
			}
		}
	}

	allDataSources := registry.GetAllDataSources()
	for name, dataSource := range allDataSources {
		if testFile, exists := allTestFiles[name]; exists {
			if len(testFile.TestFunctions) == 0 {
				pos := pass.Fset.Position(dataSource.SchemaPos)
				expectedTestFunc := BuildExpectedTestFunc(dataSource)

				msg := fmt.Sprintf("data source '%s' has test file but no TestAcc functions\n  Data source: %s:%d\n  Test file: %s\n  Expected test function: %s",
					name, pos.Filename, pos.Line,
					testFile.FilePath, expectedTestFunc)

				pass.Reportf(dataSource.SchemaPos, "%s", msg)
			}
		}
	}

	return nil, nil
}

func runUpdateTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := buildRegistry(pass, settings)

	// Check for resources with updatable attributes but no update tests
	// Only check regular resources (not data sources)
	for name, resource := range registry.GetAllResources() {
		// Check if resource has updatable attributes
		hasUpdatable := false
		for _, attr := range resource.Attributes {
			if attr.NeedsUpdateTest() {
				hasUpdatable = true
				break
			}
		}

		if !hasUpdatable {
			// Resource doesn't need update tests
			continue
		}

		// Check if resource has test file with multi-step test
		testFile := registry.GetTestFile(name)
		if testFile == nil {
			// No test file at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if any test function has multiple steps
		hasUpdateTest := false
		for _, testFunc := range testFile.TestFunctions {
			if len(testFunc.TestSteps) >= 2 {
				// Multi-step test found
				hasUpdateTest = true
				break
			}
		}

		if !hasUpdateTest {
			pass.Reportf(resource.SchemaPos, "resource '%s' has updatable attributes but no update test coverage", name)
		}
	}

	return nil, nil
}

func runImportTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := buildRegistry(pass, settings)

	// Check for resources with ImportState but no import tests
	// Only check regular resources (not data sources)
	for name, resource := range registry.GetAllResources() {
		// Only check resources that implement ImportState
		if !resource.HasImportState {
			continue
		}

		// Check if resource has test file with import test step
		testFile := registry.GetTestFile(name)
		if testFile == nil {
			// No test file at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if any test function has an import step
		hasImportTest := false
		for _, testFunc := range testFile.TestFunctions {
			if testFunc.HasImportStep {
				hasImportTest = true
				break
			}
		}

		if !hasImportTest {
			pass.Reportf(resource.SchemaPos, "resource '%s' implements ImportState but has no import test coverage", name)
		}
	}

	return nil, nil
}

func runErrorTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := buildRegistry(pass, settings)

	// Check for resources with validation rules but no error tests
	for name, resource := range registry.GetAllResources() {
		// Check if resource has validation rules
		hasValidation := false
		for _, attr := range resource.Attributes {
			if attr.NeedsValidationTest() {
				hasValidation = true
				break
			}
		}

		if !hasValidation {
			// Resource doesn't need error tests
			continue
		}

		// Check if resource has test file with error test
		testFile := registry.GetTestFile(name)
		if testFile == nil {
			// No test file at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if any test function has an error case
		hasErrorTest := false
		for _, testFunc := range testFile.TestFunctions {
			if testFunc.HasErrorCase {
				hasErrorTest = true
				break
			}
		}

		if !hasErrorTest {
			pass.Reportf(resource.SchemaPos, "resource '%s' has validation rules but no error case tests", name)
		}
	}

	return nil, nil
}

func runStateCheckAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := buildRegistry(pass, settings)

	// Check for test steps without Check fields
	for _, testFile := range registry.GetAllTestFiles() {
		for _, testFunc := range testFile.TestFunctions {
			for _, step := range testFunc.TestSteps {
				// Skip import and error test steps - they don't require Check
				if step.ImportState || step.ExpectError {
					continue
				}

				// Regular test steps should have Check fields
				if !step.HasCheck {
					// Find the resource to get its name for the error message
					resourceName := testFile.ResourceName
					if resource := registry.GetResource(resourceName); resource != nil {
						pass.Reportf(resource.SchemaPos, "test step for resource '%s' has no state validation checks", resourceName)
					}
				}
			}
		}
	}

	return nil, nil
}
