// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
)

// MatchType indicates how a test function was associated with a resource.
type MatchType int

const (
	// MatchTypeNone indicates no match was found.
	MatchTypeNone MatchType = iota
	// MatchTypeInferred indicates the match was found by parsing the HCL config (Highest Priority).
	// This is the most reliable match type as it extracts resource names directly from test configurations.
	MatchTypeInferred
	// MatchTypeFunctionName indicates the match was extracted from the test function name.
	MatchTypeFunctionName
	// MatchTypeFileProximity indicates the match was based on file naming convention.
	MatchTypeFileProximity
	// MatchTypeFuzzy indicates the match was determined via fuzzy/Levenshtein matching.
	MatchTypeFuzzy
)

// String returns the string representation of a MatchType.
func (m MatchType) String() string {
	switch m {
	case MatchTypeInferred:
		return "inferred_from_config"
	case MatchTypeFunctionName:
		return "function_name"
	case MatchTypeFileProximity:
		return "file_proximity"
	case MatchTypeFuzzy:
		return "fuzzy"
	default:
		return "none"
	}
}

// ResourceRegistry maintains thread-safe mappings of resources, data sources,
// and their associated test functions discovered during AST analysis.
type ResourceRegistry struct {
	mu             sync.RWMutex
	definitions    map[string]*ResourceInfo // Unified map of all resources and data sources
	testFunctions  []*TestFunctionInfo
	resourceTests  map[string][]*TestFunctionInfo
	fileToResource map[string]string
}

// NewResourceRegistry creates a new empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		definitions:    make(map[string]*ResourceInfo),
		testFunctions:  make([]*TestFunctionInfo, 0),
		resourceTests:  make(map[string][]*TestFunctionInfo),
		fileToResource: make(map[string]string),
	}
}

// registryKey creates a unique key for a resource in the registry.
// This allows resources, data sources, and actions with the same base name to coexist.
func registryKey(kind ResourceKind, name string) string {
	return kind.String() + ":" + name
}

// RegisterResource adds a resource, data source, or action to the registry.
func (r *ResourceRegistry) RegisterResource(info *ResourceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Infer Kind from deprecated IsDataSource/IsAction fields for backward compatibility
	if info.IsDataSource && info.Kind == KindResource {
		info.Kind = KindDataSource
	}
	if info.IsAction && info.Kind == KindResource {
		info.Kind = KindAction
	}

	key := registryKey(info.Kind, info.Name)
	r.definitions[key] = info
	r.fileToResource[info.FilePath] = key
}

// GetResourceByFile retrieves a resource by its file path.
func (r *ResourceRegistry) GetResourceByFile(filePath string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if resourceName, ok := r.fileToResource[filePath]; ok {
		return r.definitions[resourceName]
	}
	return nil
}

// GetUntestedResources returns all resources and data sources that lack test coverage.
func (r *ResourceRegistry) GetUntestedResources() []*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var untested []*ResourceInfo
	for name, info := range r.definitions {
		if len(r.resourceTests[name]) == 0 {
			untested = append(untested, info)
		}
	}
	return untested
}

// GetAllDefinitions returns a copy of all resources and data sources in a single map (thread-safe).
// This is the preferred method for iterating over all resource types, as it avoids
// the need to merge separate resources and dataSources maps.
func (r *ResourceRegistry) GetAllDefinitions() map[string]*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]*ResourceInfo, len(r.definitions))
	for k, v := range r.definitions {
		result[k] = v
	}
	return result
}

// GetResourceOrDataSource retrieves a resource or data source by name using the unified definitions map.
// It accepts either a simple name ("widget") or a compound key ("resource:widget").
// For simple names, it returns the first matching definition found.
func (r *ResourceRegistry) GetResourceOrDataSource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If it's already a compound key, use it directly
	if strings.Contains(name, ":") {
		return r.definitions[name]
	}

	// For simple names, try each kind in order
	for _, kind := range []ResourceKind{KindResource, KindDataSource, KindAction} {
		key := registryKey(kind, name)
		if info := r.definitions[key]; info != nil {
			return info
		}
	}
	return nil
}

// RegisterTestFunction adds a test function to the global index.
func (r *ResourceRegistry) RegisterTestFunction(fn *TestFunctionInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.testFunctions = append(r.testFunctions, fn)
}

// GetAllTestFunctions returns a copy of all test functions (thread-safe).
func (r *ResourceRegistry) GetAllTestFunctions() []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*TestFunctionInfo, len(r.testFunctions))
	copy(result, r.testFunctions)
	return result
}

