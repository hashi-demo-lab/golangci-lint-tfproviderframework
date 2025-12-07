// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"

	"golang.org/x/tools/go/analysis"
)

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
