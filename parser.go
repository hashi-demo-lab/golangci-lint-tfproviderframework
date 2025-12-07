// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

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

// parseTestFile parses a test file and extracts test function information using
// FILE-BASED MATCHING strategy. This implementation follows the recommendation to
// trust file naming conventions (resource_widget.go -> resource_widget_test.go)
// rather than complex function name parsing.
func parseTestFile(file *ast.File, fset *token.FileSet, filePath string) *TestFileInfo {
	return parseTestFileWithHelpers(file, fset, filePath, nil)
}

// parseTestFileWithHelpers parses a test file with support for custom test helpers.
// Uses file-based matching: If this is resource_widget_test.go, all Test* functions
// are considered tests for the widget resource, regardless of function naming.
func parseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
	// SIMPLIFIED APPROACH: Extract resource name from filename (reliable)
	// e.g., resource_widget_test.go -> widget, data_source_http_test.go -> http
	resourceName, isDataSource := extractResourceNameFromFilePath(filePath)
	if resourceName == "" {
		// Not a standard test file naming pattern
		return nil
	}

	var testFuncs []TestFunctionInfo

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		name := funcDecl.Name.Name

		// Check if this is a test function (any Test* function)
		if !strings.HasPrefix(name, "Test") {
			return true
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

// extractResourceNameFromFilePath extracts resource name from file path using Go conventions.
// Returns (resourceName, isDataSource).
// Examples:
//   - resource_widget_test.go -> ("widget", false)
//   - data_source_http_test.go -> ("http", true)
//   - group_resource_test.go -> ("group", false)
func extractResourceNameFromFilePath(filePath string) (string, bool) {
	baseName := filepath.Base(filePath)
	if !strings.HasSuffix(baseName, "_test.go") {
		return "", false
	}

	nameWithoutTest := strings.TrimSuffix(baseName, "_test.go")

	// Standard HashiCorp patterns
	if strings.HasPrefix(nameWithoutTest, "resource_") {
		resourceName := strings.TrimPrefix(nameWithoutTest, "resource_")
		return resourceName, false
	}
	if strings.HasPrefix(nameWithoutTest, "data_source_") {
		resourceName := strings.TrimPrefix(nameWithoutTest, "data_source_")
		return resourceName, true
	}
	if strings.HasPrefix(nameWithoutTest, "ephemeral_") {
		resourceName := strings.TrimPrefix(nameWithoutTest, "ephemeral_")
		return resourceName, false
	}

	// Reversed naming patterns
	if strings.HasSuffix(nameWithoutTest, "_resource") {
		resourceName := strings.TrimSuffix(nameWithoutTest, "_resource")
		return resourceName, false
	}
	if strings.HasSuffix(nameWithoutTest, "_data_source") {
		resourceName := strings.TrimSuffix(nameWithoutTest, "_data_source")
		return resourceName, true
	}
	if strings.HasSuffix(nameWithoutTest, "_datasource") {
		resourceName := strings.TrimSuffix(nameWithoutTest, "_datasource")
		return resourceName, true
	}

	return "", false
}

// buildRegistry constructs a resource registry by scanning all files in the analysis pass.
// This implements the "File-First" association logic recommended in the spec:
// 1. Scan for Resources (AST-based): Find structs with Schema() methods
// 2. Scan for Test Files (File-based): Find *_test.go files
// 3. Associate (Path-based): resource_widget.go -> resource_widget_test.go
// 4. Verify (Content): Check for TestAcc* functions
func buildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
	reg := NewResourceRegistry()

	// 1. Scan for Resources (Type-based discovery via AST)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		// Skip test files
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Apply exclusion rules
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

		// Parse resources from this file
		resources := parseResources(file, pass.Fset, filename)
		for _, resource := range resources {
			reg.RegisterResource(resource)
		}
	}

	// 2. Scan for Test Files (File-based discovery)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Parse test file
		testFileInfo := parseTestFileWithHelpers(file, pass.Fset, filename, settings.CustomTestHelpers)
		if testFileInfo == nil {
			continue
		}

		// 3. Associate with resource using file-based matching
		// The parseTestFile already extracted the resource name from filename
		// Now verify that this resource exists in our registry
		var resource *ResourceInfo
		if testFileInfo.IsDataSource {
			resource = reg.GetDataSource(testFileInfo.ResourceName)
		} else {
			resource = reg.GetResource(testFileInfo.ResourceName)
		}

		// Only register if we found a matching resource
		// This ensures we don't create false positives for helper test files
		if resource != nil {
			reg.RegisterTestFile(testFileInfo)
		}
	}

	return reg
}

// checkUsesResourceTest checks if a function body contains a call to resource.Test()
func checkUsesResourceTest(body *ast.BlockStmt) bool {
	return checkUsesResourceTestWithHelpers(body, nil)
}

// checkUsesResourceTestWithHelpers checks if a function body contains a call to resource.Test()
// or any of the custom test helper functions specified in customHelpers.
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

// extractTestSteps extracts test step information from a test function body
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

		// Check if it's resource.Test or resource.ParallelTest
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					// Found resource.Test() call, now extract TestCase argument
					if len(call.Args) >= 2 {
						// Second argument should be the TestCase
						steps = extractStepsFromTestCase(call.Args[1], &stepNumber)
					}
					return false // Stop searching after finding resource.Test
				}
			}
		}
		return true
	})

	return steps
}

// extractStepsFromTestCase extracts steps from a resource.TestCase composite literal
func extractStepsFromTestCase(testCaseExpr ast.Expr, stepNumber *int) []TestStepInfo {
	var steps []TestStepInfo

	// Look for CompositeLit representing resource.TestCase{}
	compLit, ok := testCaseExpr.(*ast.CompositeLit)
	if !ok {
		return steps
	}

	// Find the Steps field
	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Steps" {
			continue
		}

		// Steps should be a slice literal
		stepsLit, ok := kv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}

		// Parse each step
		for _, stepExpr := range stepsLit.Elts {
			step := parseTestStep(stepExpr, *stepNumber)
			steps = append(steps, step)
			*stepNumber++
		}
	}

	return steps
}

// parseTestStep parses a single TestStep composite literal
func parseTestStep(stepExpr ast.Expr, stepNum int) TestStepInfo {
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

	// Parse step fields
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

// extractCheckFunctions extracts check function names from a Check field
func extractCheckFunctions(checkExpr ast.Expr) []string {
	var functions []string

	ast.Inspect(checkExpr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Look for function calls in Check
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			functions = append(functions, sel.Sel.Name)
		}

		return true
	})

	return functions
}

// Public API functions for compatibility

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
