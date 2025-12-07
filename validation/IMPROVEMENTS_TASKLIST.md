# Linter Improvements Task List

**Based on**: Validation against 7 production Terraform providers
**Date**: 2025-12-07
**Total False Positives Identified**: ~780 across 7 providers

## Priority Ranking Criteria

Issues are ranked by:
1. **Impact**: Number of false positives eliminated
2. **Effort**: Implementation complexity
3. **Value**: Improvement to user experience

---

## Priority 1: Critical (High Impact, Low-Medium Effort)

### 1.1 Add Sweeper File Exclusion
**Impact**: Eliminates ~331 false positives (44% of Google Beta issues)
**Effort**: Low (simple filename pattern)
**Affected Providers**: google-beta

**Problem**: Files ending in `*_sweeper.go` are test infrastructure for cleaning up resources after acceptance tests. They are not production resources but are detected as resources.

**Solution**:
```go
// Add to Settings struct
ExcludeSweeperFiles bool `yaml:"exclude-sweeper-files"`

// Add to isExcludedFile function
func isSweeperFile(filePath string) bool {
    return strings.HasSuffix(filepath.Base(filePath), "_sweeper.go")
}
```

**Files to Modify**:
- `tfprovidertest.go`: Add setting and exclusion logic

---

### 1.2 Add Migration File Exclusion
**Impact**: Eliminates ~20-50 false positives
**Effort**: Low (simple filename pattern)
**Affected Providers**: google-beta

**Problem**: Files ending in `*_migrate.go` are state migration utilities, not production resources.

**Solution**:
```go
ExcludeMigrationFiles bool `yaml:"exclude-migration-files"`

func isMigrationFile(filePath string) bool {
    base := filepath.Base(filePath)
    return strings.HasSuffix(base, "_migrate.go") || strings.Contains(base, "_migration")
}
```

---

### 1.3 Support `TestDataSource_*` Pattern (HTTP Provider)
**Impact**: Eliminates 1 false positive, enables HTTP provider validation
**Effort**: Low (pattern already partially implemented)
**Affected Providers**: http

**Problem**: HTTP provider uses `TestDataSource_200` instead of `TestAccDataSourceHttp_basic`.

**Current Code** (line 62-66):
```go
var defaultTestPatterns = []string{
    "TestAcc",        // Standard HashiCorp pattern
    "TestResource",   // Alternative
    "TestDataSource", // Alternative - ALREADY EXISTS but not matching correctly
}
```

**Root Cause**: The pattern matching extracts resource name incorrectly. `TestDataSource_200` should match data source `http`.

**Solution**: Improve resource name extraction from test function names to handle abbreviated patterns.

---

## Priority 2: High (Medium Impact, Medium Effort)

### 2.1 Support `TestResource*` Pattern Without `Acc` (TLS Provider)
**Impact**: Eliminates 6 false positives
**Effort**: Medium
**Affected Providers**: tls

**Problem**: TLS provider uses patterns like:
- `TestPrivateKeyRSA`
- `TestResourceLocallySignedCert`

These don't match the expected `TestAcc*` convention.

**Solution**: Enhance `extractResourceNameFromTestFunc` to handle more patterns:
```go
func extractResourceNameFromTestFunc(funcName string) string {
    // Add pattern: TestPrivateKey* -> private_key
    // Add pattern: TestResourceLocallySignedCert -> locally_signed_cert

    // Remove "Test" prefix and try to match against known resource names
    name := strings.TrimPrefix(funcName, "Test")

    // Try splitting on uppercase to find resource name
    // TestPrivateKeyRSA -> ["Private", "Key", "RSA"] -> private_key
}
```

---

### 2.2 Add File-Based Test Matching Fallback
**Impact**: Reduces false positives for providers with non-standard naming
**Effort**: Medium
**Affected Providers**: tls, http, hcp

**Problem**: When test naming doesn't follow conventions but `resource_foo_test.go` exists with tests, assume coverage exists.

**Solution**:
```go
// If resource_foo.go exists and resource_foo_test.go exists with Test* functions,
// consider it covered even if naming convention doesn't match perfectly
func hasMatchingTestFile(resourceName string, testFiles map[string]*TestFileInfo) bool {
    expectedTestFile := "resource_" + resourceName + "_test.go"
    for filePath, testFile := range testFiles {
        if strings.HasSuffix(filePath, expectedTestFile) && len(testFile.TestFunctions) > 0 {
            return true
        }
    }
    return false
}
```

