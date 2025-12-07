// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Regex to find 'resource "example_widget" "name" {'
// Captures the resource type (e.g., "example_widget")
var resourceTypeRegex = regexp.MustCompile(`resource\s+"([^"]+)"\s+"[^"]+"\s+\{`)

// LocalHelper represents a discovered local test helper function.
type LocalHelper struct {
	Name     string
	FilePath string
	FuncDecl *ast.FuncDecl
}

// ParserConfig holds all configuration options for parsing test files.
// This struct consolidates the various parameters that were previously
// spread across multiple parseTestFile* functions.
type ParserConfig struct {
	CustomHelpers    []string      // Custom test helper functions (e.g., "mypackage.AccTest")
	LocalHelpers     []LocalHelper // Local test helper functions discovered in the codebase
	TestNamePatterns []string      // Custom test name patterns (e.g., "TestAcc*", "TestResource*")
}

// DefaultParserConfig returns a ParserConfig with default/empty values.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		CustomHelpers:    nil,
		LocalHelpers:     nil,
		TestNamePatterns: nil,
	}
}

// ExclusionResult tracks why a file was excluded from analysis.
type ExclusionResult struct {
	FilePath       string
	Excluded       bool
	Reason         string
	MatchedPattern string
}

// ExclusionDiagnostics collects information about all excluded files.
type ExclusionDiagnostics struct {
	ExcludedFiles []ExclusionResult
	TotalExcluded int
}

// hashConfigExpr generates a hash of a config expression for comparison.
// This normalizes the AST representation to detect equivalent configs.
func hashConfigExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	var buf strings.Builder
	fset := token.NewFileSet()

	// Print the AST node to a string for hashing
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}

	// Normalize whitespace
	normalized := strings.Join(strings.Fields(buf.String()), " ")

	// Generate hash
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
}

// parseResources extracts all resources and data sources from a Go source file.
func parseResources(file *ast.File, fset *token.FileSet, filePath string) []*ResourceInfo {
	var resources []*ResourceInfo

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Schema" {
			return true
		}

		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType == "" {
			return true
		}

		isDataSource := strings.HasSuffix(recvType, "DataSource")
		isResource := strings.HasSuffix(recvType, "Resource")

		if !isResource && !isDataSource {
			return true
		}

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

	for _, resource := range resources {
		if !resource.IsDataSource {
			resource.HasImportState = hasImportStateMethod(file, resource.Name)
		}
	}

	return resources
}

// ParseTestFileWithConfig parses a test file with full configuration support.
// This is the main parsing function that all other parse functions delegate to.
func ParseTestFileWithConfig(file *ast.File, fset *token.FileSet, filePath string, config ParserConfig) *TestFileInfo {
	packageName := ""
	if file.Name != nil {
		packageName = file.Name.Name
	}

	resourceName, isDataSource := extractResourceNameFromFilePath(filePath)

	var testFuncs []TestFunctionInfo

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		name := funcDecl.Name.Name

		// Check if function name matches test patterns
		if !matchesTestPattern(name, config.TestNamePatterns) {
			return true
		}

		usesResourceTest := checkUsesResourceTestWithLocalHelpers(funcDecl.Body, config.CustomHelpers, config.LocalHelpers)
		if !usesResourceTest {
			return true
		}

		steps, hasCheckDestroy, inferred := extractTestSteps(funcDecl.Body)
		testFunc := TestFunctionInfo{
			Name:              funcDecl.Name.Name,
			FilePath:          filePath,
			FunctionPos:       funcDecl.Pos(),
			UsesResourceTest:  true,
			TestSteps:         steps,
			HelperUsed:        detectHelperUsed(funcDecl.Body, config.LocalHelpers),
			HasCheckDestroy:   hasCheckDestroy,
			InferredResources: inferred,
		}

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

	// Return TestFileInfo even if no resource name extracted from filename
	// Resource association now happens via the Linker in Phase 3
	if len(testFuncs) == 0 {
		return nil
	}

	return &TestFileInfo{
		FilePath:      filePath,
		PackageName:   packageName,
		ResourceName:  resourceName,
		IsDataSource:  isDataSource,
		TestFunctions: testFuncs,
	}
}

