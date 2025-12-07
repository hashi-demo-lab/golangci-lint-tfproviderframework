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

// Regex to find 'resource "example_widget" "name" {' or 'action "example_action" "name" {'
// Captures the resource/action type (e.g., "example_widget", "aap_eda_eventstream_post")
// This handles both standard resources and terraform-plugin-framework actions
var resourceTypeRegex = regexp.MustCompile(`(?:resource|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)

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

// parseResources extracts all resources, data sources, and actions from a Go source file.
// It uses multiple detection strategies:
// 1. Schema() method on types ending with Resource/DataSource/Action
// 2. MetadataEntitySlug in factory functions (NewXxxDataSource, NewXxxResource)
// 3. Metadata() method with resp.TypeName assignment
// 4. NewXxxAction factory functions returning action.Action
func parseResources(file *ast.File, fset *token.FileSet, filePath string) []*ResourceInfo {
	var resources []*ResourceInfo
	// Use compound key "kind:name" to allow same name across different kinds
	// (e.g., "job" resource and "job" action can coexist)
	seen := make(map[string]bool)

	seenKey := func(kind ResourceKind, name string) string {
		return kind.String() + ":" + name
	}

	// Strategy 1: Look for Schema() methods on Resource/DataSource/Action types
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Schema" {
			return true
		}

		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType == "" {
			return true
		}

		var kind ResourceKind
		isDataSource := strings.HasSuffix(recvType, "DataSource")
		isResource := strings.HasSuffix(recvType, "Resource")
		isAction := strings.HasSuffix(recvType, "Action")

		if isDataSource {
			kind = KindDataSource
		} else if isAction {
			// Skip actions in Strategy 1 - they're handled by Strategy 4/4b
			// which properly extracts the TypeName from Metadata method
			return true
		} else if isResource {
			kind = KindResource
		} else {
			return true
		}

		name := extractResourceName(recvType)
		key := seenKey(kind, name)
		if name == "" || seen[key] {
			return true
		}

		seen[key] = true
		resource := &ResourceInfo{
			Name:         name,
			Kind:         kind,
			IsDataSource: isDataSource,
			IsAction:     isAction,
			FilePath:     filePath,
			SchemaPos:    funcDecl.Pos(),
			Attributes:   extractAttributes(funcDecl.Body),
		}

		resources = append(resources, resource)
		return true
	})

	// Strategy 2: Look for NewXxxDataSource/NewXxxResource factory functions
	// with MetadataEntitySlug in StringDescriptions struct
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil {
			return true
		}

		funcName := funcDecl.Name.Name
		isDataSource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "DataSource")
		isResource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "Resource") && !strings.Contains(funcName, "DataSource")

		if !isDataSource && !isResource {
			return true
		}

		// Look for MetadataEntitySlug in the function body
		if funcDecl.Body != nil {
			name := extractMetadataEntitySlug(funcDecl.Body)
			kind := KindResource
			if isDataSource {
				kind = KindDataSource
			}
			key := seenKey(kind, name)
			if name != "" && !seen[key] {
				seen[key] = true
				resources = append(resources, &ResourceInfo{
					Name:         name,
					Kind:         kind,
					IsDataSource: isDataSource,
					FilePath:     filePath,
					SchemaPos:    funcDecl.Pos(),
				})
			}
		}

		return true
	})

	// Strategy 3: Look for Metadata() methods with resp.TypeName assignment
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Metadata" {
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

		// Look for resp.TypeName = "..." assignment
		if funcDecl.Body != nil {
			name := extractTypeNameFromMetadata(funcDecl.Body)
			kind := KindResource
			if isDataSource {
				kind = KindDataSource
			}
			key := seenKey(kind, name)
			if name != "" && !seen[key] {
				seen[key] = true
				resources = append(resources, &ResourceInfo{
					Name:         name,
					Kind:         kind,
					IsDataSource: isDataSource,
					FilePath:     filePath,
					SchemaPos:    funcDecl.Pos(),
				})
			}
		}

		return true
	})

	// Strategy 4: Look for NewXxxAction factory functions returning action.Action
	// Also collect action type names for later Metadata extraction
	actionTypeNames := make(map[string]token.Pos) // typeName -> position
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil {
			return true
		}

		funcName := funcDecl.Name.Name
		// Match patterns like NewJobAction, NewWorkflowJobAction, NewEDAEventStreamPostAction
		if !strings.HasPrefix(funcName, "New") || !strings.HasSuffix(funcName, "Action") {
			return true
		}

		// Verify return type is action.Action
		if funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) == 0 {
			return true
		}

		returnType := ""
		if sel, ok := funcDecl.Type.Results.List[0].Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				returnType = ident.Name + "." + sel.Sel.Name
			}
		}

		if returnType != "action.Action" {
			return true
		}

		// Extract action type name from factory function (e.g., NewJobAction -> JobAction)
		typeName := strings.TrimPrefix(funcName, "New")
		actionTypeNames[typeName] = funcDecl.Pos()

		return true
	})

	// Strategy 4b: For each action type, find its Metadata method and extract TypeName
	// This gives us the canonical name used in HCL configs (e.g., "eda_eventstream_post")
	// Track which action types we've already processed via Metadata
	processedActionTypes := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Metadata" || funcDecl.Recv == nil {
			return true
		}

		// Check receiver type against known action types
		recvType := getReceiverTypeName(funcDecl.Recv)
		pos, isAction := actionTypeNames[recvType]
		if !isAction {
			return true
		}

		// Mark this action type as processed (don't use fallback)
		processedActionTypes[recvType] = true

		// Extract TypeName from Metadata method body
		if funcDecl.Body != nil {
			name := extractTypeNameFromMetadata(funcDecl.Body)
			key := seenKey(KindAction, name)
			if name != "" && !seen[key] {
				seen[key] = true
				resources = append(resources, &ResourceInfo{
					Name:      name,
					Kind:      KindAction,
					IsAction:  true,
					FilePath:  filePath,
					SchemaPos: pos,
				})
			}
		}

		return true
	})

	// Fallback: For actions without Metadata methods, use the factory function name
	for typeName, pos := range actionTypeNames {
		// Skip if we already processed this via Metadata
		if processedActionTypes[typeName] {
			continue
		}
		name := extractActionName("New" + typeName)
		key := seenKey(KindAction, name)
		if name != "" && !seen[key] {
			seen[key] = true
			resources = append(resources, &ResourceInfo{
				Name:      name,
				Kind:      KindAction,
				IsAction:  true,
				FilePath:  filePath,
				SchemaPos: pos,
			})
		}
	}

	for _, resource := range resources {
		if resource.Kind == KindResource {
			resource.HasImportState = hasImportStateMethod(file, resource.Name)
		}
	}

	return resources
}

// extractActionName extracts the action name from a factory function name.
// Examples: NewJobAction -> job, NewWorkflowJobAction -> workflow_job
// Note: Actions may share base names with resources (e.g., both "job" resource and "job" action exist).
// The registry uses Kind to differentiate them.
func extractActionName(funcName string) string {
	// Remove "New" prefix and "Action" suffix
	name := strings.TrimPrefix(funcName, "New")
	name = strings.TrimSuffix(name, "Action")
	if name == "" {
		return ""
	}

	// Convert PascalCase to snake_case
	return toSnakeCase(name)
}

// extractMetadataEntitySlug extracts the resource name from MetadataEntitySlug in a function body.
// It looks for patterns like: MetadataEntitySlug: "organization"
func extractMetadataEntitySlug(body *ast.BlockStmt) string {
	var name string
	ast.Inspect(body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}

		// Check if key is MetadataEntitySlug
		if ident, ok := kv.Key.(*ast.Ident); ok {
			if ident.Name == "MetadataEntitySlug" {
				// Extract string value
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					name = strings.Trim(lit.Value, `"`)
					return false
				}
			}
		}
		return true
	})
	return name
}

