package tfprovidertest

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"sync"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// T011: Settings struct with defaults
type Settings struct {
	EnableBasicTest        bool     `yaml:"enable-basic-test"`
	EnableUpdateTest       bool     `yaml:"enable-update-test"`
	EnableImportTest       bool     `yaml:"enable-import-test"`
	EnableErrorTest        bool     `yaml:"enable-error-test"`
	EnableStateCheck       bool     `yaml:"enable-state-check"`
	ResourcePathPattern    string   `yaml:"resource-path-pattern"`
	DataSourcePathPattern  string   `yaml:"data-source-path-pattern"`
	TestFilePattern        string   `yaml:"test-file-pattern"`
	ExcludePaths           []string `yaml:"exclude-paths"`
}

// Default settings enable all analyzers
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

// T012: ResourceRegistry with thread-safe maps
type ResourceRegistry struct {
	mu          sync.RWMutex
	Resources   map[string]*ResourceInfo
	DataSources map[string]*ResourceInfo
	TestFiles   map[string]*TestFileInfo
}

func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		Resources:   make(map[string]*ResourceInfo),
		DataSources: make(map[string]*ResourceInfo),
		TestFiles:   make(map[string]*TestFileInfo),
	}
}

func (r *ResourceRegistry) RegisterResource(info *ResourceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info.IsDataSource {
		r.DataSources[info.Name] = info
	} else {
		r.Resources[info.Name] = info
	}
}

func (r *ResourceRegistry) RegisterTestFile(info *TestFileInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.TestFiles[info.ResourceName] = info
}

func (r *ResourceRegistry) GetResource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Resources[name]
}

func (r *ResourceRegistry) GetDataSource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.DataSources[name]
}

func (r *ResourceRegistry) GetTestFile(resourceName string) *TestFileInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.TestFiles[resourceName]
}

func (r *ResourceRegistry) GetUntestedResources() []*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var untested []*ResourceInfo
	for name, resource := range r.Resources {
		if _, hasTest := r.TestFiles[name]; !hasTest {
			untested = append(untested, resource)
		}
	}
	for name, dataSource := range r.DataSources {
		if _, hasTest := r.TestFiles[name]; !hasTest {
			untested = append(untested, dataSource)
		}
	}
	return untested
}

// Data model entities from data-model.md
type ResourceInfo struct {
	Name           string
	IsDataSource   bool
	FilePath       string
	SchemaPos      token.Pos
	Attributes     []AttributeInfo
	HasImportState bool
	ImportStatePos token.Pos
}

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

func (a *AttributeInfo) NeedsUpdateTest() bool {
	return a.Optional && a.IsUpdatable
}

func (a *AttributeInfo) NeedsValidationTest() bool {
	return a.HasValidators || a.Required
}

type TestFileInfo struct {
	FilePath      string
	ResourceName  string
	IsDataSource  bool
	TestFunctions []TestFunctionInfo
}

type TestFunctionInfo struct {
	Name             string
	FunctionPos      token.Pos
	UsesResourceTest bool
	TestSteps        []TestStepInfo
	HasErrorCase     bool
	HasImportStep    bool
}

type TestStepInfo struct {
	StepNumber        int
	Config            string
	HasCheck          bool
	CheckFunctions    []string
	ImportState       bool
	ImportStateVerify bool
	ExpectError       bool
}

func (t *TestStepInfo) IsUpdateStep() bool {
	return t.StepNumber > 0 && t.Config != ""
}

func (t *TestStepInfo) IsValidImportStep() bool {
	return t.ImportState && t.ImportStateVerify
}