// parseTestFile parses a test file and extracts test function information.
// Deprecated: Use ParseTestFileWithConfig with DefaultParserConfig() instead.
func parseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	return ParseTestFileWithConfig(file, fset, filePath, DefaultParserConfig())
}

// parseTestFileWithHelpers parses a test file with support for custom test helpers.
// Deprecated: Use ParseTestFileWithConfig instead.
func parseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
	config := ParserConfig{
		CustomHelpers: customHelpers,
	}
	return ParseTestFileWithConfig(file, fset, filePath, config)
}

// extractResourceNameFromFilePath extracts resource name from file path.
// This function delegates to ExtractResourceNameFromPath for the actual extraction logic.
func extractResourceNameFromFilePath(filePath string) (string, bool) {
	return ExtractResourceNameFromPath(filePath)
}

// findLocalTestHelpers discovers functions that wrap resource.Test().
func findLocalTestHelpers(files []*ast.File, fset *token.FileSet) []LocalHelper {
	var helpers []LocalHelper

	for _, file := range files {
		filePath := fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				return true
			}

			name := funcDecl.Name.Name
			if strings.HasPrefix(name, "Test") {
				return true
			}
			if len(name) == 0 || (name[0] >= 'a' && name[0] <= 'z') {
				return true
			}
			if !acceptsTestingT(funcDecl) {
				return true
			}
			if !checkUsesResourceTest(funcDecl.Body) {
				return true
			}

			helpers = append(helpers, LocalHelper{
				Name:     name,
				FilePath: filePath,
				FuncDecl: funcDecl,
			})

			return true
		})
	}

	return helpers
}

// acceptsTestingT checks if a function has *testing.T as a parameter.
func acceptsTestingT(funcDecl *ast.FuncDecl) bool {
	if funcDecl == nil || funcDecl.Type == nil || funcDecl.Type.Params == nil {
		return false
	}

	for _, param := range funcDecl.Type.Params.List {
		if starExpr, ok := param.Type.(*ast.StarExpr); ok {
			if selExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
				if ident, ok := selExpr.X.(*ast.Ident); ok {
					if ident.Name == "testing" && selExpr.Sel.Name == "T" {
						return true
					}
				}
			}
		}
	}

	return false
}

// matchesExcludePattern checks if a file should be excluded.
func matchesExcludePattern(filePath string, patterns []string) ExclusionResult {
	baseName := filepath.Base(filePath)

	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, baseName); matched {
			return ExclusionResult{
				FilePath:       filePath,
				Excluded:       true,
				Reason:         "matched exclusion pattern",
				MatchedPattern: pattern,
			}
		}
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return ExclusionResult{
				FilePath:       filePath,
				Excluded:       true,
				Reason:         "matched exclusion pattern (full path)",
				MatchedPattern: pattern,
			}
		}
	}

	return ExclusionResult{FilePath: filePath, Excluded: false}
}