// extractTypeNameFromMetadata extracts the resource name from resp.TypeName assignment.
// It looks for patterns like: resp.TypeName = "provider_name" or resp.TypeName = req.ProviderTypeName + "_name"
func extractTypeNameFromMetadata(body *ast.BlockStmt) string {
	var name string
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		// Check for resp.TypeName on LHS
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "resp" || sel.Sel.Name != "TypeName" {
			return true
		}

		// Try to extract the resource name from RHS
		switch rhs := assign.Rhs[0].(type) {
		case *ast.BasicLit:
			// Direct string assignment: resp.TypeName = "resource_name"
			if rhs.Kind == token.STRING {
				fullName := strings.Trim(rhs.Value, `"`)
				// Remove provider prefix if present (e.g., "provider_resource" -> "resource")
				if idx := strings.Index(fullName, "_"); idx > 0 {
					name = fullName[idx+1:]
				} else {
					name = fullName
				}
				return false
			}
		case *ast.BinaryExpr:
			// Concatenation: resp.TypeName = req.ProviderTypeName + "_name"
			if rhs.Op == token.ADD {
				if lit, ok := rhs.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					suffix := strings.Trim(lit.Value, `"`)
					// Remove leading underscore if present
					name = strings.TrimPrefix(suffix, "_")
					return false
				}
			}
		}
		return true
	})
	return name
}