---

### 2.3 Detect and Categorize Skipped Tests (HCP Provider)
**Impact**: Provides actionable feedback for 11 HCP resources
**Effort**: Medium-High
**Affected Providers**: hcp

**Problem**: HCP tests exist but are conditionally skipped:
```go
if os.Getenv("HCP_WAYP_ACTION_TEST") == "" {
    t.Skipf("Waypoint Action tests skipped...")
}
```

**Solution**: Parse test function body for `t.Skip()` patterns:
```go
type TestFunctionInfo struct {
    // ... existing fields
    IsConditionallySkipped bool
    SkipReason             string
}

func detectSkippedTest(body *ast.BlockStmt) (bool, string) {
    // Look for t.Skip, t.Skipf, t.SkipNow patterns
    // Look for os.Getenv checks followed by skip
}
```

**Output Change**: Report as "test exists but is conditionally skipped" instead of "no test".

---

## Priority 3: Medium (Lower Impact, Variable Effort)

### 3.1 Configurable Test Name Patterns via YAML
**Impact**: Enables customization for any provider
**Effort**: Medium
**Affected Providers**: All

**Current Implementation** (partially exists):
```go
TestNamePatterns []string `yaml:"test-name-patterns"`
```

**Enhancement Needed**:
- Document configuration options
- Add example `.golangci.yml` configuration
- Support regex patterns, not just prefix matching

**Example Configuration**:
```yaml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        test-name-patterns:
          - "TestAcc*"
          - "TestResource*"
          - "TestDataSource*"
          - "Test*_basic"
          - "Test*_*"  # Fallback: any test with underscore
```

---

### 3.2 Support Provider-Prefixed Test Names (AAP Provider)
**Impact**: Eliminates 6-8 false positives
**Effort**: Low-Medium
**Affected Providers**: aap

**Problem**: AAP uses `TestAccAAPWorkflowJob_*` pattern (provider name in test).

**Solution**:
```go
// Auto-detect provider name from go.mod or directory name
func detectProviderName(pass *analysis.Pass) string {
    // Extract from terraform-provider-aap -> "aap"
}

// Match TestAccAAP* to aap_* resources
func matchProviderPrefixedTest(testName, providerName, resourceName string) bool {
    pattern := "TestAcc" + strings.ToUpper(providerName)
    // TestAccAAPWorkflowJob -> workflow_job for provider "aap"
}
```

---

### 3.3 Exclude Abstract Base Classes (AAP Provider)
**Impact**: Eliminates 2 false positives
**Effort**: Low (already implemented but needs refinement)
**Affected Providers**: aap

**Current Implementation**:
```go
ExcludeBaseClasses bool `yaml:"exclude-base-classes"`
```

**Enhancement**: Also exclude resources named `base` regardless of filename:
```go
func isAbstractResource(resourceName string) bool {
    return resourceName == "base" ||
           resourceName == "base_resource" ||
           resourceName == "abstract"
}
```

---

### 3.4 Support Legacy Directory Structures
**Impact**: Enables validation of more providers
**Effort**: Medium
**Affected Providers**: helm (already working), potential others

**Problem**: Some providers use `helm/` instead of `internal/provider/`.

**Current State**: The standalone validator handles this, but golangci-lint integration may not.

**Solution**: Add configurable resource discovery paths:
```go
ResourceDiscoveryPaths []string `yaml:"resource-discovery-paths"`
// Default: ["internal/provider", "internal", "<provider-name>"]
```

---

## Priority 4: Low (Nice-to-Have)

### 4.1 Pattern Auto-Detection
**Impact**: Reduces configuration needed
**Effort**: High
**Affected Providers**: All

**Concept**: Analyze first few test files to detect naming patterns used in the codebase, then apply those patterns.

```go
func detectTestPatterns(testFiles map[string]*TestFileInfo) []string {
    // Analyze test function names
    // Find common prefixes/patterns
    // Return detected patterns
}
```

---

### 4.2 Coverage Reporting Mode
**Impact**: Better visibility into test coverage
**Effort**: Medium
**Affected Providers**: All (especially large ones like google-beta)

**Concept**: Add `--report` mode that outputs coverage statistics instead of lint errors.