// buildRegistry constructs a resource registry by scanning all files.
// It uses a three-phase approach:
//  1. Scan for Resources (Type-based discovery via AST)
//  2. Scan ALL Test Files (unconditionally, to support function-first matching)
//  3. Link tests to resources using the Linker (function name, file proximity, fuzzy)
func buildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
	reg := NewResourceRegistry()

	// Discover local test helpers first
	localHelpers := findLocalTestHelpers(pass.Files, pass.Fset)

	// PHASE 1: Scan for Resources (Type-based discovery via AST)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if settings.ExcludeBaseClasses && isBaseClassFile(filename) {
			continue
		}
		if settings.ExcludeSweeperFiles && IsSweeperFile(filename) {
			continue
		}
		if settings.ExcludeMigrationFiles && IsMigrationFile(filename) {
			continue
		}
		if shouldExcludeFile(filename, settings.ExcludePaths) {
			continue
		}
		// Check custom exclude patterns
		if len(settings.ExcludePatterns) > 0 {
			if result := matchesExcludePattern(filename, settings.ExcludePatterns); result.Excluded {
				continue
			}
		}

		resources := parseResources(file, pass.Fset, filename)
		for _, resource := range resources {
			reg.RegisterResource(resource)
		}
	}

	// PHASE 2: Scan ALL Test Files (unconditionally)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Skip sweeper test files
		if settings.ExcludeSweeperFiles && IsSweeperFile(filename) {
			continue
		}

		// Check custom exclude patterns
		if len(settings.ExcludePatterns) > 0 {
			if result := matchesExcludePattern(filename, settings.ExcludePatterns); result.Excluded {
				continue
			}
		}

		// Parse test file with custom and local helpers and test name patterns
		config := ParserConfig{
			CustomHelpers:    settings.CustomTestHelpers,
			LocalHelpers:     localHelpers,
			TestNamePatterns: settings.TestNamePatterns,
		}
		testFileInfo := ParseTestFileWithConfig(file, pass.Fset, filename, config)
		if testFileInfo == nil {
			continue
		}

		// Register each test function in global index
		for i := range testFileInfo.TestFunctions {
			fn := &testFileInfo.TestFunctions[i]
			fn.FilePath = filename
			reg.RegisterTestFunction(fn)
		}
	}

	// PHASE 3: Link tests to resources using the Linker
	linker := NewLinker(reg, settings)
	linker.LinkTestsToResources()

	return reg
}

// parseTestFileWithHelpersAndLocals parses a test file with both custom and local helpers.
// Deprecated: Use ParseTestFileWithConfig instead.
func parseTestFileWithHelpersAndLocals(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string, localHelpers []LocalHelper) *TestFileInfo {
	config := ParserConfig{
		CustomHelpers: customHelpers,
		LocalHelpers:  localHelpers,
	}
	return ParseTestFileWithConfig(file, fset, filePath, config)
}

// parseTestFileWithSettings parses a test file with full settings support.
// Deprecated: Use ParseTestFileWithConfig instead. This function is kept for backward compatibility.
func parseTestFileWithSettings(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string, localHelpers []LocalHelper, testNamePatterns []string) *TestFileInfo {
	config := ParserConfig{
		CustomHelpers:    customHelpers,
		LocalHelpers:     localHelpers,
		TestNamePatterns: testNamePatterns,
	}
	return ParseTestFileWithConfig(file, fset, filePath, config)
}

// matchesTestPattern checks if a function name matches the test patterns.
// If testNamePatterns is empty, it uses default patterns (TestAcc*, TestResource*, etc.)
func matchesTestPattern(funcName string, testNamePatterns []string) bool {
	// Always require "Test" prefix (capital T for exported tests)
	if !strings.HasPrefix(funcName, "Test") {
		return false
	}

	// If custom patterns are provided, check against them
	if len(testNamePatterns) > 0 {
		for _, pattern := range testNamePatterns {
			// Support glob-style patterns (* as wildcard)
			if strings.HasSuffix(pattern, "*") {
				prefix := strings.TrimSuffix(pattern, "*")
				if strings.HasPrefix(funcName, prefix) {
					return true
				}
			} else if funcName == pattern {
				return true
			}
		}
		return false
	}

	// Default patterns: TestAcc*, TestResource*, TestDataSource*, Test*_
	defaultPatterns := []string{
		"TestAcc",
		"TestResource",
		"TestDataSource",
	}

	for _, pattern := range defaultPatterns {
		if strings.HasPrefix(funcName, pattern) {
			return true
		}
	}

	// Also accept Test*_ pattern (e.g., TestWidget_basic)
	if strings.Contains(funcName, "_") {
		return true
	}

	return false
}

// MatchesTestPattern is the public API for checking test patterns.
func MatchesTestPattern(funcName string, testNamePatterns []string) bool {
	return matchesTestPattern(funcName, testNamePatterns)
}