// LinkTestToResource associates a test function with a resource.
// It accepts either a simple name ("widget") or a compound key ("resource:widget").
// For simple names, it finds the first matching definition and uses its compound key.
func (r *ResourceRegistry) LinkTestToResource(resourceName string, fn *TestFunctionInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := resourceName
	// If it's not already a compound key, try to find the right one
	if !strings.Contains(resourceName, ":") {
		// Try each kind in order of priority
		for _, kind := range []ResourceKind{KindResource, KindDataSource, KindAction} {
			candidateKey := registryKey(kind, resourceName)
			if _, exists := r.definitions[candidateKey]; exists {
				key = candidateKey
				break
			}
		}
	}
	r.resourceTests[key] = append(r.resourceTests[key], fn)
}

// GetResourceTests returns all test functions associated with a resource.
// It accepts either a simple name ("widget") or a compound key ("resource:widget").
// For simple names, it aggregates tests from all matching kinds (resource, datasource, action).
func (r *ResourceRegistry) GetResourceTests(resourceName string) []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If it's already a compound key (contains ":"), use it directly
	if strings.Contains(resourceName, ":") {
		return r.resourceTests[resourceName]
	}

	// For simple names, aggregate tests from all kinds
	var allTests []*TestFunctionInfo
	for _, kind := range []ResourceKind{KindResource, KindDataSource, KindAction} {
		key := registryKey(kind, resourceName)
		if tests := r.resourceTests[key]; len(tests) > 0 {
			allTests = append(allTests, tests...)
		}
	}
	return allTests
}

// GetUnmatchedTestFunctions returns test functions that couldn't be associated with any resource.
// A test is considered unmatched if it has MatchTypeNone (no matching strategy succeeded).
func (r *ResourceRegistry) GetUnmatchedTestFunctions() []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var unmatched []*TestFunctionInfo
	for _, fn := range r.testFunctions {
		if fn.MatchType == MatchTypeNone {
			unmatched = append(unmatched, fn)
		}
	}
	return unmatched
}

// ResourceKind represents the type of Terraform provider component.
type ResourceKind int

const (
	// KindResource represents a standard Terraform resource.
	KindResource ResourceKind = iota
	// KindDataSource represents a Terraform data source.
	KindDataSource
	// KindAction represents a Terraform action (plugin framework).
	KindAction
)

// String returns the string representation of a ResourceKind.
func (k ResourceKind) String() string {
	switch k {
	case KindResource:
		return "resource"
	case KindDataSource:
		return "data source"
	case KindAction:
		return "action"
	default:
		return "unknown"
	}
}

// ResourceInfo holds metadata about a Terraform resource, data source, or action.
type ResourceInfo struct {
	Name           string
	Kind           ResourceKind // New: replaces IsDataSource
	IsDataSource   bool         // Deprecated: use Kind == KindDataSource
	IsAction       bool         // Deprecated: use Kind == KindAction
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
	PackageName   string
	ResourceName  string
	IsDataSource  bool
	TestFunctions []TestFunctionInfo
}

// TestFunctionInfo represents a single TestAcc function and its test steps.
type TestFunctionInfo struct {
	Name              string
	FilePath          string
	FunctionPos       token.Pos
	UsesResourceTest  bool
	TestSteps         []TestStepInfo
	HasErrorCase      bool
	HasImportStep     bool
	InferredResources []string
	MatchConfidence   float64
	MatchType         MatchType
	HelperUsed        string // Name of helper function used (e.g., "resource.Test", "AccTestHelper")
	HasCheckDestroy   bool   // HasCheckDestroy tracks presence of CheckDestroy in resource.TestCase
	HasPreCheck       bool   // HasPreCheck tracks presence of PreCheck function
}

// TestStepInfo represents a single step within a resource.TestCase.
type TestStepInfo struct {
	StepNumber           int
	StepPos              token.Pos
	Config               string
	ConfigHash           string
	HasConfig            bool
	HasCheck             bool
	CheckFunctions       []string
	ImportState          bool
	ImportStateVerify    bool
	ExpectError          bool
	IsUpdateStepFlag     bool
	PreviousConfigHash   string
	HasPlanCheck         bool // HasPlanCheck tracks presence of ConfigPlanChecks
	HasConfigStateChecks bool // HasConfigStateChecks tracks presence of ConfigStateChecks (newer pattern)
	ExpectNonEmptyPlan   bool // ExpectNonEmptyPlan tracks if step expects non-empty plan
	RefreshState         bool // RefreshState tracks if step uses refresh mode
}

// IsUpdateStep returns true if this is not the first step and has a config.
func (t *TestStepInfo) IsUpdateStep() bool {
	return t.StepNumber > 0 && t.HasConfig
}

