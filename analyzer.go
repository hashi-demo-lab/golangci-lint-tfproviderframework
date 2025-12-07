// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
)

// registryCache holds the cached registry per analysis pass to avoid rebuilding it 5 times
type registryCache struct {
	mu       sync.Mutex
	registry *ResourceRegistry
	once     sync.Once
}

// Global cache map keyed by analysis.Pass pointer to support multiple concurrent analysis runs
var (
	globalCacheMu sync.Mutex
	globalCache   = make(map[*analysis.Pass]*registryCache)
)

// getOrBuildRegistry retrieves a cached registry for the given pass, or builds it if not yet cached.
// This ensures buildRegistry is called only once per analysis pass, even when 5 analyzers run.
func getOrBuildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
	globalCacheMu.Lock()
	cache, exists := globalCache[pass]
	if !exists {
		cache = &registryCache{}
		globalCache[pass] = cache
	}
	globalCacheMu.Unlock()

	// Use sync.Once to ensure buildRegistry is called only once per pass
	cache.once.Do(func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		cache.registry = buildRegistry(pass, settings)
	})

	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.registry
}

// clearRegistryCache clears the cache for a given pass (used for cleanup after analysis)
func clearRegistryCache(pass *analysis.Pass) {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	delete(globalCache, pass)
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

// DriftCheckAnalyzer ensures test functions have CheckDestroy for drift detection.
var DriftCheckAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-drift-check",
	Doc:  "Checks that acceptance tests include CheckDestroy for drift detection.",
	Run:  runDriftCheckAnalyzer,
}

// SweeperAnalyzer ensures packages have sweeper registrations for cleanup.
var SweeperAnalyzer = &analysis.Analyzer{
	Name: "tfprovider-test-sweepers",
	Doc:  "Checks that packages have test sweeper registrations for cleanup.",
	Run:  runSweeperAnalyzer,
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
	registry := getOrBuildRegistry(pass, settings)

	// Report untested resources with enhanced location information
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

		// Enhanced message with suggestions
		msg := fmt.Sprintf("%s '%s' has no acceptance test\n"+
			"  %s: %s:%d\n"+
			"  Expected test file: %s\n"+
			"  Expected test function: %s\n"+
			"  Suggestion: Create %s with function %s",
			resourceType, resource.Name,
			resourceTypeTitle, pos.Filename, pos.Line,
			expectedTestPath, expectedTestFunc,
			filepath.Base(expectedTestPath), expectedTestFunc)

		pass.Reportf(resource.SchemaPos, "%s", msg)
	}

	return nil, nil
}

func runUpdateTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := getOrBuildRegistry(pass, settings)

	// Check for resources with updatable attributes but no update tests
	// Only check regular resources (not data sources)
	for name, resource := range registry.GetAllResources() {
		// Check if resource has updatable attributes using isAttributeUpdatable
		hasUpdatable := false
		var updatableAttrs []string
		for _, attr := range resource.Attributes {
			if isAttributeUpdatable(attr) {
				hasUpdatable = true
				updatableAttrs = append(updatableAttrs, attr.Name)
			}
		}

		if !hasUpdatable {
			// Resource doesn't need update tests
			continue
		}

		// Get all test functions for this resource
		testFunctions := registry.GetResourceTests(name)

		// No tests at all - covered by BasicTestAnalyzer
		if len(testFunctions) == 0 {
			continue
		}

		// Check if any test function has a real update step
		hasUpdateTest := false
		for _, testFunc := range testFunctions {
			for _, step := range testFunc.TestSteps {
				// Use IsRealUpdateStep to properly distinguish real updates
				// from "Apply -> Import" patterns
				if step.IsRealUpdateStep() {
					hasUpdateTest = true
					break
				}
			}
			// Fallback: if we have multiple config steps (excluding imports), consider it an update test
			if !hasUpdateTest && len(testFunc.TestSteps) >= 2 {
				configSteps := 0
				for _, step := range testFunc.TestSteps {
					if step.HasConfig && !step.ImportState {
						configSteps++
					}
				}
				if configSteps >= 2 {
					hasUpdateTest = true
				}
			}
			if hasUpdateTest {
				break
			}
		}

		if !hasUpdateTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' has updatable attributes but no update test coverage\n"+
				"  Resource: %s:%d\n"+
				"  Updatable attributes: %s\n"+
				"  Suggestion: Add a test step that modifies one of these attributes",
				name, pos.Filename, pos.Line,
				strings.Join(updatableAttrs, ", "))
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

// isAttributeUpdatable determines if an attribute needs an update test.
// It returns false for:
//   - Non-optional attributes (Computed-only attributes don't need update tests)
//   - Attributes with RequiresReplace modifiers
//
// It defaults to true if unsure, to avoid false negatives.
func isAttributeUpdatable(attr AttributeInfo) bool {
	// Computed-only attributes don't need update tests
	if !attr.Optional {
		return false
	}

	// Attributes with RequiresReplace don't need update tests
	// (the resource is recreated on change, not updated)
	if !attr.IsUpdatable {
		return false
	}

	// Default: assume it IS updatable if we aren't sure
	return true
}