// checkUsesResourceTest checks if a function body contains a call to resource.Test()
func checkUsesResourceTest(body *ast.BlockStmt) bool {
	return checkUsesResourceTestWithHelpers(body, nil)
}

// checkUsesResourceTestWithHelpers checks if a function body contains a call to resource.Test()
// or any of the custom test helper functions.
func checkUsesResourceTestWithHelpers(body *ast.BlockStmt, customHelpers []string) bool {
	return checkUsesResourceTestWithLocalHelpers(body, customHelpers, nil)
}

// checkUsesResourceTestWithLocalHelpers checks if a function body contains a call to resource.Test(),
// custom helpers, or local helpers.
func checkUsesResourceTestWithLocalHelpers(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper) bool {
	if body == nil {
		return false
	}

	localHelperNames := make(map[string]bool)
	for _, h := range localHelpers {
		localHelperNames[h.Name] = true
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					found = true
					return false
				}

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

		if ident, ok := call.Fun.(*ast.Ident); ok {
			if localHelperNames[ident.Name] {
				found = true
				return false
			}
		}

		return true
	})
	return found
}

// detectHelperUsed determines which helper function is used in a test function body.
func detectHelperUsed(body *ast.BlockStmt, localHelpers []LocalHelper) string {
	if body == nil {
		return ""
	}

	localHelperNames := make(map[string]bool)
	for _, h := range localHelpers {
		localHelperNames[h.Name] = true
	}

	var helperUsed string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					helperUsed = "resource." + sel.Sel.Name
					return false
				}
			}
		}

		if ident, ok := call.Fun.(*ast.Ident); ok {
			if localHelperNames[ident.Name] {
				helperUsed = ident.Name
				return false
			}
		}

		return true
	})

	return helperUsed
}

// extractTestSteps extracts test step information from a test function body.
// Returns the list of test steps, a boolean indicating whether CheckDestroy was found,
// and a slice of inferred resource names extracted from Config strings.
func extractTestSteps(body *ast.BlockStmt) ([]TestStepInfo, bool, []string) {
	var steps []TestStepInfo
	hasCheckDestroy := false
	uniqueInferred := make(map[string]bool)
	if body == nil {
		return steps, hasCheckDestroy, nil
	}
	stepNumber := 0

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					if len(call.Args) >= 2 {
						steps, hasCheckDestroy = extractStepsFromTestCase(call.Args[1], &stepNumber, uniqueInferred)
					}
					return false
				}
			}
		}
		return true
	})

	// Convert uniqueInferred map keys to slice
	var inferredResources []string
	for resourceName := range uniqueInferred {
		inferredResources = append(inferredResources, resourceName)
	}

	return steps, hasCheckDestroy, inferredResources
}

// extractStepsFromTestCase extracts steps from a resource.TestCase composite literal.
// Returns the list of test steps and a boolean indicating whether CheckDestroy was found.
// The inferred map is populated with resource names extracted from Config strings.
func extractStepsFromTestCase(testCaseExpr ast.Expr, stepNumber *int, inferred map[string]bool) ([]TestStepInfo, bool) {
	var steps []TestStepInfo
	hasCheckDestroy := false

	compLit, ok := testCaseExpr.(*ast.CompositeLit)
	if !ok {
		return steps, hasCheckDestroy
	}

	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "CheckDestroy":
			hasCheckDestroy = true
		case "Steps":
			stepsLit, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}

			// Parse each step with hash
			for _, stepExpr := range stepsLit.Elts {
				step := parseTestStepWithHash(stepExpr, *stepNumber, inferred)
				steps = append(steps, step)
				*stepNumber++
			}
		}
	}

	// Second pass: Determine update steps
	for i := range steps {
		if i > 0 {
			steps[i].PreviousConfigHash = steps[i-1].ConfigHash
			steps[i].IsUpdateStepFlag = steps[i].DetermineIfUpdateStep(&steps[i-1])
		}
	}

	return steps, hasCheckDestroy
}

