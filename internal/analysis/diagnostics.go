// Package analysis implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package analysis

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/example/tfprovidertest/internal/matching"
	"github.com/example/tfprovidertest/internal/registry"
)

// toTitleCase converts a string to title case.
// E.g., "example_widget" -> "ExampleWidget"
func toTitleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if len(part) > 0 {
			runes := []rune(part)
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, "")
}

// HasMatchingTestFile checks if a resource has matching test functions.
func HasMatchingTestFile(resourceName string, isDataSource bool, reg *registry.ResourceRegistry) bool {
	testFunctions := reg.GetResourceTests(resourceName)
	return len(testFunctions) > 0
}

// BuildExpectedTestPath constructs the expected test file path for a given resource.
func BuildExpectedTestPath(resource *registry.ResourceInfo) string {
	filePath := resource.FilePath
	if strings.HasSuffix(filePath, ".go") {
		return strings.TrimSuffix(filePath, ".go") + "_test.go"
	}
	return filePath + "_test.go"
}

// BuildExpectedTestFunc constructs the expected test function name for a given resource.
func BuildExpectedTestFunc(resource *registry.ResourceInfo) string {
	titleName := toTitleCase(resource.Name)
	if resource.Kind == registry.KindDataSource {
		return fmt.Sprintf("TestAccDataSource%s_basic", titleName)
	}
	return fmt.Sprintf("TestAcc%s_basic", titleName)
}

// ClassifyTestFunctionMatch determines if a test function matches a resource.
// This function uses the same pattern matching logic as the Linker to ensure consistency.
func ClassifyTestFunctionMatch(funcName string, resourceName string) (status string, reason string) {
	// Use the same matching logic as the Linker
	// Create a resource set with just this resource
	resourceNames := map[string]bool{resourceName: true}

	// Try to match using the same logic as MatchResourceByName
	matched, found := matching.MatchResourceByName(funcName, resourceNames)
	if found && matched == resourceName {
		return "matched", ""
	}

	// If we get here, the function doesn't match
	// Provide helpful diagnostic reasons
	if strings.HasPrefix(funcName, "TestAcc") {
		return "not_matched", "does not match resource '" + resourceName + "'"
	}
	if strings.HasPrefix(funcName, "Test") && !strings.HasPrefix(funcName, "TestAcc") {
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
func BuildVerboseDiagnosticInfo(resource *registry.ResourceInfo, reg *registry.ResourceRegistry) registry.VerboseDiagnosticInfo {
	resourceType := "resource"
	if resource.Kind == registry.KindDataSource {
		resourceType = "data source"
	}
	info := registry.VerboseDiagnosticInfo{
		ResourceName: resource.Name,
		ResourceType: resourceType,
		ResourceFile: resource.FilePath,
		ResourceLine: 0,
	}
	expectedTestPath := BuildExpectedTestPath(resource)
	testFunctions := reg.GetResourceTests(resource.Name)

	// Collect unique test file paths
	testFilePaths := make(map[string]bool)
	for _, testFunc := range testFunctions {
		if testFunc.FilePath != "" {
			testFilePaths[testFunc.FilePath] = true
		}
	}

	if len(testFunctions) > 0 {
		// Mark test files as found
		for path := range testFilePaths {
			info.TestFilesSearched = append(info.TestFilesSearched, registry.TestFileSearchResult{FilePath: path, Found: true})
		}
		// Add test function info
		for _, testFunc := range testFunctions {
			status, reason := ClassifyTestFunctionMatch(testFunc.Name, resource.Name)
			info.TestFunctionsFound = append(info.TestFunctionsFound, registry.TestFunctionMatchInfo{
				Name: testFunc.Name, Line: 0, MatchStatus: status, MatchReason: reason,
			})
		}
	} else {
		info.TestFilesSearched = []registry.TestFileSearchResult{{FilePath: expectedTestPath, Found: false}}
	}

	titleName := toTitleCase(resource.Name)
	if resource.Kind == registry.KindDataSource {
		info.ExpectedPatterns = []string{"TestAccDataSource" + titleName + "*", "TestDataSource" + titleName + "*"}
	} else {
		info.ExpectedPatterns = []string{"TestAcc" + titleName + "*", "TestAccResource" + titleName + "*", "TestResource" + titleName + "*"}
	}
	info.SuggestedFixes = buildSuggestedFixes(resource, testFunctions)
	return info
}

// buildSuggestedFixes generates suggested fixes for untested resources.
func buildSuggestedFixes(resource *registry.ResourceInfo, testFunctions []*registry.TestFunctionInfo) []string {
	var fixes []string
	expectedFunc := BuildExpectedTestFunc(resource)
	if len(testFunctions) == 0 {
		expectedPath := BuildExpectedTestPath(resource)
		fixes = append(fixes, fmt.Sprintf("Create test file %s with function %s", filepath.Base(expectedPath), expectedFunc))
	} else {
		// Get first test file path
		testFilePath := ""
		if len(testFunctions) > 0 && testFunctions[0].FilePath != "" {
			testFilePath = testFunctions[0].FilePath
		}
		if testFilePath != "" {
			fixes = append(fixes, fmt.Sprintf("Option 1: Rename tests to follow convention (%s)", expectedFunc))
			fixes = append(fixes, "Option 2: Configure custom test patterns in .golangci.yml:\n      test-name-patterns:\n        - \"Test"+toTitleCase(resource.Name)+"\"")
		} else {
			fixes = append(fixes, fmt.Sprintf("Add acceptance test function %s", expectedFunc))
		}
	}
	return fixes
}

// FormatVerboseDiagnostic formats a VerboseDiagnosticInfo into a human-readable string.
func FormatVerboseDiagnostic(info registry.VerboseDiagnosticInfo) string {
	var sb strings.Builder
	sb.WriteString("\n  Resource Location:\n")
	sb.WriteString(fmt.Sprintf("    %s: %s:%d\n", info.ResourceType, info.ResourceFile, info.ResourceLine))
	sb.WriteString("\n  Test Files Searched:\n")
	for _, tf := range info.TestFilesSearched {
		status := "not found"
		if tf.Found {
			status = "found"
		}
		sb.WriteString(fmt.Sprintf("    - %s (%s)\n", filepath.Base(tf.FilePath), status))
	}
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
	if len(info.ExpectedPatterns) > 0 {
		sb.WriteString("\n  Why Not Matched:\n")
		sb.WriteString(fmt.Sprintf("    Expected pattern: %s\n", strings.Join(info.ExpectedPatterns, " or ")))
		if info.FoundPattern != "" {
			sb.WriteString(fmt.Sprintf("    Found pattern: %s\n", info.FoundPattern))
		}
	}
	if len(info.SuggestedFixes) > 0 {
		sb.WriteString("\n  Suggested Fix:\n")
		for _, fix := range info.SuggestedFixes {
			sb.WriteString(fmt.Sprintf("    %s\n", fix))
		}
	}
	return sb.String()
}
