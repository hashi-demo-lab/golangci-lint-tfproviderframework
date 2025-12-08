// Package registry implements a registry for managing Terraform provider resources,
// data sources, and their associated test functions discovered during AST analysis.
package registry

import (
	"go/token"
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
// Tests classified as provider tests (TestCategoryProvider) or function tests (TestCategoryFunction)
// are excluded since they don't test resources.
func (r *ResourceRegistry) GetUnmatchedTestFunctions() []*TestFunctionInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var unmatched []*TestFunctionInfo
	for _, fn := range r.testFunctions {
		if fn.MatchType == MatchTypeNone {
			// Skip provider and function tests - these don't test resources
			if fn.Category == TestCategoryProvider || fn.Category == TestCategoryFunction {
				continue
			}
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

// TestCategory classifies what a test is testing (resource, provider config, functions, etc.)
type TestCategory int

const (
	// TestCategoryResource indicates the test is for a specific resource/data source/action.
	TestCategoryResource TestCategory = iota
	// TestCategoryProvider indicates the test is for provider configuration (credentials, endpoints).
	TestCategoryProvider
	// TestCategoryFunction indicates the test is for provider functions (Terraform 1.6+).
	TestCategoryFunction
	// TestCategoryIntegration indicates infrastructure or integration tests.
	TestCategoryIntegration
)

// String returns the string representation of a TestCategory.
func (c TestCategory) String() string {
	switch c {
	case TestCategoryResource:
		return "resource"
	case TestCategoryProvider:
		return "provider"
	case TestCategoryFunction:
		return "function"
	case TestCategoryIntegration:
		return "integration"
	default:
		return "unknown"
	}
}

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
	Kind           ResourceKind
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

// InferredHCLBlock represents a resource/data/action block found in HCL config.
type InferredHCLBlock struct {
	BlockType    string // "resource", "data", or "action"
	ResourceType string // e.g., "aws_instance", "aap_job_launch"
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
	InferredResources []string           // Legacy: just resource type names
	InferredHCLBlocks []InferredHCLBlock // New: typed HCL blocks with block type
	MatchConfidence   float64
	MatchType         MatchType
	HelperUsed        string       // Name of helper function used (e.g., "resource.Test", "AccTestHelper")
	HasCheckDestroy   bool         // HasCheckDestroy tracks presence of CheckDestroy in resource.TestCase
	HasPreCheck       bool         // HasPreCheck tracks presence of PreCheck function
	Category          TestCategory // Category classifies test type (resource, provider, function, integration)
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