// ParseTestFileWithConfig parses a test file with full configuration support.
// This is the main parsing function that all other parse functions delegate to.
func ParseTestFileWithConfig(file *ast.File, fset *token.FileSet, filePath string, config ParserConfig) *TestFileInfo {
	packageName := ""
	if file.Name != nil {
		packageName = file.Name.Name
	}

	resourceName, isDataSource := extractResourceNameFromFilePath(filePath)

	// Build helper function map: function name -> resource/action patterns in return strings
	helperPatterns := buildHelperPatternMap(file)

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

		steps, hasCheckDestroy, inferred := extractTestStepsWithHelpers(funcDecl.Body, helperPatterns)
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

// buildHelperPatternMap scans a file for helper functions that return HCL strings
// and extracts resource/action patterns from them.
func buildHelperPatternMap(file *ast.File) map[string][]string {
	patterns := make(map[string][]string)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return true
		}

		funcName := funcDecl.Name.Name

		// Look for return statements with string literals or fmt.Sprintf
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok || len(ret.Results) == 0 {
				return true
			}

			for _, result := range ret.Results {
				extractPatternsFromExpr(result, func(pattern string) {
					patterns[funcName] = append(patterns[funcName], pattern)
				})
			}
			return true
		})
		return true
	})

	return patterns
}

// extractPatternsFromExpr extracts resource/action patterns from an expression.
// It handles string literals, fmt.Sprintf calls, and string concatenation.
func extractPatternsFromExpr(expr ast.Expr, addPattern func(string)) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			content := strings.Trim(e.Value, "`\"")
			matches := resourceTypeRegex.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				if len(match) > 1 {
					addPattern(match[1])
				}
			}
		}
	case *ast.CallExpr:
		// Handle fmt.Sprintf and similar
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "fmt" {
				// Look at the format string (first argument)
				if len(e.Args) > 0 {
					extractPatternsFromExpr(e.Args[0], addPattern)
				}
			}
		}
	case *ast.BinaryExpr:
		// Handle string concatenation
		if e.Op == token.ADD {
			extractPatternsFromExpr(e.X, addPattern)
			extractPatternsFromExpr(e.Y, addPattern)
		}
	}
}

// extractTestStepsWithHelpers is like extractTestSteps but also looks up helper patterns.
func extractTestStepsWithHelpers(body *ast.BlockStmt, helperPatterns map[string][]string) ([]TestStepInfo, bool, []string) {
	var steps []TestStepInfo
	var hasCheckDestroy bool
	uniqueInferred := make(map[string]bool)
	stepNumber := 1

	ast.Inspect(body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || (ident.Name != "resource" && sel.Sel.Name != "Test" && sel.Sel.Name != "ParallelTest") {
			return true
		}

		// Find TestCase argument
		if len(callExpr.Args) < 2 {
			return true
		}

		testSteps, foundCheckDestroy := extractStepsFromTestCaseWithHelpers(callExpr.Args[1], &stepNumber, uniqueInferred, helperPatterns)
		steps = append(steps, testSteps...)
		if foundCheckDestroy {
			hasCheckDestroy = true
		}

		return true
	})

	var inferredResources []string
	for resourceName := range uniqueInferred {
		inferredResources = append(inferredResources, resourceName)
	}

	return steps, hasCheckDestroy, inferredResources
}

// extractStepsFromTestCaseWithHelpers extracts steps and looks up helper patterns.
func extractStepsFromTestCaseWithHelpers(testCaseExpr ast.Expr, stepNumber *int, inferred map[string]bool, helperPatterns map[string][]string) ([]TestStepInfo, bool) {
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

			for _, stepExpr := range stepsLit.Elts {
				step := parseTestStepWithHashAndHelpers(stepExpr, *stepNumber, inferred, helperPatterns)
				steps = append(steps, step)
				*stepNumber++
			}
		}
	}

	for i := range steps {
		if i > 0 {
			steps[i].PreviousConfigHash = steps[i-1].ConfigHash
			steps[i].IsUpdateStepFlag = steps[i].DetermineIfUpdateStep(&steps[i-1])
		}
	}

	return steps, hasCheckDestroy
}

// parseTestStepWithHashAndHelpers parses a step and looks up helper patterns for Config.
func parseTestStepWithHashAndHelpers(stepExpr ast.Expr, stepNum int, inferred map[string]bool, helperPatterns map[string][]string) TestStepInfo {
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

			if inferred != nil {
				// Try direct extraction first
				extractResourceNamesFromConfigValue(kv.Value, inferred)

				// If Config is a function call, look up helper patterns
				if callExpr, ok := kv.Value.(*ast.CallExpr); ok {
					if ident, ok := callExpr.Fun.(*ast.Ident); ok {
						if patterns, exists := helperPatterns[ident.Name]; exists {
							for _, p := range patterns {
								inferred[p] = true
							}
						}
					}
				}
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
		}
	}

	return step
}

// extractResourceNamesFromConfigValue extracts resource/action names from a Config value.
func extractResourceNamesFromConfigValue(expr ast.Expr, inferred map[string]bool) {
	extractPatternsFromExpr(expr, func(pattern string) {
		inferred[pattern] = true
	})
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
