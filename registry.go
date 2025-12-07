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
	definitions    map[string]*ResourceInfo   // Unified map of all resources and data sources
	resources      map[string]*ResourceInfo   // Legacy: filtered view of resources (IsDataSource=false)
	dataSources    map[string]*ResourceInfo   // Legacy: filtered view of data sources (IsDataSource=true)
	testFunctions  []*TestFunctionInfo
	resourceTests  map[string][]*TestFunctionInfo
	fileToResource map[string]string
}

// NewResourceRegistry creates a new empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		definitions:    make(map[string]*ResourceInfo),
		resources:      make(map[string]*ResourceInfo),
		dataSources:    make(map[string]*ResourceInfo),
		testFunctions:  make([]*TestFunctionInfo, 0),
		resourceTests:  make(map[string][]*TestFunctionInfo),
		fileToResource: make(map[string]string),
	}
}

// RegisterResource adds a resource or data source to the registry.
// It stores the resource in both the unified definitions map and the
// appropriate filtered map (resources or dataSources) for backward compatibility.
func (r *ResourceRegistry) RegisterResource(info *ResourceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store in unified definitions map
	r.definitions[info.Name] = info

	// Also store in legacy filtered maps for backward compatibility
	if info.IsDataSource {
		r.dataSources[info.Name] = info
	} else {
		r.resources[info.Name] = info
	}
	r.fileToResource[info.FilePath] = info.Name
}

// GetResource retrieves a resource by name.
func (r *ResourceRegistry) GetResource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resources[name]
}

// GetDataSource retrieves a data source by name.
func (r *ResourceRegistry) GetDataSource(name string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.dataSources[name]
}

// GetResourceByFile retrieves a resource by its file path.
func (r *ResourceRegistry) GetResourceByFile(filePath string) *ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if resourceName, ok := r.fileToResource[filePath]; ok {
		if resource, ok := r.resources[resourceName]; ok {
			return resource
		}
		if dataSource, ok := r.dataSources[resourceName]; ok {
			return dataSource
		}
	}
	return nil
}

// GetUntestedResources returns all resources and data sources that lack test coverage.
func (r *ResourceRegistry) GetUntestedResources() []*ResourceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var untested []*ResourceInfo
	for name, resource := range r.resources {
		if len(r.resourceTests[name]) == 0 {
			untested = append(untested, resource)
		}
	}
	for name, dataSource := range r.dataSources {
		if len(r.resourceTests[name]) == 0 {
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
func (r *ResourceRegistry) LinkTestToResource(resourceName string, fn *TestFunctionInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resourceTests[resourceName] = append(r.resourceTests[resourceName], fn)
}

// GetResourceTests returns all test functions associated with a resource.
func (r *ResourceRegistry) GetResourceTests(resourceName string) []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.resourceTests[resourceName]
}

// GetUnmatchedTestFunctions returns test functions that couldn't be associated with any resource.
func (r *ResourceRegistry) GetUnmatchedTestFunctions() []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var unmatched []*TestFunctionInfo
	for _, fn := range r.testFunctions {
		if len(fn.InferredResources) == 0 {
			unmatched = append(unmatched, fn)
		}
	}
	return unmatched
}

// ResourceInfo holds metadata about a Terraform resource or data source.
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
}

// TestStepInfo represents a single step within a resource.TestCase.
type TestStepInfo struct {
	StepNumber         int
	StepPos            token.Pos
	Config             string
	ConfigHash         string
	HasConfig          bool
	HasCheck           bool
	CheckFunctions     []string
	ImportState        bool
	ImportStateVerify  bool
	ExpectError        bool
	IsUpdateStepFlag   bool
	PreviousConfigHash string
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