// IsRealUpdateStep returns true if this step is a genuine update step,
// excluding import steps and steps without configs.
// This is used to distinguish real update tests from "Apply -> Import" patterns.
func (t *TestStepInfo) IsRealUpdateStep() bool {
	return t.StepNumber > 0 && t.HasConfig && !t.ImportState
}

// DetermineIfUpdateStep checks if a step is an update step.
func (t *TestStepInfo) DetermineIfUpdateStep(prevStep *TestStepInfo) bool {
	if t.StepNumber == 0 {
		return false
	}
	if t.ImportState {
		return false
	}
	if !t.HasConfig {
		return false
	}
	if prevStep == nil || !prevStep.HasConfig {
		return false
	}
	if t.ConfigHash == prevStep.ConfigHash {
		return false
	}
	return true
}

// IsValidImportStep returns true if this step properly tests ImportState.
func (t *TestStepInfo) IsValidImportStep() bool {
	return t.ImportState && t.ImportStateVerify
}

// HasStateOrPlanCheck returns true if this test function has at least one step
// with state validation (Check field, ConfigStateChecks) or plan validation (ConfigPlanChecks).
func (t *TestFunctionInfo) HasStateOrPlanCheck() bool {
	for _, step := range t.TestSteps {
		if step.HasCheck || step.HasPlanCheck || step.HasConfigStateChecks {
			return true
		}
	}
	return false
}

// ResourceCoverage represents aggregated test coverage for a single resource or data source.
type ResourceCoverage struct {
	Resource         *ResourceInfo
	Tests            []*TestFunctionInfo
	HasBasicTest     bool // At least one test exists
	HasStateCheck    bool // At least one test has Check field
	HasPlanCheck     bool // At least one test has ConfigPlanChecks
	HasCheckDestroy  bool // At least one test has CheckDestroy
	HasImportTest    bool // At least one test has ImportState step
	HasUpdateTest    bool // At least one test has update steps (multiple configs)
	HasErrorTest     bool // At least one test has ExpectError
	TestCount        int
	StepCount        int
	UpdateStepCount  int
	ImportStepCount  int
}

// GetResourceCoverage computes aggregated test coverage for a resource.
func (r *ResourceRegistry) GetResourceCoverage(resourceName string) *ResourceCoverage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resource := r.definitions[resourceName]
	if resource == nil {
		return nil
	}

	tests := r.resourceTests[resourceName]
	coverage := &ResourceCoverage{
		Resource:  resource,
		Tests:     tests,
		TestCount: len(tests),
	}

	for _, test := range tests {
		coverage.HasBasicTest = true

		if test.HasCheckDestroy {
			coverage.HasCheckDestroy = true
		}
		if test.HasImportStep {
			coverage.HasImportTest = true
		}
		if test.HasErrorCase {
			coverage.HasErrorTest = true
		}

		for _, step := range test.TestSteps {
			coverage.StepCount++

			if step.HasCheck || step.HasConfigStateChecks {
				coverage.HasStateCheck = true
			}
			if step.HasPlanCheck {
				coverage.HasPlanCheck = true
			}
			if step.ImportState {
				coverage.ImportStepCount++
			}
			if step.IsRealUpdateStep() {
				coverage.HasUpdateTest = true
				coverage.UpdateStepCount++
			}
		}
	}

	return coverage
}

// GetAllResourceCoverage returns coverage information for all resources and data sources.
func (r *ResourceRegistry) GetAllResourceCoverage() []*ResourceCoverage {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var coverages []*ResourceCoverage
	for name, resource := range r.definitions {
		tests := r.resourceTests[name]
		coverage := &ResourceCoverage{
			Resource:  resource,
			Tests:     tests,
			TestCount: len(tests),
		}

		for _, test := range tests {
			coverage.HasBasicTest = true

			if test.HasCheckDestroy {
				coverage.HasCheckDestroy = true
			}
			if test.HasImportStep {
				coverage.HasImportTest = true
			}
			if test.HasErrorCase {
				coverage.HasErrorTest = true
			}

			for _, step := range test.TestSteps {
				coverage.StepCount++

				if step.HasCheck || step.HasConfigStateChecks {
					coverage.HasStateCheck = true
				}
				if step.HasPlanCheck {
					coverage.HasPlanCheck = true
				}
				if step.ImportState {
					coverage.ImportStepCount++
				}
				if step.IsRealUpdateStep() {
					coverage.HasUpdateTest = true
					coverage.UpdateStepCount++
				}
			}
		}

		coverages = append(coverages, coverage)
	}

	return coverages
}