// T013-T015: AST parser implementation for detecting resources and attributes
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
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func extractAttributes(body *ast.BlockStmt) []AttributeInfo {
	// Simplified attribute extraction
	// In a full implementation, this would parse the schema.Schema structure
	var attributes []AttributeInfo
	// Placeholder - full implementation would traverse the AST to find Attributes map
	return attributes
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
			expectedType := strings.Title(resourceName) + "Resource"
			if recvType == expectedType || recvType == "*"+expectedType {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// T016: Test file parser
func parseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	var testFuncs []TestFunctionInfo
	var resourceName string
	isDataSource := false

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || !strings.HasPrefix(funcDecl.Name.Name, "TestAcc") {
			return true
		}

		// Extract resource name from test function name
		// TestAccResourceWidget_basic -> widget
		// TestAccDataSourceAccount_basic -> account
		name := funcDecl.Name.Name
		if strings.HasPrefix(name, "TestAccDataSource") {
			isDataSource = true
			parts := strings.SplitN(strings.TrimPrefix(name, "TestAccDataSource"), "_", 2)
			if len(parts) > 0 && resourceName == "" {
				resourceName = toSnakeCase(parts[0])
			}
		} else if strings.HasPrefix(name, "TestAccResource") {
			isDataSource = false
			parts := strings.SplitN(strings.TrimPrefix(name, "TestAccResource"), "_", 2)
			if len(parts) > 0 && resourceName == "" {
				resourceName = toSnakeCase(parts[0])
			}
		}

		// Check if test uses resource.Test()
		usesResourceTest := checkUsesResourceTest(funcDecl.Body)
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

func checkUsesResourceTest(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for resource.Test() call
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && sel.Sel.Name == "Test" {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func extractTestSteps(body *ast.BlockStmt) []TestStepInfo {
	// Simplified test step extraction
	// Full implementation would parse the TestCase struct and extract Steps
	var steps []TestStepInfo
	// Placeholder - would traverse AST to find Steps field
	return steps
}

// T017: Plugin struct with required methods
type Plugin struct {
	settings Settings
}

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

func (p *Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

// T018: Plugin registration
func init() {
	register.Plugin("tfprovidertest", New)
}

// Analyzer placeholders - will be implemented in user story phases
var BasicTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-basic-test",
	Doc:  "Checks that every resource and data source has at least one acceptance test.",
	Run:  runBasicTestAnalyzer,
}

var UpdateTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-update-test",
	Doc:  "Checks that resources with updatable attributes have multi-step update tests.",
	Run:  runUpdateTestAnalyzer,
}

var ImportTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-resource-import-test",
	Doc:  "Checks that resources implementing ImportState have import tests.",
	Run:  runImportTestAnalyzer,
}

var ErrorTestAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-error-cases",
	Doc:  "Checks that resources with validation rules have error case tests.",
	Run:  runErrorTestAnalyzer,
}

var StateCheckAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-check-functions",
	Doc:  "Checks that test steps include state validation check functions.",
	Run:  runStateCheckAnalyzer,
}

// runBasicTestAnalyzer implements User Story 1: Basic Test Coverage
// Detects resources and data sources that lack basic acceptance tests
func runBasicTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	registry := NewResourceRegistry()

	// First pass: collect all resources and data sources
	for _, file := range pass.Files {
		filePath := pass.Fset.Position(file.Pos()).Filename

		// Skip test files
		if strings.HasSuffix(filePath, "_test.go") {
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

		// Parse test file
		testFile := parseTestFile(file, pass.Fset, filePath)
		if testFile != nil && testFile.ResourceName != "" {
			registry.RegisterTestFile(testFile)
		}
	}

	// Third pass: report untested resources
	untested := registry.GetUntestedResources()
	for _, resource := range untested {
		resourceType := "resource"
		if resource.IsDataSource {
			resourceType = "data source"
		}

		pass.Reportf(resource.SchemaPos, "%s '%s' has no acceptance test file", resourceType, resource.Name)
	}

	// Also check for resources with test files but no TestAcc functions
	for name, resource := range registry.Resources {
		if testFile, exists := registry.TestFiles[name]; exists {
			if len(testFile.TestFunctions) == 0 {
				pass.Reportf(resource.SchemaPos, "resource '%s' has test file but no TestAcc functions", name)
			}
		}
	}

	for name, dataSource := range registry.DataSources {
		if testFile, exists := registry.TestFiles[name]; exists {
			if len(testFile.TestFunctions) == 0 {
				pass.Reportf(dataSource.SchemaPos, "data source '%s' has test file but no TestAcc functions", name)
			}
		}
	}

	return nil, nil
}

func runUpdateTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// To be implemented in Phase 4 (User Story 2)
	return nil, nil
}

func runImportTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// To be implemented in Phase 5 (User Story 3)
	return nil, nil
}

func runErrorTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// To be implemented in Phase 6 (User Story 4)
	return nil, nil
}

func runStateCheckAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// To be implemented in Phase 7 (User Story 5)
	return nil, nil
}

// Helper function to check if file path matches pattern
func matchesPattern(filePath, pattern string) bool {
	matched, _ := filepath.Match(pattern, filepath.Base(filePath))
	return matched
}