// IsAttributeUpdatable is the public API for checking if an attribute needs update tests.
func IsAttributeUpdatable(attr AttributeInfo) bool {
	return isAttributeUpdatable(attr)
}

func runImportTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := getOrBuildRegistry(pass, settings)

	// Check for resources with ImportState but no import tests
	// Only check regular resources (not data sources)
	for name, resource := range registry.GetAllResources() {
		// Only check resources that implement ImportState
		if !resource.HasImportState {
			continue
		}

		// Get all test functions for this resource
		testFunctions := registry.GetResourceTests(name)
		if len(testFunctions) == 0 {
			// No tests at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if ANY test function has an import step
		hasImportTest := false
		for _, testFunc := range testFunctions {
			if testFunc.HasImportStep {
				hasImportTest = true
				break
			}
		}

		if !hasImportTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' implements ImportState but has no import test coverage\n"+
				"  Resource: %s:%d\n"+
				"  Suggestion: Add a test step with ImportState: true, ImportStateVerify: true",
				name, pos.Filename, pos.Line)
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

func runErrorTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := getOrBuildRegistry(pass, settings)

	// Check for resources with validation rules but no error tests
	for name, resource := range registry.GetAllResources() {
		// Check if resource has validation rules
		hasValidation := false
		var validatedAttrs []string
		for _, attr := range resource.Attributes {
			if attr.NeedsValidationTest() {
				hasValidation = true
				validatedAttrs = append(validatedAttrs, attr.Name)
			}
		}

		if !hasValidation {
			// Resource doesn't need error tests
			continue
		}

		// Get all test functions for this resource
		testFunctions := registry.GetResourceTests(name)
		if len(testFunctions) == 0 {
			// No tests at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if ANY test function has an error case
		hasErrorTest := false
		for _, testFunc := range testFunctions {
			if testFunc.HasErrorCase {
				hasErrorTest = true
				break
			}
		}

		if !hasErrorTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' has validation rules but no error case tests\n"+
				"  Resource: %s:%d\n"+
				"  Validated attributes: %s\n"+
				"  Suggestion: Add a test step with ExpectError to verify validation",
				name, pos.Filename, pos.Line,
				strings.Join(validatedAttrs, ", "))
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

func runStateCheckAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := getOrBuildRegistry(pass, settings)

	// Check for test steps without Check fields
	// Iterate through all test functions
	for _, testFunc := range registry.GetAllTestFunctions() {
		for _, step := range testFunc.TestSteps {
			// Skip import and error test steps - they don't require Check
			if step.ImportState || step.ExpectError {
				continue
			}

			// Regular test steps with Config should have Check fields or Plan checks
			if !step.HasCheck && !step.HasPlanCheck && step.HasConfig {
				// Try to find the resource being tested from InferredResources
				resourceContext := ""
				if len(testFunc.InferredResources) > 0 {
					resourceContext = fmt.Sprintf(" (testing %s)", testFunc.InferredResources[0])
				}

				// Use step position if available, otherwise use function position
				pos := step.StepPos
				if pos == 0 {
					pos = testFunc.FunctionPos
				}

				msg := fmt.Sprintf("test step in %s%s has no state validation checks or plan checks\n"+
					"  Suggestion: Add Check: resource.ComposeTestCheckFunc(...) or ConfigPlanChecks to verify state",
					testFunc.Name, resourceContext)

				// Report at the step position
				pass.Reportf(pos, "%s", msg)
			}
		}
	}

	return nil, nil
}

func runDriftCheckAnalyzer(pass *analysis.Pass) (interface{}, error) {
	settings := DefaultSettings()
	registry := getOrBuildRegistry(pass, settings)

	// Check test functions for missing CheckDestroy
	for _, testFunc := range registry.GetAllTestFunctions() {
		if !testFunc.HasCheckDestroy {
			pos := testFunc.FunctionPos
			msg := fmt.Sprintf("test function %s missing CheckDestroy for drift detection\n"+
				"  Suggestion: Add CheckDestroy: testAccCheckDestroy to resource.TestCase",
				testFunc.Name)
			pass.Reportf(pos, "%s", msg)
		}
	}

	return nil, nil
}

func runSweeperAnalyzer(pass *analysis.Pass) (interface{}, error) {
	// Check if any file in the package has sweeper registrations
	hasSweepers := false
	for _, file := range pass.Files {
		if CheckHasSweepers(file) {
			hasSweepers = true
			break
		}
	}

	if !hasSweepers {
		// Report at package level (first file position)
		if len(pass.Files) > 0 {
			pass.Reportf(pass.Files[0].Pos(), "package has no test sweeper registrations\n"+
				"  Suggestion: Add resource.AddTestSweepers() calls for cleanup")
		}
	}

	return nil, nil
}