```
$ golangci-lint run --custom.tfprovidertest.report=true

Coverage Report:
- Resources: 646/1262 (51.2%)
- Data Sources: 160/290 (55.2%)
- Untested: 745 items
```

---

### 4.3 Categorized Output
**Impact**: Better actionable insights
**Effort**: Medium

**Concept**: Group issues by category:
- Missing tests (true gaps)
- Naming convention mismatches (potential false positives)
- Conditionally skipped tests (environment-gated)
- Utility files (sweepers, migrations)

---

## Priority 1.5: Critical UX (High Impact, Low Effort)

### 1.5.1 Enhanced Issue Location Reporting
**Impact**: Significantly improves actionability of issues
**Effort**: Low
**Affected Providers**: All

**Problem**: Current output doesn't provide enough context for users to locate and fix issues:
```
resource 'http' has no acceptance test file
```

**User Feedback**: "Increase visibility of where the issue was found so I know where it can be fixed"

**Solution**: Enhance diagnostic output to include:
1. **Full file path** to the resource definition
2. **Line number** where the resource Schema() is defined
3. **Expected test file location** to help users create the missing test
4. **Expected test function name** pattern

**Current Output**:
```
resource 'http' has no acceptance test file
```

**Improved Output**:
```
tfprovider-resource-basic-test: resource 'http' has no acceptance test

  Resource Location:
    File: /workspace/validation/terraform-provider-http/internal/provider/data_source_http.go
    Line: 45 (Schema method)

  Expected Test:
    File: /workspace/validation/terraform-provider-http/internal/provider/data_source_http_test.go
    Function: TestAccDataSourceHttp_basic or TestDataSource_http_basic

  Suggested Fix:
    Create a test file with a basic acceptance test that validates the resource
    can be created and destroyed successfully.
```

**Implementation**:
```go
// Enhance the Issue struct
type EnhancedDiagnostic struct {
    Analyzer         string
    ResourceName     string
    ResourceType     string // "resource" or "data source"
    ResourceFile     string
    ResourceLine     int
    ExpectedTestFile string
    ExpectedTestFunc string
    SuggestedFix     string
}

// Update runBasicTestAnalyzer
func runBasicTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
    // ...
    for _, resource := range untested {
        resourceType := "resource"
        if resource.IsDataSource {
            resourceType = "data source"
        }

        // Get position info
        pos := pass.Fset.Position(resource.SchemaPos)

        // Build expected test file path
        expectedTestFile := buildExpectedTestPath(resource)
        expectedTestFunc := buildExpectedTestFunc(resource)

        // Enhanced message with location context
        msg := fmt.Sprintf(
            "%s '%s' has no acceptance test\n"+
            "  Resource: %s:%d\n"+
            "  Expected test file: %s\n"+
            "  Expected test function: %s",
            resourceType,
            resource.Name,
            pos.Filename,
            pos.Line,
            expectedTestFile,
            expectedTestFunc,
        )

        pass.Reportf(resource.SchemaPos, msg)
    }
}

func buildExpectedTestPath(resource *ResourceInfo) string {
    dir := filepath.Dir(resource.FilePath)
    if resource.IsDataSource {
        return filepath.Join(dir, "data_source_"+resource.Name+"_test.go")
    }
    return filepath.Join(dir, "resource_"+resource.Name+"_test.go")
}

func buildExpectedTestFunc(resource *ResourceInfo) string {
    titleName := toTitleCase(resource.Name)
    if resource.IsDataSource {
        return fmt.Sprintf("TestAccDataSource%s_basic", titleName)
    }
    return fmt.Sprintf("TestAccResource%s_basic", titleName)
}
```

**Files to Modify**:
- `tfprovidertest.go`: Update all `pass.Reportf` calls in analyzer functions

---

### 1.5.2 Add `--verbose` Mode for Detailed Diagnostics
**Impact**: Helps users understand why an issue was flagged
**Effort**: Low-Medium

**Problem**: Users don't understand why a resource was flagged when tests exist.

**Solution**: Add verbose mode that shows:
- What test files were found
- What test functions were found
- Why they didn't match the resource

