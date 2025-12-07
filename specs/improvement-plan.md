# tfprovidertest Improvement Plan

**Date:** 2025-12-07 09:53 UTC
**Branch:** 001-tfprovider-test-linter
**Status:** Implementation Phase
**Document Version:** 2.0

---

## Executive Summary

This document outlines a comprehensive improvement plan for the `tfprovidertest` linter based on feedback analysis, research of established tools (tfproviderlint), and HashiCorp's official testing best practices. The primary goal is to transition from a fragile **File-First** matching strategy to a robust **Function-First** indexing approach.

---

## Research Sources

- [tfproviderlint - GitHub](https://github.com/bflad/tfproviderlint) - Terraform Provider Lint Tool by Brian Flad
- [AT002 Analyzer - pkg.go.dev](https://pkg.go.dev/github.com/bflad/tfproviderlint/passes/AT002) - Example of function-based test analysis
- [HashiCorp Naming Conventions](https://developer.hashicorp.com/terraform/plugin/best-practices/naming) - Official naming best practices
- [AWS Provider Acceptance Tests Guide](https://hashicorp.github.io/terraform-provider-aws/running-and-writing-acceptance-tests/) - Real-world test organization patterns
- [Terraform Acceptance Testing](https://www.terraform.io/plugin/sdkv2/testing/acceptance-tests) - Official testing documentation

---

## Problem Statement

### Current Architecture Limitations

The current implementation relies on **File-Based Matching** which fails in these scenarios:

| Scenario | Current Behavior | Impact |
|----------|-----------------|--------|
| Composite Files (`resources.go` with ResourceA + ResourceB) | Only last resource matched | False "untested" reports |
| Grouped Tests (`all_tests_test.go`) | All tests ignored | Entire test suites missed |
| Non-Standard Naming (`compute_instance_test.go`) | May not match resource | Provider-specific failures |
| Cross-File Tests (tests in different directory) | Association fails | Large provider incompatibility |

---

## Implementation Checklist

### Phase 1: Foundation Refactoring

#### 1.1 Decouple TestFileInfo from ResourceName

**Goal:** Remove the 1:1 constraint between test files and resources

**Current Code Location:** `registry.go:177-182`

```go
// CURRENT (File-First) - registry.go:177-182
type TestFileInfo struct {
    FilePath      string
    ResourceName  string        // Forces 1:1 mapping - REMOVE THIS
    IsDataSource  bool
    TestFunctions []TestFunctionInfo
}
```

- [ ] **1.1.1** Update `TestFileInfo` struct in `registry.go:177-182`

  **Implementation:**
  ```go
  // PROPOSED (Function-First) - registry.go
  type TestFileInfo struct {
      FilePath      string
      PackageName   string              // NEW: Package context for imports
      TestFunctions []TestFunctionInfo
      // ResourceName removed - association now happens at function level
  }
  ```

  **Files to modify:** `registry.go`
  **Lines affected:** 177-182
  **Test file:** `registry_test.go` (update TestFileInfo construction tests)

- [ ] **1.1.2** Update `TestFunctionInfo` struct in `registry.go:184-192`

  **Current Code:**
  ```go
  // CURRENT - registry.go:184-192
  type TestFunctionInfo struct {
      Name             string
      FunctionPos      token.Pos
      UsesResourceTest bool
      TestSteps        []TestStepInfo
      HasErrorCase     bool
      HasImportStep    bool
  }
  ```

  **Implementation:**
  ```go
  // PROPOSED - registry.go
  type TestFunctionInfo struct {
      Name              string
      FilePath          string              // NEW: Source file path
      FunctionPos       token.Pos
      UsesResourceTest  bool
      TestSteps         []TestStepInfo
      HasErrorCase      bool
      HasImportStep     bool
      InferredResources []string            // NEW: Matched resource names
      MatchConfidence   float64             // NEW: 0.0-1.0 confidence score
      MatchType         MatchType           // NEW: How the match was determined
  }

  // MatchType indicates how a test function was associated with a resource
  type MatchType int

  const (
      MatchTypeNone MatchType = iota
      MatchTypeFunctionName    // Extracted from TestAccWidget_basic -> widget
      MatchTypeFileProximity   // Same base name: resource_widget.go <-> resource_widget_test.go
      MatchTypeFuzzy           // Levenshtein distance match
  )

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
  ```

  **Files to modify:** `registry.go`
  **Lines affected:** 184-192
  **Test coverage required:**
  - Test `MatchType.String()` for all values
  - Test `TestFunctionInfo` with new fields populated

- [ ] **1.1.3** Update `ResourceRegistry` struct in `registry.go:15-22`

  **Current Code:**
  ```go
  // CURRENT - registry.go:15-22
  type ResourceRegistry struct {
      mu          sync.RWMutex
      resources   map[string]*ResourceInfo
      dataSources map[string]*ResourceInfo
      testFiles   map[string]*TestFileInfo
      fileToResource map[string]string
  }
  ```

  **Implementation:**
  ```go
  // PROPOSED - registry.go
  type ResourceRegistry struct {
      mu             sync.RWMutex
      resources      map[string]*ResourceInfo
      dataSources    map[string]*ResourceInfo
      testFiles      map[string]*TestFileInfo      // Key: file path (not resource name)
      testFunctions  []*TestFunctionInfo           // NEW: Global function index
      resourceTests  map[string][]*TestFunctionInfo // NEW: resource name -> functions
      fileToResource map[string]string
  }
  ```

  **Files to modify:** `registry.go`
  **Lines affected:** 15-22, 25-32 (NewResourceRegistry)

- [ ] **1.1.4** Update registry methods in `registry.go`

  **New/Modified Methods:**

  ```go
  // NewResourceRegistry - registry.go:25-32 (MODIFY)
  func NewResourceRegistry() *ResourceRegistry {
      return &ResourceRegistry{
          resources:      make(map[string]*ResourceInfo),
          dataSources:    make(map[string]*ResourceInfo),
          testFiles:      make(map[string]*TestFileInfo),
          testFunctions:  make([]*TestFunctionInfo, 0),           // NEW
          resourceTests:  make(map[string][]*TestFunctionInfo),   // NEW
          fileToResource: make(map[string]string),
      }
  }

  // RegisterTestFile - registry.go:48-52 (MODIFY)
  // Now uses file path as key instead of resource name
  func (r *ResourceRegistry) RegisterTestFile(info *TestFileInfo) {
      r.mu.Lock()
      defer r.mu.Unlock()
      r.testFiles[info.FilePath] = info  // Changed: key is now FilePath
  }

  // RegisterTestFunction - NEW METHOD
  func (r *ResourceRegistry) RegisterTestFunction(fn *TestFunctionInfo) {
      r.mu.Lock()
      defer r.mu.Unlock()
      r.testFunctions = append(r.testFunctions, fn)
  }

  // LinkTestToResource - NEW METHOD
  // Associates a test function with a resource
  func (r *ResourceRegistry) LinkTestToResource(resourceName string, fn *TestFunctionInfo) {
      r.mu.Lock()
      defer r.mu.Unlock()
      r.resourceTests[resourceName] = append(r.resourceTests[resourceName], fn)
  }

  // GetResourceTests - NEW METHOD
  // Returns all test functions associated with a resource
  func (r *ResourceRegistry) GetResourceTests(resourceName string) []*TestFunctionInfo {
      r.mu.RLock()
      defer r.mu.RUnlock()
      return r.resourceTests[resourceName]
  }

  // GetAllTestFunctions - NEW METHOD
  func (r *ResourceRegistry) GetAllTestFunctions() []*TestFunctionInfo {
      r.mu.RLock()
      defer r.mu.RUnlock()
      result := make([]*TestFunctionInfo, len(r.testFunctions))
      copy(result, r.testFunctions)
      return result
  }

  // GetUnmatchedTestFunctions - NEW METHOD
  // Returns test functions that couldn't be associated with any resource
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
  ```

  **Update GetUntestedResources - registry.go:91-107:**
  ```go
  // GetUntestedResources - MODIFY to use new resourceTests map
  func (r *ResourceRegistry) GetUntestedResources() []*ResourceInfo {
      r.mu.RLock()
      defer r.mu.RUnlock()

      var untested []*ResourceInfo
      for name, resource := range r.resources {
          if tests := r.resourceTests[name]; len(tests) == 0 {
              untested = append(untested, resource)
          }
      }
      for name, dataSource := range r.dataSources {
          if tests := r.resourceTests[name]; len(tests) == 0 {
              untested = append(untested, dataSource)
          }
      }
      return untested
  }
  ```

- [ ] **1.1.5** Write unit tests for registry changes

  **Test file:** `registry_test.go`

  ```go
  func TestRegisterTestFunction(t *testing.T) {
      reg := NewResourceRegistry()

      fn := &TestFunctionInfo{
          Name:     "TestAccWidget_basic",
          FilePath: "/path/to/resource_widget_test.go",
      }

      reg.RegisterTestFunction(fn)

      funcs := reg.GetAllTestFunctions()
      if len(funcs) != 1 {
          t.Errorf("expected 1 function, got %d", len(funcs))
      }
  }

  func TestLinkTestToResource(t *testing.T) {
      reg := NewResourceRegistry()

      // Register a resource first
      reg.RegisterResource(&ResourceInfo{Name: "widget"})

      fn := &TestFunctionInfo{
          Name:              "TestAccWidget_basic",
          InferredResources: []string{"widget"},
          MatchConfidence:   1.0,
          MatchType:         MatchTypeFunctionName,
      }

      reg.RegisterTestFunction(fn)
      reg.LinkTestToResource("widget", fn)

      tests := reg.GetResourceTests("widget")
      if len(tests) != 1 {
          t.Errorf("expected 1 test, got %d", len(tests))
      }
  }

  func TestGetUnmatchedTestFunctions(t *testing.T) {
      reg := NewResourceRegistry()

      matched := &TestFunctionInfo{
          Name:              "TestAccWidget_basic",
          InferredResources: []string{"widget"},
      }
      unmatched := &TestFunctionInfo{
          Name:              "TestAccOrphan_basic",
          InferredResources: []string{}, // No matches
      }

      reg.RegisterTestFunction(matched)
      reg.RegisterTestFunction(unmatched)

      orphans := reg.GetUnmatchedTestFunctions()
      if len(orphans) != 1 {
          t.Errorf("expected 1 orphan, got %d", len(orphans))
      }
      if orphans[0].Name != "TestAccOrphan_basic" {
          t.Errorf("unexpected orphan: %s", orphans[0].Name)
      }
  }

  func TestRegistryConcurrentAccess(t *testing.T) {
      reg := NewResourceRegistry()
      var wg sync.WaitGroup

      // Concurrent writes
      for i := 0; i < 100; i++ {
          wg.Add(1)
          go func(n int) {
              defer wg.Done()
              fn := &TestFunctionInfo{
                  Name: fmt.Sprintf("TestFunc%d", n),
              }
              reg.RegisterTestFunction(fn)
          }(i)
      }

      // Concurrent reads while writing
      for i := 0; i < 50; i++ {
          wg.Add(1)
          go func() {
              defer wg.Done()
              _ = reg.GetAllTestFunctions()
          }()
      }

      wg.Wait()

      funcs := reg.GetAllTestFunctions()
      if len(funcs) != 100 {
          t.Errorf("expected 100 functions, got %d", len(funcs))
      }
  }
  ```

#### 1.2 Implement Global Test Function Index

**Goal:** Parse and index ALL TestAcc* functions regardless of filename

- [ ] **1.2.1** Modify `parseTestFileWithHelpers()` in `parser.go:79-139`

  **Current Code Problem:**
  ```go
  // CURRENT - parser.go:82-86
  resourceName, isDataSource := extractResourceNameFromFilePath(filePath)
  if resourceName == "" {
      // Not a standard test file naming pattern
      return nil  // PROBLEM: Ignores non-standard test files!
  }
  ```

  **Implementation:**
  ```go
  // PROPOSED - parser.go
  func parseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *TestFileInfo {
      // Extract package name for context
      packageName := ""
      if file.Name != nil {
          packageName = file.Name.Name
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
              FilePath:         filePath,  // NEW: Store file path
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

      // CHANGED: Return TestFileInfo even if no resource name extracted
      // Resource association will happen in the linking phase
      if len(testFuncs) == 0 {
          return nil
      }

      return &TestFileInfo{
          FilePath:      filePath,
          PackageName:   packageName,  // NEW
          TestFunctions: testFuncs,
      }
  }
  ```

- [ ] **1.2.2** Update `buildRegistry()` Phase 2 in `parser.go:193-258`

  **Implementation:**
  ```go
  // buildRegistry - parser.go (REWRITE)
  func buildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
      reg := NewResourceRegistry()

      // PHASE 1: Scan for Resources (Type-based discovery via AST)
      for _, file := range pass.Files {
          filename := pass.Fset.Position(file.Pos()).Filename

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

          // Parse test file - now returns all TestAcc* functions
          testFileInfo := parseTestFileWithHelpers(file, pass.Fset, filename, settings.CustomTestHelpers)
          if testFileInfo == nil {
              continue
          }

          // Register test file by path
          reg.RegisterTestFile(testFileInfo)

          // Register each test function in global index
          for i := range testFileInfo.TestFunctions {
              fn := &testFileInfo.TestFunctions[i]
              fn.FilePath = filename
              reg.RegisterTestFunction(fn)
          }
      }

      // PHASE 3: Link tests to resources (NEW)
      linker := NewLinker(reg, settings)
      linker.LinkTestsToResources()

      return reg
  }
  ```

- [ ] **1.2.3** Add function name extraction utility in `utils.go`

  **Implementation:**
  ```go
  // utils.go - NEW FUNCTIONS

  import (
      "regexp"
      "strings"
  )

  // testFuncPattern matches TestAcc{Provider}{Resource}_{suffix}
  // Examples:
  //   TestAccAWSInstance_basic -> provider=AWS, resource=Instance
  //   TestAccWidget_basic -> provider="", resource=Widget
  //   TestAccDataSourceHTTP_basic -> prefix=DataSource, resource=HTTP
  var testFuncPatterns = []*regexp.Regexp{
      // Pattern 1: TestAcc{Provider}{Resource}_{suffix} (e.g., TestAccAWSInstance_basic)
      regexp.MustCompile(`^TestAcc([A-Z][a-z]+)?([A-Z][a-zA-Z0-9]+)_`),
      // Pattern 2: TestAccDataSource{Resource}_{suffix}
      regexp.MustCompile(`^TestAccDataSource([A-Z][a-zA-Z0-9]+)_`),
      // Pattern 3: TestAccResource{Resource}_{suffix}
      regexp.MustCompile(`^TestAccResource([A-Z][a-zA-Z0-9]+)_`),
      // Pattern 4: TestAcc{Resource}_{suffix} (no provider prefix)
      regexp.MustCompile(`^TestAcc([A-Z][a-zA-Z0-9]+)_`),
  }

  // ExtractResourceFromFuncName attempts to extract a resource name from a test function name.
  // Returns the resource name in snake_case and whether a match was found.
  //
  // Examples:
  //   TestAccAWSInstance_basic -> "instance", true
  //   TestAccWidget_basic -> "widget", true
  //   TestAccDataSourceHTTP_basic -> "http", true
  //   TestHelper -> "", false
  func ExtractResourceFromFuncName(funcName string) (string, bool) {
      // Try data source pattern first (more specific)
      if matches := testFuncPatterns[1].FindStringSubmatch(funcName); len(matches) > 1 {
          return toSnakeCase(matches[1]), true
      }

      // Try resource pattern
      if matches := testFuncPatterns[2].FindStringSubmatch(funcName); len(matches) > 1 {
          return toSnakeCase(matches[1]), true
      }

      // Try provider+resource pattern
      if matches := testFuncPatterns[0].FindStringSubmatch(funcName); len(matches) > 2 {
          // matches[1] = provider (optional), matches[2] = resource
          if matches[2] != "" {
              return toSnakeCase(matches[2]), true
          }
      }

      // Try simple pattern (no provider prefix)
      if matches := testFuncPatterns[3].FindStringSubmatch(funcName); len(matches) > 1 {
          return toSnakeCase(matches[1]), true
      }

      return "", false
  }

  // ExtractProviderFromFuncName extracts the provider prefix from a test function name.
  // Returns empty string if no provider prefix found.
  //
  // Examples:
  //   TestAccAWSInstance_basic -> "aws"
  //   TestAccWidget_basic -> ""
  func ExtractProviderFromFuncName(funcName string) string {
      if matches := testFuncPatterns[0].FindStringSubmatch(funcName); len(matches) > 1 {
          if matches[1] != "" {
              return strings.ToLower(matches[1])
          }
      }
      return ""
  }
  ```

- [ ] **1.2.4** Write unit tests for function parsing

  **Test file:** `utils_test.go`

  ```go
  func TestExtractResourceFromFuncName(t *testing.T) {
      tests := []struct {
          funcName     string
          wantResource string
          wantFound    bool
      }{
          // Standard patterns
          {"TestAccWidget_basic", "widget", true},
          {"TestAccAWSInstance_basic", "instance", true},
          {"TestAccGoogleComputeInstance_update", "compute_instance", true},
          {"TestAccAzureRMVirtualMachine_disappears", "virtual_machine", true},

          // Data source patterns
          {"TestAccDataSourceHTTP_basic", "http", true},
          {"TestAccDataSourceAWSAMI_filter", "ami", true},

          // Resource prefix patterns
          {"TestAccResourceWidget_basic", "widget", true},

          // Edge cases
          {"TestAccS3Bucket_basic", "bucket", true},           // Short provider
          {"TestAccEC2Instance_basic", "instance", true},       // EC2 edge case
          {"TestAccIAMRole_basic", "role", true},              // IAM edge case

          // Non-matching patterns
          {"TestHelper", "", false},
          {"TestUnit_something", "", false},
          {"BenchmarkWidget", "", false},
          {"testAccWidget_basic", "", false},  // lowercase
      }

      for _, tt := range tests {
          t.Run(tt.funcName, func(t *testing.T) {
              got, found := ExtractResourceFromFuncName(tt.funcName)
              if found != tt.wantFound {
                  t.Errorf("found = %v, want %v", found, tt.wantFound)
              }
              if got != tt.wantResource {
                  t.Errorf("resource = %q, want %q", got, tt.wantResource)
              }
          })
      }
  }

  func TestExtractProviderFromFuncName(t *testing.T) {
      tests := []struct {
          funcName     string
          wantProvider string
      }{
          {"TestAccAWSInstance_basic", "aws"},
          {"TestAccGoogleComputeInstance_update", "google"},
          {"TestAccAzureRMVirtualMachine_disappears", "azure"},
          {"TestAccWidget_basic", ""},  // No provider
          {"TestAccDataSourceHTTP_basic", ""},
      }

      for _, tt := range tests {
          t.Run(tt.funcName, func(t *testing.T) {
              got := ExtractProviderFromFuncName(tt.funcName)
              if got != tt.wantProvider {
                  t.Errorf("provider = %q, want %q", got, tt.wantProvider)
              }
          })
      }
  }
  ```

#### 1.3 Implement Resource-Function Linker

**Goal:** Create a linking pass that associates functions to resources using multiple strategies

- [ ] **1.3.1** Create new file `linker.go`

  **Implementation:**
  ```go
  // linker.go - NEW FILE
  package tfprovidertest

  import (
      "path/filepath"
      "strings"
  )

  // Linker associates test functions with resources using multiple strategies
  type Linker struct {
      registry *ResourceRegistry
      settings Settings
  }

  // NewLinker creates a new Linker instance
  func NewLinker(registry *ResourceRegistry, settings Settings) *Linker {
      return &Linker{
          registry: registry,
          settings: settings,
      }
  }

  // ResourceMatch represents a potential resource match for a test function
  type ResourceMatch struct {
      ResourceName string
      Confidence   float64
      MatchType    MatchType
  }

  // LinkTestsToResources iterates over all test functions and associates them with resources
  func (l *Linker) LinkTestsToResources() {
      allResources := l.registry.GetAllResources()
      allDataSources := l.registry.GetAllDataSources()

      // Merge resources and data sources for matching
      resourceNames := make(map[string]bool)
      for name := range allResources {
          resourceNames[name] = true
      }
      for name := range allDataSources {
          resourceNames[name] = true
      }

      for _, fn := range l.registry.GetAllTestFunctions() {
          matches := l.findResourceMatches(fn, resourceNames)

          // Update function with matches
          for _, match := range matches {
              fn.InferredResources = append(fn.InferredResources, match.ResourceName)
              fn.MatchConfidence = match.Confidence
              fn.MatchType = match.MatchType

              // Link to resource
              l.registry.LinkTestToResource(match.ResourceName, fn)
          }
      }
  }

  // findResourceMatches finds all matching resources for a test function
  func (l *Linker) findResourceMatches(fn *TestFunctionInfo, resourceNames map[string]bool) []ResourceMatch {
      var matches []ResourceMatch

      // Strategy 1: Function name extraction (highest confidence)
      if l.settings.EnableFunctionMatching {
          if resourceName, found := ExtractResourceFromFuncName(fn.Name); found {
              if resourceNames[resourceName] {
                  matches = append(matches, ResourceMatch{
                      ResourceName: resourceName,
                      Confidence:   1.0,
                      MatchType:    MatchTypeFunctionName,
                  })
                  return matches // High confidence match, no need to continue
              }
          }
      }

      // Strategy 2: File proximity (medium confidence)
      if l.settings.EnableFileMatching {
          if resourceName := l.matchByFileProximity(fn.FilePath, resourceNames); resourceName != "" {
              matches = append(matches, ResourceMatch{
                  ResourceName: resourceName,
                  Confidence:   0.8,
                  MatchType:    MatchTypeFileProximity,
              })
              return matches
          }
      }

      // Strategy 3: Fuzzy matching (low confidence, disabled by default)
      if l.settings.EnableFuzzyMatching {
          fuzzyMatches := l.findFuzzyMatches(fn.Name, resourceNames)
          for _, fm := range fuzzyMatches {
              if fm.Confidence >= l.settings.FuzzyMatchThreshold {
                  matches = append(matches, fm)
              }
          }
      }

      return matches
  }

  // matchByFileProximity tries to match based on file naming convention
  func (l *Linker) matchByFileProximity(testFilePath string, resourceNames map[string]bool) string {
      baseName := filepath.Base(testFilePath)

      // Remove _test.go suffix
      if !strings.HasSuffix(baseName, "_test.go") {
          return ""
      }
      nameWithoutTest := strings.TrimSuffix(baseName, "_test.go")

      // Try standard patterns
      patterns := []struct {
          prefix string
          suffix string
      }{
          {"resource_", ""},
          {"data_source_", ""},
          {"", "_resource"},
          {"", "_data_source"},
      }

      for _, p := range patterns {
          resourceName := nameWithoutTest
          if p.prefix != "" && strings.HasPrefix(resourceName, p.prefix) {
              resourceName = strings.TrimPrefix(resourceName, p.prefix)
          }
          if p.suffix != "" && strings.HasSuffix(resourceName, p.suffix) {
              resourceName = strings.TrimSuffix(resourceName, p.suffix)
          }

          if resourceNames[resourceName] {
              return resourceName
          }
      }

      return ""
  }

  // findFuzzyMatches finds resources with similar names using Levenshtein distance
  func (l *Linker) findFuzzyMatches(funcName string, resourceNames map[string]bool) []ResourceMatch {
      var matches []ResourceMatch

      // Extract potential resource name from function
      resourceFromFunc, _ := ExtractResourceFromFuncName(funcName)
      if resourceFromFunc == "" {
          return matches
      }

      for resourceName := range resourceNames {
          confidence := calculateSimilarity(resourceFromFunc, resourceName)
          if confidence >= l.settings.FuzzyMatchThreshold {
              matches = append(matches, ResourceMatch{
                  ResourceName: resourceName,
                  Confidence:   confidence,
                  MatchType:    MatchTypeFuzzy,
              })
          }
      }

      return matches
  }

  // calculateSimilarity calculates string similarity using normalized Levenshtein distance
  func calculateSimilarity(a, b string) float64 {
      if a == b {
          return 1.0
      }

      distance := levenshteinDistance(a, b)
      maxLen := len(a)
      if len(b) > maxLen {
          maxLen = len(b)
      }

      if maxLen == 0 {
          return 1.0
      }

      return 1.0 - float64(distance)/float64(maxLen)
  }

  // levenshteinDistance calculates the Levenshtein distance between two strings
  func levenshteinDistance(a, b string) int {
      if len(a) == 0 {
          return len(b)
      }
      if len(b) == 0 {
          return len(a)
      }

      // Create matrix
      matrix := make([][]int, len(a)+1)
      for i := range matrix {
          matrix[i] = make([]int, len(b)+1)
          matrix[i][0] = i
      }
      for j := range matrix[0] {
          matrix[0][j] = j
      }

      // Fill matrix
      for i := 1; i <= len(a); i++ {
          for j := 1; j <= len(b); j++ {
              cost := 1
              if a[i-1] == b[j-1] {
                  cost = 0
              }
              matrix[i][j] = min(
                  matrix[i-1][j]+1,      // deletion
                  matrix[i][j-1]+1,      // insertion
                  matrix[i-1][j-1]+cost, // substitution
              )
          }
      }

      return matrix[len(a)][len(b)]
  }

  func min(nums ...int) int {
      m := nums[0]
      for _, n := range nums[1:] {
          if n < m {
              m = n
          }
      }
      return m
  }
  ```

- [ ] **1.3.2-1.3.5** (Covered in implementation above)

- [ ] **1.3.6** Write unit tests for linker

  **Test file:** `linker_test.go`

  ```go
  package tfprovidertest

  import (
      "testing"
  )

  func TestLinkerFunctionNameMatching(t *testing.T) {
      reg := NewResourceRegistry()

      // Register resources
      reg.RegisterResource(&ResourceInfo{Name: "widget"})
      reg.RegisterResource(&ResourceInfo{Name: "instance"})

      // Register test functions
      fn1 := &TestFunctionInfo{Name: "TestAccWidget_basic", FilePath: "/test.go"}
      fn2 := &TestFunctionInfo{Name: "TestAccInstance_update", FilePath: "/test.go"}
      reg.RegisterTestFunction(fn1)
      reg.RegisterTestFunction(fn2)

      // Run linker
      settings := DefaultSettings()
      settings.EnableFunctionMatching = true
      linker := NewLinker(reg, settings)
      linker.LinkTestsToResources()

      // Verify matches
      widgetTests := reg.GetResourceTests("widget")
      if len(widgetTests) != 1 {
          t.Errorf("expected 1 widget test, got %d", len(widgetTests))
      }
      if widgetTests[0].MatchType != MatchTypeFunctionName {
          t.Errorf("expected MatchTypeFunctionName, got %v", widgetTests[0].MatchType)
      }
      if widgetTests[0].MatchConfidence != 1.0 {
          t.Errorf("expected confidence 1.0, got %f", widgetTests[0].MatchConfidence)
      }
  }

  func TestLinkerFileProximityMatching(t *testing.T) {
      reg := NewResourceRegistry()
      reg.RegisterResource(&ResourceInfo{Name: "widget"})

      // Test function with non-standard name but matching file
      fn := &TestFunctionInfo{
          Name:     "TestWidgetOperations_all",  // Doesn't follow TestAcc pattern
          FilePath: "/path/to/resource_widget_test.go",
      }
      reg.RegisterTestFunction(fn)

      settings := DefaultSettings()
      settings.EnableFunctionMatching = true
      settings.EnableFileMatching = true
      linker := NewLinker(reg, settings)
      linker.LinkTestsToResources()

      widgetTests := reg.GetResourceTests("widget")
      if len(widgetTests) != 1 {
          t.Errorf("expected 1 widget test, got %d", len(widgetTests))
      }
      if widgetTests[0].MatchType != MatchTypeFileProximity {
          t.Errorf("expected MatchTypeFileProximity, got %v", widgetTests[0].MatchType)
      }
  }

  func TestLevenshteinDistance(t *testing.T) {
      tests := []struct {
          a, b     string
          expected int
      }{
          {"", "", 0},
          {"abc", "abc", 0},
          {"abc", "abd", 1},
          {"abc", "abcd", 1},
          {"kitten", "sitting", 3},
      }

      for _, tt := range tests {
          got := levenshteinDistance(tt.a, tt.b)
          if got != tt.expected {
              t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
          }
      }
  }

  func TestCalculateSimilarity(t *testing.T) {
      tests := []struct {
          a, b        string
          minExpected float64
      }{
          {"widget", "widget", 1.0},
          {"widget", "widgets", 0.8},  // 1 char difference
          {"instance", "instances", 0.8},
          {"bucket", "socket", 0.5},   // Different words
      }

      for _, tt := range tests {
          got := calculateSimilarity(tt.a, tt.b)
          if got < tt.minExpected {
              t.Errorf("calculateSimilarity(%q, %q) = %f, want >= %f", tt.a, tt.b, got, tt.minExpected)
          }
      }
  }
  ```

---

### Phase 2: Test Step Analysis Improvements

#### 2.1 Enhance TestStepInfo for Update Detection

**Goal:** Accurately distinguish update steps from import/idempotency steps

- [ ] **2.1.1** Update `TestStepInfo` struct in `registry.go:194-205`

  **Current Code:**
  ```go
  // CURRENT - registry.go:194-205
  type TestStepInfo struct {
      StepNumber        int
      StepPos           token.Pos
      Config            string
      HasConfig         bool
      HasCheck          bool
      CheckFunctions    []string
      ImportState       bool
      ImportStateVerify bool
      ExpectError       bool
  }
  ```

  **Implementation:**
  ```go
  // PROPOSED - registry.go
  type TestStepInfo struct {
      StepNumber        int
      StepPos           token.Pos
      Config            string
      ConfigHash        string    // NEW: Hash of config for comparison
      HasConfig         bool
      HasCheck          bool
      CheckFunctions    []string
      ImportState       bool
      ImportStateVerify bool
      ExpectError       bool
      IsUpdateStep      bool      // NEW: True if this modifies existing resource
      PreviousConfigHash string   // NEW: Hash of previous step's config
  }
  ```

- [ ] **2.1.2** Implement config hashing in `parser.go`

  **Implementation:**
  ```go
  // parser.go - NEW FUNCTIONS

  import (
      "crypto/sha256"
      "encoding/hex"
      "go/ast"
      "go/printer"
      "strings"
  )

  // hashConfigExpr generates a hash of a config expression for comparison
  // This normalizes the AST representation to detect equivalent configs
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
  ```

- [ ] **2.1.3** Implement update step detection logic

  **Implementation:**
  ```go
  // parser.go - ADD TO extractStepsFromTestCase

  // DetermineIfUpdateStep checks if a step is an update step
  // A step is an update step if:
  // 1. It's not the first step (StepNumber > 0)
  // 2. It's not an import step
  // 3. It has a config
  // 4. Its config differs from the previous step's config
  func (s *TestStepInfo) DetermineIfUpdateStep(prevStep *TestStepInfo) bool {
      // First step is never an update
      if s.StepNumber == 0 {
          return false
      }

      // Import steps are not update steps
      if s.ImportState {
          return false
      }

      // Must have a config
      if !s.HasConfig {
          return false
      }

      // If no previous step or previous has no config, this is the first config
      if prevStep == nil || !prevStep.HasConfig {
          return false
      }

      // Config must differ from previous (not an idempotency test)
      if s.ConfigHash == prevStep.ConfigHash {
          return false
      }

      return true
  }
  ```

- [ ] **2.1.4** Update `extractTestSteps()` to compute update flags

  **Implementation:**
  ```go
  // parser.go - MODIFY extractStepsFromTestCase

  func extractStepsFromTestCase(testCaseExpr ast.Expr, stepNumber *int) []TestStepInfo {
      var steps []TestStepInfo

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

          stepsLit, ok := kv.Value.(*ast.CompositeLit)
          if !ok {
              continue
          }

          // Parse each step
          for _, stepExpr := range stepsLit.Elts {
              step := parseTestStepWithHash(stepExpr, *stepNumber)
              steps = append(steps, step)
              *stepNumber++
          }
      }

      // SECOND PASS: Determine update steps
      for i := range steps {
          if i > 0 {
              steps[i].PreviousConfigHash = steps[i-1].ConfigHash
              steps[i].IsUpdateStep = steps[i].DetermineIfUpdateStep(&steps[i-1])
          }
      }

      return steps
  }

  // parseTestStepWithHash parses a step and computes its config hash
  func parseTestStepWithHash(stepExpr ast.Expr, stepNum int) TestStepInfo {
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
              step.ConfigHash = hashConfigExpr(kv.Value)  // NEW
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
  ```

- [ ] **2.1.5** Write unit tests for update detection

  **Test file:** `parser_test.go`

  ```go
  func TestDetermineIfUpdateStep(t *testing.T) {
      tests := []struct {
          name     string
          step     TestStepInfo
          prevStep *TestStepInfo
          want     bool
      }{
          {
              name:     "first step is not update",
              step:     TestStepInfo{StepNumber: 0, HasConfig: true, ConfigHash: "abc"},
              prevStep: nil,
              want:     false,
          },
          {
              name:     "import step is not update",
              step:     TestStepInfo{StepNumber: 1, ImportState: true},
              prevStep: &TestStepInfo{HasConfig: true, ConfigHash: "abc"},
              want:     false,
          },
          {
              name:     "same config is idempotency test",
              step:     TestStepInfo{StepNumber: 1, HasConfig: true, ConfigHash: "abc"},
              prevStep: &TestStepInfo{HasConfig: true, ConfigHash: "abc"},
              want:     false,
          },
          {
              name:     "different config is update step",
              step:     TestStepInfo{StepNumber: 1, HasConfig: true, ConfigHash: "def"},
              prevStep: &TestStepInfo{HasConfig: true, ConfigHash: "abc"},
              want:     true,
          },
      }

      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              got := tt.step.DetermineIfUpdateStep(tt.prevStep)
              if got != tt.want {
                  t.Errorf("DetermineIfUpdateStep() = %v, want %v", got, tt.want)
              }
          })
      }
  }
  ```

#### 2.2 Improve Updatable Attribute Detection

**Goal:** Better detect RequiresReplace modifiers and support suppressions

- [ ] **2.2.1** Create whitelist of standard RequiresReplace modifiers in `utils.go`

  **Implementation:**
  ```go
  // utils.go - NEW

  // standardRequiresReplaceModifiers lists all known RequiresReplace plan modifiers
  // from the terraform-plugin-framework library
  var standardRequiresReplaceModifiers = map[string]bool{
      "stringplanmodifier.RequiresReplace":    true,
      "stringplanmodifier.RequiresReplaceIf":  true,
      "int64planmodifier.RequiresReplace":     true,
      "int64planmodifier.RequiresReplaceIf":   true,
      "boolplanmodifier.RequiresReplace":      true,
      "boolplanmodifier.RequiresReplaceIf":    true,
      "float64planmodifier.RequiresReplace":   true,
      "float64planmodifier.RequiresReplaceIf": true,
      "listplanmodifier.RequiresReplace":      true,
      "listplanmodifier.RequiresReplaceIf":    true,
      "mapplanmodifier.RequiresReplace":       true,
      "mapplanmodifier.RequiresReplaceIf":     true,
      "setplanmodifier.RequiresReplace":       true,
      "setplanmodifier.RequiresReplaceIf":     true,
      "objectplanmodifier.RequiresReplace":    true,
      "objectplanmodifier.RequiresReplaceIf":  true,
      "numberplanmodifier.RequiresReplace":    true,
      "numberplanmodifier.RequiresReplaceIf":  true,
  }
  ```

- [ ] **2.2.2** Enhance `hasRequiresReplace()` function in `utils.go:167-188`

  **Implementation:**
  ```go
  // utils.go - REPLACE hasRequiresReplace

  // RequiresReplaceResult indicates the confidence level of RequiresReplace detection
  type RequiresReplaceResult struct {
      Found      bool
      Confidence string // "certain", "uncertain", "none"
      ModifierName string
  }

  // hasRequiresReplaceWithConfidence checks if a node contains RequiresReplace modifier
  // Returns the result with confidence level
  func hasRequiresReplaceWithConfidence(node ast.Node) RequiresReplaceResult {
      result := RequiresReplaceResult{Confidence: "none"}

      ast.Inspect(node, func(n ast.Node) bool {
          call, ok := n.(*ast.CallExpr)
          if !ok {
              return true
          }

          // Check for selector expression (package.Function())
          if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
              funcName := sel.Sel.Name

              // Check if it's a RequiresReplace function
              if strings.Contains(funcName, "RequiresReplace") {
                  // Try to get full qualifier
                  if pkgIdent, ok := sel.X.(*ast.Ident); ok {
                      fullName := pkgIdent.Name + "." + funcName
                      if standardRequiresReplaceModifiers[fullName] {
                          result = RequiresReplaceResult{
                              Found:        true,
                              Confidence:   "certain",
                              ModifierName: fullName,
                          }
                          return false
                      }
                  }

                  // Unknown modifier with RequiresReplace in name
                  result = RequiresReplaceResult{
                      Found:        true,
                      Confidence:   "uncertain",
                      ModifierName: funcName,
                  }
                  return false
              }
          }

          return true
      })

      return result
  }

  // hasRequiresReplace is the simple boolean version for backward compatibility
  func hasRequiresReplace(node ast.Node) bool {
      return hasRequiresReplaceWithConfidence(node).Found
  }
  ```

- [ ] **2.2.3** Implement conservative default for unknown modifiers

  **Implementation:**
  ```go
  // utils.go - MODIFY extractAttributes

  // In parseAttributesMap function, update PlanModifiers handling:
  case "PlanModifiers":
      result := hasRequiresReplaceWithConfidence(attrKV.Value)
      if result.Found {
          attr.IsUpdatable = false
      } else if result.Confidence == "uncertain" {
          // Unknown modifier - default to updatable (conservative)
          attr.IsUpdatable = true
          // Could log warning here in verbose mode
      }
  ```

- [ ] **2.2.4** Implement suppression comment detection

  **Implementation:**
  ```go
  // utils.go - NEW FUNCTIONS

  import (
      "go/ast"
      "regexp"
  )

  // suppressionPatterns matches lint suppression comments
  var suppressionPatterns = []*regexp.Regexp{
      regexp.MustCompile(`//\s*lint:ignore\s+tfprovider-(\S+)`),
      regexp.MustCompile(`//\s*nolint:tfprovider-(\S+)`),
      regexp.MustCompile(`//\s*tfprovider:ignore:(\S+)`),
  }

  // CheckSuppressionComment checks if a node has a suppression comment for a specific check
  func CheckSuppressionComment(comments []*ast.CommentGroup, checkName string) bool {
      if comments == nil {
          return false
      }

      for _, cg := range comments {
          for _, c := range cg.List {
              for _, pattern := range suppressionPatterns {
                  if matches := pattern.FindStringSubmatch(c.Text); len(matches) > 1 {
                      if matches[1] == checkName || matches[1] == "all" {
                          return true
                      }
                  }
              }
          }
      }

      return false
  }

  // GetSuppressedChecks returns all suppressed check names from comments
  func GetSuppressedChecks(comments []*ast.CommentGroup) []string {
      var suppressed []string

      if comments == nil {
          return suppressed
      }

      for _, cg := range comments {
          for _, c := range cg.List {
              for _, pattern := range suppressionPatterns {
                  if matches := pattern.FindStringSubmatch(c.Text); len(matches) > 1 {
                      suppressed = append(suppressed, matches[1])
                  }
              }
          }
      }

      return suppressed
  }
  ```

- [ ] **2.2.5** Write unit tests for attribute detection

  **Test file:** `utils_test.go`

  ```go
  func TestHasRequiresReplaceWithConfidence(t *testing.T) {
      // Test with known modifier
      src := `stringplanmodifier.RequiresReplace()`
      expr := parseExpr(t, src)
      result := hasRequiresReplaceWithConfidence(expr)
      if !result.Found || result.Confidence != "certain" {
          t.Errorf("expected certain match, got %+v", result)
      }

      // Test with unknown modifier
      src = `customplanmodifier.RequiresReplace()`
      expr = parseExpr(t, src)
      result = hasRequiresReplaceWithConfidence(expr)
      if !result.Found || result.Confidence != "uncertain" {
          t.Errorf("expected uncertain match, got %+v", result)
      }
  }

  func TestCheckSuppressionComment(t *testing.T) {
      tests := []struct {
          comment   string
          checkName string
          want      bool
      }{
          {"//lint:ignore tfprovider-basic-test", "basic-test", true},
          {"//nolint:tfprovider-update-test", "update-test", true},
          {"//tfprovider:ignore:all", "any-check", true},
          {"// Regular comment", "basic-test", false},
      }

      for _, tt := range tests {
          cg := &ast.CommentGroup{
              List: []*ast.Comment{{Text: tt.comment}},
          }
          got := CheckSuppressionComment([]*ast.CommentGroup{cg}, tt.checkName)
          if got != tt.want {
              t.Errorf("CheckSuppressionComment(%q, %q) = %v, want %v",
                  tt.comment, tt.checkName, got, tt.want)
          }
      }
  }
  ```

---

### Phase 3: Helper and Sweeper Handling

#### 3.1 Enhance Custom Test Helper Detection

**Goal:** Detect local package helpers and custom test wrappers

- [ ] **3.1.1** Add local helper discovery in `parser.go`

  **Implementation:**
  ```go
  // parser.go - NEW FUNCTIONS

  // LocalHelper represents a discovered local test helper function
  type LocalHelper struct {
      Name     string
      FilePath string
      FuncDecl *ast.FuncDecl
  }

  // findLocalTestHelpers discovers functions that wrap resource.Test()
  // These are functions that:
  // 1. Don't have "Test" prefix (not test functions themselves)
  // 2. Call resource.Test() or resource.ParallelTest()
  // 3. Accept *testing.T as a parameter
  func findLocalTestHelpers(files []*ast.File, fset *token.FileSet) []LocalHelper {
      var helpers []LocalHelper

      for _, file := range files {
          filePath := fset.Position(file.Pos()).Filename

          // Skip non-test files (helpers are usually in test files)
          if !strings.HasSuffix(filePath, "_test.go") {
              continue
          }

          ast.Inspect(file, func(n ast.Node) bool {
              funcDecl, ok := n.(*ast.FuncDecl)
              if !ok || funcDecl.Body == nil {
                  return true
              }

              name := funcDecl.Name.Name

              // Skip actual test functions
              if strings.HasPrefix(name, "Test") {
                  return true
              }

              // Skip unexported functions
              if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
                  return true
              }

              // Check if it accepts *testing.T
              if !acceptsTestingT(funcDecl) {
                  return true
              }

              // Check if it calls resource.Test()
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

  // acceptsTestingT checks if a function has *testing.T as a parameter
  func acceptsTestingT(funcDecl *ast.FuncDecl) bool {
      if funcDecl.Type.Params == nil {
          return false
      }

      for _, param := range funcDecl.Type.Params.List {
          // Check for *testing.T
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
  ```

- [ ] **3.1.2** Enhance `checkUsesResourceTestWithHelpers()` in `parser.go:265-301`

  **Implementation:**
  ```go
  // parser.go - MODIFY checkUsesResourceTestWithHelpers

  // checkUsesResourceTestWithHelpers checks if a function body contains a call to resource.Test()
  // or any of the custom/local test helper functions.
  func checkUsesResourceTestWithHelpers(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper) bool {
      if body == nil {
          return false
      }

      // Build lookup map for local helpers
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

          // Check for resource.Test() call (selector expression)
          if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
              if ident, ok := sel.X.(*ast.Ident); ok {
                  // Standard resource.Test() or resource.ParallelTest()
                  if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
                      found = true
                      return false
                  }

                  // Custom test helpers (package.Function format)
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

          // Check for local helper calls (direct function call)
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
  ```

- [ ] **3.1.3** Add helper call chain tracking

  **Implementation:**
  ```go
  // registry.go - ADD to TestFunctionInfo

  type TestFunctionInfo struct {
      // ... existing fields ...
      HelperUsed string  // NEW: Name of helper function used (if any)
  }

  // parser.go - MODIFY test function parsing to track helper

  func parseTestFunctionWithHelper(funcDecl *ast.FuncDecl, customHelpers []string, localHelpers []LocalHelper) TestFunctionInfo {
      info := TestFunctionInfo{
          Name:        funcDecl.Name.Name,
          FunctionPos: funcDecl.Pos(),
      }

      // Detect which helper/pattern is used
      ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
          call, ok := n.(*ast.CallExpr)
          if !ok {
              return true
          }

          if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
              if ident, ok := sel.X.(*ast.Ident); ok {
                  if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
                      info.HelperUsed = "resource." + sel.Sel.Name
                      info.UsesResourceTest = true
                      return false
                  }
              }
          }

          // Check local helpers
          if ident, ok := call.Fun.(*ast.Ident); ok {
              for _, h := range localHelpers {
                  if h.Name == ident.Name {
                      info.HelperUsed = ident.Name
                      info.UsesResourceTest = true
                      return false
                  }
              }
          }

          return true
      })

      return info
  }
  ```

- [ ] **3.1.4** Write unit tests for helper detection

  **Test file:** `parser_test.go`

  ```go
  func TestFindLocalTestHelpers(t *testing.T) {
      src := `
  package provider_test

  import (
      "testing"
      "github.com/hashicorp/terraform-plugin-testing/helper/resource"
  )

  // AccTestHelper is a custom test wrapper
  func AccTestHelper(t *testing.T, tc resource.TestCase) {
      resource.Test(t, tc)
  }

  // TestAccWidget_basic is an actual test
  func TestAccWidget_basic(t *testing.T) {
      AccTestHelper(t, resource.TestCase{})
  }
  `

      fset := token.NewFileSet()
      file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
      if err != nil {
          t.Fatal(err)
      }

      helpers := findLocalTestHelpers([]*ast.File{file}, fset)
      if len(helpers) != 1 {
          t.Errorf("expected 1 helper, got %d", len(helpers))
      }
      if helpers[0].Name != "AccTestHelper" {
          t.Errorf("expected AccTestHelper, got %s", helpers[0].Name)
      }
  }
  ```

#### 3.2 Improve File Exclusion Patterns

**Goal:** Make exclusion patterns configurable and observable

- [ ] **3.2.1** Update `Settings` struct in `settings.go:7-40`

  **Implementation:**
  ```go
  // settings.go - ADD new fields

  type Settings struct {
      // ... existing fields ...

      // ExcludePatterns defines glob patterns for files to exclude from analysis
      // Default: ["*_sweeper.go", "*_test_helpers.go", "test_*.go"]
      ExcludePatterns []string `yaml:"exclude-patterns"`

      // IncludeHelperPatterns defines patterns to recognize as test helpers
      // Default: ["*Helper*", "*Wrapper*"]
      IncludeHelperPatterns []string `yaml:"include-helper-patterns"`

      // DiagnosticExclusions enables verbose output for excluded files
      DiagnosticExclusions bool `yaml:"diagnostic-exclusions"`

      // EnableFunctionMatching enables function name-based test matching
      EnableFunctionMatching bool `yaml:"enable-function-matching"`

      // EnableFuzzyMatching enables fuzzy string matching for resources
      EnableFuzzyMatching bool `yaml:"enable-fuzzy-matching"`

      // FuzzyMatchThreshold sets minimum similarity for fuzzy matches (0.0-1.0)
      FuzzyMatchThreshold float64 `yaml:"fuzzy-match-threshold"`
  }
  ```

- [ ] **3.2.2** Implement glob pattern matching in `parser.go`

  **Implementation:**
  ```go
  // parser.go - NEW FUNCTION

  // ExclusionResult tracks why a file was excluded
  type ExclusionResult struct {
      FilePath       string
      Excluded       bool
      Reason         string
      MatchedPattern string
  }

  // matchesExcludePattern checks if a file should be excluded and returns details
  func matchesExcludePattern(filePath string, patterns []string) ExclusionResult {
      baseName := filepath.Base(filePath)

      for _, pattern := range patterns {
          // Try matching against base name
          if matched, _ := filepath.Match(pattern, baseName); matched {
              return ExclusionResult{
                  FilePath:       filePath,
                  Excluded:       true,
                  Reason:         "matched exclusion pattern",
                  MatchedPattern: pattern,
              }
          }

          // Try matching against full path
          if matched, _ := filepath.Match(pattern, filePath); matched {
              return ExclusionResult{
                  FilePath:       filePath,
                  Excluded:       true,
                  Reason:         "matched exclusion pattern (full path)",
                  MatchedPattern: pattern,
              }
          }
      }

      return ExclusionResult{
          FilePath: filePath,
          Excluded: false,
      }
  }
  ```

- [ ] **3.2.3** Add exclusion diagnostics

  **Implementation:**
  ```go
  // parser.go - MODIFY buildRegistry

  // ExclusionDiagnostics collects information about excluded files
  type ExclusionDiagnostics struct {
      ExcludedFiles []ExclusionResult
      TotalExcluded int
  }

  // In buildRegistry, track exclusions:
  func buildRegistry(pass *analysis.Pass, settings Settings) *ResourceRegistry {
      reg := NewResourceRegistry()
      var diagnostics ExclusionDiagnostics

      for _, file := range pass.Files {
          filename := pass.Fset.Position(file.Pos()).Filename

          // Check exclusion patterns
          if result := matchesExcludePattern(filename, settings.ExcludePatterns); result.Excluded {
              diagnostics.ExcludedFiles = append(diagnostics.ExcludedFiles, result)
              diagnostics.TotalExcluded++
              continue
          }

          // ... rest of processing ...
      }

      // Log diagnostics if enabled
      if settings.DiagnosticExclusions && diagnostics.TotalExcluded > 0 {
          for _, excl := range diagnostics.ExcludedFiles {
              log.Printf("[DIAGNOSTIC] Excluded %s: %s (pattern: %s)",
                  excl.FilePath, excl.Reason, excl.MatchedPattern)
          }
          log.Printf("[DIAGNOSTIC] Total files excluded: %d", diagnostics.TotalExcluded)
      }

      return reg
  }
  ```

- [ ] **3.2.4** Update default exclusion patterns

  **Implementation:**
  ```go
  // settings.go - MODIFY DefaultSettings

  func DefaultSettings() Settings {
      return Settings{
          // ... existing defaults ...

          ExcludePatterns: []string{
              "*_sweeper.go",
              "*_sweeper_test.go",
              "*_test_helpers.go",
              "test_helpers_*.go",
              "*_acc_test_helpers.go",
          },
          IncludeHelperPatterns: []string{
              "*Helper*",
              "*Wrapper*",
              "AccTest*",
          },
          DiagnosticExclusions:   false,
          EnableFunctionMatching: true,
          EnableFileMatching:     true,
          EnableFuzzyMatching:    false,  // Disabled by default
          FuzzyMatchThreshold:    0.7,
      }
  }
  ```

- [ ] **3.2.5** Write unit tests for exclusion patterns

  **Test file:** `parser_test.go`

  ```go
  func TestMatchesExcludePattern(t *testing.T) {
      patterns := []string{"*_sweeper.go", "*_test_helpers.go"}

      tests := []struct {
          filePath string
          excluded bool
          pattern  string
      }{
          {"/path/to/resource_widget_sweeper.go", true, "*_sweeper.go"},
          {"/path/to/acc_test_helpers.go", true, "*_test_helpers.go"},
          {"/path/to/resource_widget.go", false, ""},
          {"/path/to/resource_widget_test.go", false, ""},
      }

      for _, tt := range tests {
          result := matchesExcludePattern(tt.filePath, patterns)
          if result.Excluded != tt.excluded {
              t.Errorf("matchesExcludePattern(%q) excluded = %v, want %v",
                  tt.filePath, result.Excluded, tt.excluded)
          }
          if tt.excluded && result.MatchedPattern != tt.pattern {
              t.Errorf("matchesExcludePattern(%q) pattern = %q, want %q",
                  tt.filePath, result.MatchedPattern, tt.pattern)
          }
      }
  }
  ```

---

### Phase 4: Analyzer Improvements

#### 4.1 Update Analyzers for New Registry

**Goal:** Update all analyzers to use the new function-first registry

- [ ] **4.1.1** Update `runBasicTestAnalyzer()` in `analyzer.go:48-110`

  **Implementation:**
  ```go
  // analyzer.go - REWRITE runBasicTestAnalyzer

  func runBasicTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
      settings := DefaultSettings()
      registry := buildRegistry(pass, settings)

      // Report untested resources
      untested := registry.GetUntestedResources()
      for _, resource := range untested {
          resourceType := "resource"
          resourceTypeTitle := "Resource"
          if resource.IsDataSource {
              resourceType = "data source"
              resourceTypeTitle = "Data source"
          }

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

      // Report resources with tests but potential issues
      for name, resource := range registry.GetAllResources() {
          tests := registry.GetResourceTests(name)
          if len(tests) == 0 {
              continue // Already reported above
          }

          // Check for low-confidence matches
          for _, test := range tests {
              if test.MatchConfidence < 0.8 && settings.ShowMatchConfidence {
                  pos := pass.Fset.Position(resource.SchemaPos)
                  msg := fmt.Sprintf("resource '%s' matched to test '%s' with %.0f%% confidence (%s match)",
                      name, test.Name, test.MatchConfidence*100, test.MatchType)
                  pass.Reportf(pos, "%s", msg)
              }
          }
      }

      return nil, nil
  }
  ```

- [ ] **4.1.2** Update `runUpdateTestAnalyzer()` in `analyzer.go:112-156`

  **Implementation:**
  ```go
  // analyzer.go - REWRITE runUpdateTestAnalyzer

  func runUpdateTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
      settings := DefaultSettings()
      registry := buildRegistry(pass, settings)

      for name, resource := range registry.GetAllResources() {
          // Check if resource has updatable attributes
          hasUpdatable := false
          var updatableAttrs []string
          for _, attr := range resource.Attributes {
              if attr.NeedsUpdateTest() {
                  hasUpdatable = true
                  updatableAttrs = append(updatableAttrs, attr.Name)
              }
          }

          if !hasUpdatable {
              continue
          }

          // Get all test functions for this resource
          tests := registry.GetResourceTests(name)
          if len(tests) == 0 {
              continue // Covered by BasicTestAnalyzer
          }

          // Check if any test has an actual update step (not just multiple steps)
          hasUpdateTest := false
          for _, test := range tests {
              for _, step := range test.TestSteps {
                  if step.IsUpdateStep {  // NEW: Use computed flag
                      hasUpdateTest = true
                      break
                  }
              }
              if hasUpdateTest {
                  break
              }
          }

          if !hasUpdateTest {
              pos := pass.Fset.Position(resource.SchemaPos)
              msg := fmt.Sprintf("resource '%s' has updatable attributes but no update test coverage\n"+
                  "  Updatable attributes: %s\n"+
                  "  Suggestion: Add a test step that modifies one of these attributes",
                  name, strings.Join(updatableAttrs, ", "))
              pass.Reportf(pos, "%s", msg)
          }
      }

      return nil, nil
  }
  ```

- [ ] **4.1.3** Update `runImportTestAnalyzer()` in `analyzer.go:158-192`

  **Implementation:**
  ```go
  // analyzer.go - REWRITE runImportTestAnalyzer

  func runImportTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
      settings := DefaultSettings()
      registry := buildRegistry(pass, settings)

      for name, resource := range registry.GetAllResources() {
          if !resource.HasImportState {
              continue
          }

          tests := registry.GetResourceTests(name)
          if len(tests) == 0 {
              continue // Covered by BasicTestAnalyzer
          }

          // Check if ANY test function has an import step
          hasImportTest := false
          for _, test := range tests {
              if test.HasImportStep {
                  hasImportTest = true
                  break
              }
          }

          if !hasImportTest {
              pos := pass.Fset.Position(resource.SchemaPos)
              msg := fmt.Sprintf("resource '%s' implements ImportState but has no import test coverage\n"+
                  "  Suggestion: Add a test step with ImportState: true, ImportStateVerify: true",
                  name)
              pass.Reportf(pos, "%s", msg)
          }
      }

      return nil, nil
  }
  ```

- [ ] **4.1.4** Update `runErrorTestAnalyzer()` in `analyzer.go:194-236`

  **Implementation:**
  ```go
  // analyzer.go - REWRITE runErrorTestAnalyzer

  func runErrorTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
      settings := DefaultSettings()
      registry := buildRegistry(pass, settings)

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
              continue
          }

          tests := registry.GetResourceTests(name)
          if len(tests) == 0 {
              continue // Covered by BasicTestAnalyzer
          }

          // Check if ANY test function has an error case
          hasErrorTest := false
          for _, test := range tests {
              if test.HasErrorCase {
                  hasErrorTest = true
                  break
              }
          }

          if !hasErrorTest {
              pos := pass.Fset.Position(resource.SchemaPos)
              msg := fmt.Sprintf("resource '%s' has validation rules but no error case tests\n"+
                  "  Validated attributes: %s\n"+
                  "  Suggestion: Add a test step with ExpectError to verify validation",
                  name, strings.Join(validatedAttrs, ", "))
              pass.Reportf(pos, "%s", msg)
          }
      }

      return nil, nil
  }
  ```

- [ ] **4.1.5** Update `runStateCheckAnalyzer()` in `analyzer.go:238-264`

  **Implementation:**
  ```go
  // analyzer.go - REWRITE runStateCheckAnalyzer

  func runStateCheckAnalyzer(pass *analysis.Pass) (interface{}, error) {
      settings := DefaultSettings()
      registry := buildRegistry(pass, settings)

      // Check ALL test functions, not just those with resource associations
      for _, fn := range registry.GetAllTestFunctions() {
          for _, step := range fn.TestSteps {
              // Skip import and error test steps
              if step.ImportState || step.ExpectError {
                  continue
              }

              if !step.HasCheck && step.HasConfig {
                  pos := step.StepPos
                  if pos == 0 {
                      pos = fn.FunctionPos
                  }

                  resourceContext := ""
                  if len(fn.InferredResources) > 0 {
                      resourceContext = fmt.Sprintf(" (testing %s)", strings.Join(fn.InferredResources, ", "))
                  }

                  pass.Reportf(pos, "test step in %s%s has no state validation checks\n"+
                      "  Suggestion: Add Check: resource.ComposeTestCheckFunc(...) to verify state",
                      fn.Name, resourceContext)
              }
          }
      }

      return nil, nil
  }
  ```

- [ ] **4.1.6** Write integration tests for updated analyzers

  **Test file:** `analyzer_test.go`

  ```go
  func TestBasicAnalyzerWithFunctionMatching(t *testing.T) {
      // Test that function-name-based matching works
      src := `
  package provider

  type WidgetResource struct{}
  func (r *WidgetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
      resp.Schema = schema.Schema{}
  }
  `
      testSrc := `
  package provider

  import "testing"

  func TestAccWidget_basic(t *testing.T) {
      resource.Test(t, resource.TestCase{
          Steps: []resource.TestStep{{Config: "test"}},
      })
  }
  `
      // Run analyzer and verify no issues reported
      // (Widget resource matched via function name)
  }
  ```

#### 4.2 Add Confidence-Based Reporting

**Goal:** Include match confidence in reports and support severity levels

- [ ] **4.2.1** Update `Report` struct

  **Implementation:**
  ```go
  // report.go - NEW FILE

  package tfprovidertest

  import (
      "fmt"
      "go/token"
  )

  // Severity indicates the importance of a report
  type Severity int

  const (
      SeverityInfo Severity = iota
      SeverityWarning
      SeverityError
  )

  func (s Severity) String() string {
      switch s {
      case SeverityInfo:
          return "INFO"
      case SeverityWarning:
          return "WARNING"
      case SeverityError:
          return "ERROR"
      default:
          return "UNKNOWN"
      }
  }

  // Report represents a single analysis finding
  type Report struct {
      Pos         token.Pos
      Message     string
      Severity    Severity
      Confidence  float64
      MatchType   string
      ResourceName string
      Suggestions []string
  }

  // DetermineSeverity calculates severity based on confidence
  func DetermineSeverity(confidence float64, hasIssue bool) Severity {
      if !hasIssue {
          return SeverityInfo
      }

      switch {
      case confidence >= 0.9:
          return SeverityError
      case confidence >= 0.7:
          return SeverityWarning
      default:
          return SeverityInfo
      }
  }

  // FormatReport formats a report for output
  func FormatReport(r Report) string {
      msg := fmt.Sprintf("[%s] %s", r.Severity, r.Message)

      if r.Confidence > 0 && r.Confidence < 1.0 {
          msg += fmt.Sprintf(" (%.0f%% confidence, %s match)", r.Confidence*100, r.MatchType)
      }

      if len(r.Suggestions) > 0 {
          msg += "\n  Suggestions:"
          for _, s := range r.Suggestions {
              msg += "\n    - " + s
          }
      }

      return msg
  }
  ```

- [ ] **4.2.2-4.2.5** (Implementation details in report.go above and integration in analyzers)

---

### Phase 5: Configuration and CLI Enhancements

#### 5.1 Enhanced Configuration Options

**Goal:** Add configuration for new matching strategies

- [ ] **5.1.1** Update `Settings` struct in `settings.go` (complete definition)

  **Implementation:**
  ```go
  // settings.go - COMPLETE REWRITE

  package tfprovidertest

  import (
      "fmt"
      "regexp"
  )

  type Settings struct {
      // Analyzer toggles
      EnableBasicTest       bool     `yaml:"enable-basic-test"`
      EnableUpdateTest      bool     `yaml:"enable-update-test"`
      EnableImportTest      bool     `yaml:"enable-import-test"`
      EnableErrorTest       bool     `yaml:"enable-error-test"`
      EnableStateCheck      bool     `yaml:"enable-state-check"`

      // Path patterns
      ResourcePathPattern   string   `yaml:"resource-path-pattern"`
      DataSourcePathPattern string   `yaml:"data-source-path-pattern"`
      TestFilePattern       string   `yaml:"test-file-pattern"`
      ExcludePaths          []string `yaml:"exclude-paths"`

      // File exclusions
      ExcludeBaseClasses    bool     `yaml:"exclude-base-classes"`
      ExcludeSweeperFiles   bool     `yaml:"exclude-sweeper-files"`
      ExcludeMigrationFiles bool     `yaml:"exclude-migration-files"`
      ExcludePatterns       []string `yaml:"exclude-patterns"`
      DiagnosticExclusions  bool     `yaml:"diagnostic-exclusions"`

      // Test detection
      TestNamePatterns      []string `yaml:"test-name-patterns"`
      CustomTestHelpers     []string `yaml:"custom-test-helpers"`
      IncludeHelperPatterns []string `yaml:"include-helper-patterns"`

      // Matching strategies
      EnableFunctionMatching bool    `yaml:"enable-function-matching"`
      EnableFileMatching     bool    `yaml:"enable-file-based-matching"`
      EnableFuzzyMatching    bool    `yaml:"enable-fuzzy-matching"`
      FuzzyMatchThreshold    float64 `yaml:"fuzzy-match-threshold"`

      // Provider configuration
      ProviderPrefix        string   `yaml:"provider-prefix"`
      ResourceNamingPattern string   `yaml:"resource-naming-pattern"`

      // Output options
      Verbose               bool     `yaml:"verbose"`
      ShowMatchConfidence   bool     `yaml:"show-match-confidence"`
      ShowUnmatchedTests    bool     `yaml:"show-unmatched-tests"`
      ShowOrphanedResources bool     `yaml:"show-orphaned-resources"`
  }

  func DefaultSettings() Settings {
      return Settings{
          EnableBasicTest:         true,
          EnableUpdateTest:        true,
          EnableImportTest:        true,
          EnableErrorTest:         true,
          EnableStateCheck:        true,
          ResourcePathPattern:     "resource_*.go",
          DataSourcePathPattern:   "data_source_*.go",
          TestFilePattern:         "*_test.go",
          ExcludePaths:            []string{},
          ExcludeBaseClasses:      true,
          ExcludeSweeperFiles:     true,
          ExcludeMigrationFiles:   true,
          ExcludePatterns:         []string{"*_sweeper.go", "*_test_helpers.go"},
          DiagnosticExclusions:    false,
          TestNamePatterns:        []string{},
          CustomTestHelpers:       []string{},
          IncludeHelperPatterns:   []string{"*Helper*", "*Wrapper*"},
          EnableFunctionMatching:  true,
          EnableFileMatching:      true,
          EnableFuzzyMatching:     false,
          FuzzyMatchThreshold:     0.7,
          ProviderPrefix:          "",
          ResourceNamingPattern:   "",
          Verbose:                 false,
          ShowMatchConfidence:     false,
          ShowUnmatchedTests:      false,
          ShowOrphanedResources:   true,
      }
  }

  // Validate validates the settings and returns any errors
  func (s *Settings) Validate() error {
      // Validate threshold range
      if s.FuzzyMatchThreshold < 0.0 || s.FuzzyMatchThreshold > 1.0 {
          return fmt.Errorf("fuzzy-match-threshold must be between 0.0 and 1.0, got %f", s.FuzzyMatchThreshold)
      }

      // Validate regex patterns
      if s.ResourceNamingPattern != "" {
          if _, err := regexp.Compile(s.ResourceNamingPattern); err != nil {
              return fmt.Errorf("invalid resource-naming-pattern: %w", err)
          }
      }

      // Warn on conflicting settings
      if !s.EnableFunctionMatching && !s.EnableFileMatching && !s.EnableFuzzyMatching {
          return fmt.Errorf("at least one matching strategy must be enabled")
      }

      return nil
  }
  ```

#### 5.2 CLI Diagnostic Commands

**Goal:** Add CLI flags for diagnostic output

- [ ] **5.2.1** Add CLI flags in `cmd/validate/main.go`

  **Implementation:**
  ```go
  // cmd/validate/main.go - COMPLETE REWRITE

  package main

  import (
      "encoding/json"
      "flag"
      "fmt"
      "go/ast"
      "go/parser"
      "go/token"
      "os"
      "path/filepath"
      "strings"
      "text/tabwriter"

      tfprovidertest "github.com/example/tfprovidertest"
      "golang.org/x/tools/go/analysis"
  )

  func main() {
      // Basic flags
      providerPath := flag.String("provider", "", "Path to the Terraform provider directory")
      verbose := flag.Bool("verbose", false, "Enable verbose output")

      // Diagnostic flags
      showMatches := flag.Bool("show-matches", false, "Show all resource → test function associations")
      showUnmatched := flag.Bool("show-unmatched", false, "Show test functions without resource association")
      showOrphaned := flag.Bool("show-orphaned", false, "Show resources without any test coverage")
      outputFormat := flag.String("format", "text", "Output format: text, json, or table")

      // Strategy flags
      matchStrategy := flag.String("match-strategy", "all", "Matching strategy: function, file, fuzzy, or all")
      confidenceThreshold := flag.Float64("confidence-threshold", 0.7, "Minimum confidence for matches (0.0-1.0)")

      // Provider-specific flags
      providerPrefix := flag.String("provider-prefix", "", "Provider prefix for function name matching (e.g., AWS, Google)")

      flag.Parse()

      if *providerPath == "" {
          printUsage()
          os.Exit(1)
      }

      providerCodeDir := findProviderCodeDir(*providerPath)
      if providerCodeDir == "" {
          fmt.Printf("Error: Could not find provider code directory in %s\n", *providerPath)
          os.Exit(1)
      }

      fmt.Printf("Analyzing provider at: %s\n\n", providerCodeDir)

      // Build settings from flags
      settings := tfprovidertest.DefaultSettings()
      settings.Verbose = *verbose
      settings.ShowMatchConfidence = *showMatches
      settings.ShowUnmatchedTests = *showUnmatched
      settings.ShowOrphanedResources = *showOrphaned
      settings.FuzzyMatchThreshold = *confidenceThreshold
      settings.ProviderPrefix = *providerPrefix

      // Configure matching strategy
      switch *matchStrategy {
      case "function":
          settings.EnableFunctionMatching = true
          settings.EnableFileMatching = false
          settings.EnableFuzzyMatching = false
      case "file":
          settings.EnableFunctionMatching = false
          settings.EnableFileMatching = true
          settings.EnableFuzzyMatching = false
      case "fuzzy":
          settings.EnableFunctionMatching = true
          settings.EnableFileMatching = true
          settings.EnableFuzzyMatching = true
      case "all":
          settings.EnableFunctionMatching = true
          settings.EnableFileMatching = true
          settings.EnableFuzzyMatching = false // Still disabled by default
      }

      // Validate settings
      if err := settings.Validate(); err != nil {
          fmt.Printf("Error: Invalid settings: %v\n", err)
          os.Exit(1)
      }

      // Parse provider
      fset := token.NewFileSet()
      pkgs, err := parser.ParseDir(fset, providerCodeDir, nil, parser.ParseComments)
      if err != nil {
          fmt.Printf("Error parsing provider directory: %v\n", err)
          os.Exit(1)
      }

      var allFiles []*ast.File
      for _, pkg := range pkgs {
          for _, file := range pkg.Files {
              allFiles = append(allFiles, file)
          }
      }

      // Handle diagnostic commands
      if *showMatches || *showUnmatched || *showOrphaned {
          runDiagnostics(fset, allFiles, settings, *outputFormat, *showMatches, *showUnmatched, *showOrphaned)
          return
      }

      // Run standard analysis
      runAnalyzers(fset, allFiles, settings)
  }

  func printUsage() {
      fmt.Println("Usage: validate -provider <path> [options]")
      fmt.Println()
      fmt.Println("Options:")
      fmt.Println("  -provider string")
      fmt.Println("        Path to the Terraform provider directory (required)")
      fmt.Println()
      fmt.Println("Diagnostic Options:")
      fmt.Println("  -show-matches")
      fmt.Println("        Show all resource → test function associations")
      fmt.Println("  -show-unmatched")
      fmt.Println("        Show test functions without resource association")
      fmt.Println("  -show-orphaned")
      fmt.Println("        Show resources without any test coverage")
      fmt.Println("  -format string")
      fmt.Println("        Output format: text, json, or table (default: text)")
      fmt.Println()
      fmt.Println("Matching Options:")
      fmt.Println("  -match-strategy string")
      fmt.Println("        Matching strategy: function, file, fuzzy, or all (default: all)")
      fmt.Println("  -confidence-threshold float")
      fmt.Println("        Minimum confidence for matches, 0.0-1.0 (default: 0.7)")
      fmt.Println("  -provider-prefix string")
      fmt.Println("        Provider prefix for function name matching (e.g., AWS, Google)")
      fmt.Println()
      fmt.Println("Output Options:")
      fmt.Println("  -verbose")
      fmt.Println("        Enable verbose diagnostic output")
      fmt.Println()
      fmt.Println("Examples:")
      fmt.Println("  validate -provider ./terraform-provider-aws")
      fmt.Println("  validate -provider ./provider -show-matches -format table")
      fmt.Println("  validate -provider ./provider -match-strategy function -confidence-threshold 0.8")
  }

  func runDiagnostics(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings, format string, showMatches, showUnmatched, showOrphaned bool) {
      // Build registry with settings
      // Note: This requires exposing BuildRegistry or creating a diagnostic-specific function

      if showMatches {
          fmt.Println("=== Resource → Test Function Associations ===")
          // Implementation depends on registry access
      }

      if showUnmatched {
          fmt.Println("\n=== Unmatched Test Functions ===")
          // Implementation depends on registry access
      }

      if showOrphaned {
          fmt.Println("\n=== Orphaned Resources (No Tests) ===")
          // Implementation depends on registry access
      }
  }

  // MatchInfo represents a resource-test association for diagnostic output
  type MatchInfo struct {
      ResourceName string  `json:"resource_name"`
      TestFunction string  `json:"test_function"`
      TestFile     string  `json:"test_file"`
      Confidence   float64 `json:"confidence"`
      MatchType    string  `json:"match_type"`
  }

  func outputMatchesText(matches []MatchInfo) {
      for _, m := range matches {
          fmt.Printf("  %s → %s (%.0f%%, %s)\n", m.ResourceName, m.TestFunction, m.Confidence*100, m.MatchType)
      }
  }

  func outputMatchesTable(matches []MatchInfo) {
      w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
      fmt.Fprintln(w, "RESOURCE\tTEST FUNCTION\tCONFIDENCE\tMATCH TYPE")
      fmt.Fprintln(w, "--------\t-------------\t----------\t----------")
      for _, m := range matches {
          fmt.Fprintf(w, "%s\t%s\t%.0f%%\t%s\n", m.ResourceName, m.TestFunction, m.Confidence*100, m.MatchType)
      }
      w.Flush()
  }

  func outputMatchesJSON(matches []MatchInfo) {
      enc := json.NewEncoder(os.Stdout)
      enc.SetIndent("", "  ")
      enc.Encode(matches)
  }

  func runAnalyzers(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings) {
      // Create plugin
      settingsMap := map[string]interface{}{
          "Verbose": settings.Verbose,
      }
      plugin, err := tfprovidertest.New(settingsMap)
      if err != nil {
          fmt.Printf("Error creating plugin: %v\n", err)
          os.Exit(1)
      }

      analyzers, err := plugin.BuildAnalyzers()
      if err != nil {
          fmt.Printf("Error building analyzers: %v\n", err)
          os.Exit(1)
      }

      totalIssues := 0
      for _, analyzer := range analyzers {
          fmt.Printf("Running %s...\n", analyzer.Name)

          pass := &analysis.Pass{
              Analyzer: analyzer,
              Fset:     fset,
              Files:    files,
              Report: func(diag analysis.Diagnostic) {
                  pos := fset.Position(diag.Pos)
                  fmt.Printf("\n[%s] %s:%d\n", analyzer.Name, pos.Filename, pos.Line)
                  fmt.Printf("  %s\n", diag.Message)
                  totalIssues++
              },
          }

          if _, err := analyzer.Run(pass); err != nil {
              fmt.Printf("  Error running analyzer: %v\n", err)
          }
      }

      fmt.Println()
      fmt.Println("=== Summary ===")
      if totalIssues == 0 {
          fmt.Println("✅ No issues found - all resources have proper test coverage!")
      } else {
          fmt.Printf("⚠️  Found %d issue(s)\n", totalIssues)
      }
  }

  func findProviderCodeDir(providerPath string) string {
      possiblePaths := []string{
          filepath.Join(providerPath, "internal", "provider"),
          filepath.Join(providerPath, "internal"),
          filepath.Join(providerPath, filepath.Base(providerPath)),
      }

      baseName := filepath.Base(providerPath)
      if strings.HasPrefix(baseName, "terraform-provider-") {
          shortName := strings.TrimPrefix(baseName, "terraform-provider-")
          possiblePaths = append(possiblePaths, filepath.Join(providerPath, shortName))
      }

      for _, path := range possiblePaths {
          if stat, err := os.Stat(path); err == nil && stat.IsDir() {
              return path
          }
      }

      return ""
  }
  ```

- [ ] **5.2.2-5.2.6** (Covered in implementation above)

---

### Phase 6: Testing and Validation

#### 6.1 Regression Testing

- [ ] **6.1.1** Run all existing unit tests
  ```bash
  go test ./... -v
  ```

- [ ] **6.1.2** Run integration tests against test fixtures
  ```bash
  go test ./... -v -run TestIntegration
  ```

- [ ] **6.1.3** Compare results with file-first mode
  ```bash
  # Run with old behavior
  ./validate -provider ./testdata/providers/standard -match-strategy file > file_results.txt

  # Run with new behavior
  ./validate -provider ./testdata/providers/standard -match-strategy function > function_results.txt

  # Compare
  diff file_results.txt function_results.txt
  ```

#### 6.2 Real-World Provider Testing

- [ ] **6.2.1** Test against AWS provider structure
- [ ] **6.2.2** Test against Google Cloud provider structure
- [ ] **6.2.3** Test against Azure provider structure
- [ ] **6.2.4** Test against composite file scenarios

---

### Phase 7: Documentation and Release

#### 7.1 Documentation Updates

- [ ] **7.1.1** Update AGENTS.md files
- [ ] **7.1.2** Add inline code documentation
- [ ] **7.1.3** Create migration guide

#### 7.2 Release Preparation

- [ ] **7.2.1** Update version number
- [ ] **7.2.2** Update CHANGELOG
- [ ] **7.2.3** Create release tag
- [ ] **7.2.4** Verify CI/CD passes

---

## Success Metrics

| Metric | Current | Target | How to Measure |
|--------|---------|--------|----------------|
| Resources correctly matched | ~60% | >95% | Run against AWS provider sample |
| False positive "untested" reports | High | <5% | Manual review of reports |
| Composite file handling | 0% | 100% | Test fixtures with multi-resource files |
| Non-standard naming support | Limited | Full | Test against Google/Azure providers |
| Update test false positives | High | <10% | Test fixtures with import/idempotency tests |

---

## Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| Function name parsing edge cases | Medium | Medium | Comprehensive regex testing, fallback to file matching |
| Performance with large codebases | Low | Medium | Lazy evaluation, caching, parallel processing |
| Breaking existing integrations | Low | High | Maintain backward compatibility, gradual rollout |
| Fuzzy matching false positives | Medium | Low | Configurable threshold, disabled by default |

---

## Appendix A: tfproviderlint Patterns Adopted

### Multi-Layer Analysis (from AT002)
```
testfuncdecl.Analyzer → testaccfuncdecl.Analyzer → AT002.Analyzer
```
We adopt this pattern: generic test detection → acceptance test filter → specific analysis.

### Suppression Comments (from tfproviderlint)
```go
//lintignore:AT002  →  //lint:ignore tfprovider-<check>
```

### Function Naming Convention (from HashiCorp)
```
TestAcc{Provider}{Resource}_{suffix}
Example: TestAccAWSInstance_basic
```

---

## Appendix B: Test Function Matching Patterns

### Primary Pattern (Regex)
```regex
^TestAcc(?P<provider>[A-Z][a-zA-Z0-9]*)(?P<resource>[A-Z][a-zA-Z0-9]*)_(?P<suffix>.+)$
```

### Examples
| Function Name | Provider | Resource | Suffix |
|--------------|----------|----------|--------|
| `TestAccAWSInstance_basic` | AWS | Instance | basic |
| `TestAccGoogleComputeInstance_update` | Google | ComputeInstance | update |
| `TestAccAzureRMVirtualMachine_disappears` | AzureRM | VirtualMachine | disappears |

### Fallback Patterns
1. `TestAcc<Resource>_*` (no provider prefix)
2. `Test<Resource>_*` (non-acceptance but resource-specific)
3. File-based: `resource_<name>_test.go`

---

## Appendix C: Proposed Data Model Changes

### Current (File-First)
```go
type TestFileInfo struct {
    FilePath      string
    ResourceName  string        // Forces 1:1 mapping
    TestFunctions []TestFunctionInfo
}

type ResourceRegistry struct {
    testFiles map[string]*TestFileInfo  // Keyed by resource name
}
```

### Proposed (Function-First)
```go
type TestFileInfo struct {
    FilePath      string
    PackageName   string
    TestFunctions []TestFunctionInfo
    // ResourceName removed
}

type TestFunctionInfo struct {
    Name              string
    FilePath          string              // NEW
    FunctionPos       token.Pos
    UsesResourceTest  bool
    TestSteps         []TestStepInfo
    HasErrorCase      bool
    HasImportStep     bool
    InferredResources []string            // NEW
    MatchConfidence   float64             // NEW
}

type ResourceRegistry struct {
    resources      map[string]*ResourceInfo
    dataSources    map[string]*ResourceInfo
    testFiles      map[string]*TestFileInfo      // Keyed by file path
    testFunctions  []*TestFunctionInfo           // NEW: Global index
    resourceTests  map[string][]*TestFunctionInfo // NEW: Resource → functions
}
```

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2025-12-07 09:53 UTC | Claude | Initial plan based on improvements.md feedback |
| 1.1 | 2025-12-07 09:58 UTC | Claude | Converted to detailed checklist format |
| 2.0 | 2025-12-07 10:30 UTC | Claude | Expanded with full implementation details, code examples, and line references |