// GetResourcesMissingStateChecks returns resources that have tests but no state/plan checks.
func (r *ResourceRegistry) GetResourcesMissingStateChecks() []*ResourceCoverage {
	coverages := r.GetAllResourceCoverage()
	var missing []*ResourceCoverage
	for _, cov := range coverages {
		// Only report resources that have tests but lack validation
		if cov.HasBasicTest && !cov.HasStateCheck && !cov.HasPlanCheck {
			missing = append(missing, cov)
		}
	}
	return missing
}

// GetResourcesMissingCheckDestroy returns resources that have tests but no CheckDestroy.
func (r *ResourceRegistry) GetResourcesMissingCheckDestroy() []*ResourceCoverage {
	coverages := r.GetAllResourceCoverage()
	var missing []*ResourceCoverage
	for _, cov := range coverages {
		// Only report resources that have tests but lack CheckDestroy
		// Data sources typically don't need CheckDestroy
		if cov.HasBasicTest && !cov.HasCheckDestroy && !cov.Resource.IsDataSource {
			missing = append(missing, cov)
		}
	}
	return missing
}

// TestFileSearchResult represents a test file that was searched for a resource.
type TestFileSearchResult struct {
	FilePath string
	Found    bool
}

// TestFunctionMatchInfo represents a test function found during analysis.
type TestFunctionMatchInfo struct {
	Name        string
	Line        int
	MatchStatus string
	MatchReason string
}

// VerboseDiagnosticInfo holds detailed diagnostic information.
type VerboseDiagnosticInfo struct {
	ResourceName       string
	ResourceType       string
	ResourceFile       string
	ResourceLine       int
	TestFilesSearched  []TestFileSearchResult
	TestFunctionsFound []TestFunctionMatchInfo
	ExpectedPatterns   []string
	FoundPattern       string
	SuggestedFixes     []string
}

// HasMatchingTestFile checks if a resource has matching test functions.
func HasMatchingTestFile(resourceName string, isDataSource bool, registry *ResourceRegistry) bool {
	testFunctions := registry.GetResourceTests(resourceName)
	return len(testFunctions) > 0
}

// BuildExpectedTestPath constructs the expected test file path for a given resource.
func BuildExpectedTestPath(resource *ResourceInfo) string {
	filePath := resource.FilePath
	if strings.HasSuffix(filePath, ".go") {
		return strings.TrimSuffix(filePath, ".go") + "_test.go"
	}
	return filePath + "_test.go"
}

// BuildExpectedTestFunc constructs the expected test function name for a given resource.
func BuildExpectedTestFunc(resource *ResourceInfo) string {
	titleName := toTitleCase(resource.Name)
	if resource.IsDataSource {
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

	// Try to match using the same logic as matchResourceByName
	matched, found := MatchResourceByName(funcName, resourceNames)
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
func BuildVerboseDiagnosticInfo(resource *ResourceInfo, registry *ResourceRegistry) VerboseDiagnosticInfo {
	resourceType := "resource"
	if resource.IsDataSource {
		resourceType = "data source"
	}
	info := VerboseDiagnosticInfo{
		ResourceName: resource.Name,
		ResourceType: resourceType,
		ResourceFile: resource.FilePath,
		ResourceLine: 0,
	}
	expectedTestPath := BuildExpectedTestPath(resource)
	testFunctions := registry.GetResourceTests(resource.Name)

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
			info.TestFilesSearched = append(info.TestFilesSearched, TestFileSearchResult{FilePath: path, Found: true})
		}
		// Add test function info
		for _, testFunc := range testFunctions {
			status, reason := ClassifyTestFunctionMatch(testFunc.Name, resource.Name)
			info.TestFunctionsFound = append(info.TestFunctionsFound, TestFunctionMatchInfo{
				Name: testFunc.Name, Line: 0, MatchStatus: status, MatchReason: reason,
			})
		}
	} else {
		info.TestFilesSearched = []TestFileSearchResult{{FilePath: expectedTestPath, Found: false}}
	}

	titleName := toTitleCase(resource.Name)
	if resource.IsDataSource {
		info.ExpectedPatterns = []string{"TestAccDataSource" + titleName + "*", "TestDataSource" + titleName + "*"}
	} else {
		info.ExpectedPatterns = []string{"TestAcc" + titleName + "*", "TestAccResource" + titleName + "*", "TestResource" + titleName + "*"}
	}
	info.SuggestedFixes = buildSuggestedFixes(resource, testFunctions)
	return info
}

func buildSuggestedFixes(resource *ResourceInfo, testFunctions []*TestFunctionInfo) []string {
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
func FormatVerboseDiagnostic(info VerboseDiagnosticInfo) string {
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