// parseTestStep parses a single TestStep composite literal (backward compatible)
func parseTestStep(stepExpr ast.Expr, stepNum int) TestStepInfo {
	return parseTestStepWithHash(stepExpr, stepNum, nil)
}

// parseTestStepWithHash parses a single TestStep composite literal and computes its config hash.
// If inferred is non-nil, resource names extracted from Config strings are added to it.
func parseTestStepWithHash(stepExpr ast.Expr, stepNum int, inferred map[string]bool) TestStepInfo {
	step := TestStepInfo{
		StepNumber: stepNum,
	}

	stepLit, ok := stepExpr.(*ast.CompositeLit)
	if !ok {
		return step
	}

	if len(stepLit.Elts) > 0 {
		step.StepPos = stepLit.Elts[0].Pos()
	}

	for _, elt := range stepLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "Config":
			step.HasConfig = true
			step.ConfigHash = hashConfigExpr(kv.Value)

			// Extract resource name from Config string
			if inferred != nil {
				if basicLit, ok := kv.Value.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					// Remove backticks or quotes
					configContent := strings.Trim(basicLit.Value, "`\"")
					matches := resourceTypeRegex.FindAllStringSubmatch(configContent, -1)
					for _, match := range matches {
						if len(match) > 1 {
							inferred[match[1]] = true
						}
					}
				}
				// Note: We intentionally skip dynamic configs (fmt.Sprintf) for now
			}
		case "Check":
			step.HasCheck = true
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
			step.ExpectError = true
		case "ConfigPlanChecks":
			step.HasPlanCheck = true
		}
	}

	return step
}

// extractCheckFunctions extracts check function names from a Check field
func extractCheckFunctions(checkExpr ast.Expr) []string {
	var functions []string

	ast.Inspect(checkExpr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			functions = append(functions, sel.Sel.Name)
		}

		return true
	})

	return functions
}

// CheckHasSweepers scans a file for resource.AddTestSweepers calls.
// This is typically found in TestMain or init() functions.
func CheckHasSweepers(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "resource" && sel.Sel.Name == "AddTestSweepers" {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

// Public API functions

// ParseResources is the public API for parsing resources from a file.
func ParseResources(file *ast.File, fset *token.FileSet, filePath string) []*ResourceInfo {
	return parseResources(file, fset, filePath)
}

// ParseTestFile is the public API for parsing test files.
func ParseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	return parseTestFile(file, fset, filePath)
}

// ParseTestFileWithHelpers is the public API for parsing test files with custom helpers.
func ParseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
	return parseTestFileWithHelpers(file, fset, filePath, customHelpers)
}

// FindLocalTestHelpers is the public API for discovering local test helpers.
func FindLocalTestHelpers(files []*ast.File, fset *token.FileSet) []LocalHelper {
	return findLocalTestHelpers(files, fset)
}

// AcceptsTestingT is the public API for checking if a function accepts *testing.T.
func AcceptsTestingT(funcDecl *ast.FuncDecl) bool {
	return acceptsTestingT(funcDecl)
}

// MatchesExcludePattern is the public API for checking if a file should be excluded.
func MatchesExcludePattern(filePath string, patterns []string) ExclusionResult {
	return matchesExcludePattern(filePath, patterns)
}

// CheckUsesResourceTestWithLocalHelpers is the public API for checking helper usage.
func CheckUsesResourceTestWithLocalHelpers(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper) bool {
	return checkUsesResourceTestWithLocalHelpers(body, customHelpers, localHelpers)
}

// DetectHelperUsed is the public API for detecting helper function usage.
func DetectHelperUsed(body *ast.BlockStmt, localHelpers []LocalHelper) string {
	return detectHelperUsed(body, localHelpers)
}

// HashConfigExpr is the public API for hashing config expressions.
func HashConfigExpr(expr ast.Expr) string {
	return hashConfigExpr(expr)
}