**Example Verbose Output**:
```
tfprovider-resource-basic-test: resource 'private_key' has no acceptance test

  Resource Location:
    File: internal/provider/resource_private_key.go:89

  Test Files Searched:
    - internal/provider/resource_private_key_test.go (found)

  Test Functions Found:
    - TestPrivateKeyRSA (line 45) - NOT MATCHED (missing 'Acc' prefix)
    - TestPrivateKeyECDSA (line 89) - NOT MATCHED (missing 'Acc' prefix)
    - TestPrivateKeyED25519 (line 123) - NOT MATCHED (missing 'Acc' prefix)

  Why Not Matched:
    Expected pattern: TestAccResource* or TestAcc*PrivateKey*
    Found pattern: TestPrivateKey* (non-standard)

  Suggested Fix:
    Option 1: Rename tests to follow convention (TestAccResourcePrivateKey_RSA)
    Option 2: Configure custom test patterns in .golangci.yml:
      test-name-patterns:
        - "TestPrivateKey"
```

---

### 1.5.3 JSON Output Format for CI/CD Integration
**Impact**: Enables programmatic processing of results
**Effort**: Medium

**Concept**: Output structured JSON for integration with CI pipelines:

```json
{
  "provider": "terraform-provider-tls",
  "timestamp": "2025-12-07T06:00:00Z",
  "summary": {
    "resources_found": 6,
    "data_sources_found": 2,
    "issues_found": 6,
    "coverage_percent": 25.0
  },
  "issues": [
    {
      "analyzer": "tfprovider-resource-basic-test",
      "severity": "warning",
      "resource_name": "private_key",
      "resource_type": "resource",
      "location": {
        "file": "internal/provider/resource_private_key.go",
        "line": 89,
        "column": 1
      },
      "expected_test": {
        "file": "internal/provider/resource_private_key_test.go",
        "function": "TestAccResourcePrivateKey_basic"
      },
      "existing_tests": [
        {"name": "TestPrivateKeyRSA", "line": 45, "match_reason": "missing Acc prefix"}
      ],
      "message": "resource 'private_key' has no acceptance test"
    }
  ]
}
```

---

## Implementation Roadmap

### Phase 1: Quick Wins & UX (1-2 days)
1. [x] Base class exclusion (already implemented)
2. [ ] **Enhanced issue location reporting (Priority 1.5.1)** - HIGH PRIORITY
3. [ ] Sweeper file exclusion (Priority 1.1)
4. [ ] Migration file exclusion (Priority 1.2)

### Phase 2: Pattern Improvements (3-5 days)
5. [ ] Fix TestDataSource_* matching (Priority 1.3)
6. [ ] Support TestResource* without Acc (Priority 2.1)
7. [ ] File-based test matching fallback (Priority 2.2)
8. [ ] Verbose mode for diagnostics (Priority 1.5.2)

### Phase 3: Advanced Features (1-2 weeks)
9. [ ] Detect conditionally skipped tests (Priority 2.3)
10. [ ] Configurable patterns via YAML (Priority 3.1)
11. [ ] Provider-prefixed test names (Priority 3.2)
12. [ ] JSON output format (Priority 1.5.3)

### Phase 4: Polish (ongoing)
13. [ ] Pattern auto-detection (Priority 4.1)
14. [ ] Coverage reporting mode (Priority 4.2)
15. [ ] Categorized output (Priority 4.3)

---

## Success Metrics

After implementing Phase 1 & 2:
- **google-beta**: ~400 issues (down from 745) - 46% reduction
- **http**: 0 issues (down from 1) - 100% reduction
- **tls**: 0 issues (down from 6) - 100% reduction
- **hcp**: 0 issues with "skipped" categorization (down from 11)
- **aap**: 6 issues (down from 8) - 25% reduction

**Overall**: ~50% reduction in false positives across all providers.

---

## Configuration Example

After implementing all improvements:

```yaml
# .golangci.yml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # Analyzer toggles
        enable-basic-test: true
        enable-update-test: true
        enable-import-test: true
        enable-error-test: true
        enable-state-check: true

        # File exclusions (all default to true)
        exclude-base-classes: true
        exclude-sweeper-files: true
        exclude-migration-files: true

        # Test naming patterns (defaults shown)
        test-name-patterns:
          - "TestAcc"
          - "TestResource"
          - "TestDataSource"
          - "Test*_"

        # Enable file-based fallback matching
        enable-file-based-matching: true

        # Detect and categorize skipped tests
        detect-skipped-tests: true

        # Custom paths for non-standard providers
        resource-discovery-paths:
          - "internal/provider"
          - "internal"
```
