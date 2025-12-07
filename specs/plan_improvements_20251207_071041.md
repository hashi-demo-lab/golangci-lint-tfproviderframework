# Linter Improvements Plan

**Date**: 2025-12-07 07:10:41 UTC
**Author**: Claude Code
**Status**: Draft

---

## Executive Summary

This document outlines planned improvements to the tfprovider-test-linter to:
1. Extend test coverage detection using configurable test patterns
2. Enhance issue reports with file and line number information
3. Add SDKv2 legacy detection with migration warnings

---

## Current Analyzers

The linter includes 5 analyzers that can be individually enabled/disabled:

| Analyzer | Setting | What It Tests |
|----------|---------|---------------|
| **BasicTestAnalyzer** | `enable-basic-test: true` | Checks that every resource and data source has at least one acceptance test (`TestAcc*` function) |
| **UpdateTestAnalyzer** | `enable-update-test: true` | Checks that resources with updatable attributes (Optional, no RequiresReplace) have multi-step update tests (2+ test steps) |
| **ImportTestAnalyzer** | `enable-import-test: true` | Checks that resources implementing `ImportState` method have import tests (`ImportState: true` in test steps) |
| **ErrorTestAnalyzer** | `enable-error-test: true` | Checks that resources with validation rules (Required, Validators) have error case tests (`ExpectError` in test steps) |
| **StateCheckAnalyzer** | `enable-state-check: true` | Checks that test steps include state validation check functions (`Check` field with `TestCheckResourceAttr`, etc.) |

### Analyzer Details

#### 1. BasicTestAnalyzer (`tfprovider-resource-basic-test`)
**Purpose**: Ensure every resource/data source has basic acceptance test coverage.

**What it checks**:
- Resource has a corresponding `*_test.go` file
- Test file contains at least one `TestAcc*` function
- Uses `resource.Test()` or equivalent

**Detection logic**:
```go
// Reports if:
// 1. No test file found for resource
// 2. Test file exists but has no TestAcc* functions
```

#### 2. UpdateTestAnalyzer (`tfprovider-resource-update-test`)
**Purpose**: Ensure resources with mutable attributes have update tests.

**What it checks**:
- Resource has `Optional: true` attributes without `RequiresReplace` plan modifier
- Test has multiple steps (TestSteps >= 2) to verify update works

**Detection logic**:
```go
// Attribute needs update test if:
attr.Optional && attr.IsUpdatable  // IsUpdatable = no RequiresReplace

// Reports if resource has updatable attributes but no multi-step test
```

#### 3. ImportTestAnalyzer (`tfprovider-resource-import-test`)
**Purpose**: Ensure resources supporting import have import tests.

**What it checks**:
- Resource implements `ImportState` method
- Test has step with `ImportState: true` and `ImportStateVerify: true`

**Detection logic**:
```go
// Resource needs import test if:
resource.HasImportState == true

// Valid import test step has:
step.ImportState == true && step.ImportStateVerify == true
```

#### 4. ErrorTestAnalyzer (`tfprovider-test-error-cases`)
**Purpose**: Ensure validation rules are tested with error cases.

**What it checks**:
- Resource has `Required: true` attributes or `Validators` field
- Test has step with `ExpectError` field

**Detection logic**:
```go
// Attribute needs error test if:
attr.Required || attr.HasValidators

// Valid error test step has:
step.ExpectError == true
```

#### 5. StateCheckAnalyzer (`tfprovider-test-check-functions`)
**Purpose**: Ensure test steps validate state after apply.

**What it checks**:
- Each non-import, non-error test step has `Check` field
- Check field uses proper state check functions (`TestCheckResourceAttr`, etc.)

**Detection logic**:
```go
// Step needs Check if:
!step.ImportState && !step.ExpectError && step.HasConfig

// Reports if step is missing Check field
```

---

## Current State

### What Works
- Basic test coverage detection for resources and data sources
- File-based matching for non-standard naming conventions
- Sweeper, migration, and base class file exclusion
- Multiple test naming patterns (TestAcc*, TestResource*, TestDataSource*)

### Current Output Format
```
1. [tfprovider-resource-basic-test] Resource 'widget' has no acceptance test
   File: /path/to/resource_widget.go
```

### Gaps Identified
1. **No line numbers** in issue reports
2. **No test pattern coverage** analysis (which patterns matched, which didn't)
3. **No SDKv2 detection** or migration warnings
4. **Limited actionability** - users don't know exactly where to look

---

## Planned Improvements

### 1. Enhanced Issue Reporting with Line Numbers

**Goal**: Every issue report includes precise file and line number information.

**Current Output**:
```
Resource 'vault_secrets_integration_azure_deprecated' has no acceptance test
   File: /workspace/.../resource_vault_secrets_integration_azure_deprecated.go
```

**Proposed Output**:
```
Resource 'vault_secrets_integration_azure_deprecated' has no acceptance test
   Location: /workspace/.../resource_vault_secrets_integration_azure_deprecated.go:45
   Schema defined at: line 45-120
   Expected test file: resource_vault_secrets_integration_azure_deprecated_test.go
   Expected test function: TestAccVaultSecretsIntegrationAzureDeprecated_basic
```

**Implementation**:
```go
type EnhancedIssue struct {
    Analyzer         string
    Severity         string  // "error", "warning", "info"
    ResourceName     string
    ResourceType     string  // "resource", "data_source", "ephemeral"
    FilePath         string
    LineNumber       int     // Line where Schema() is defined
    EndLine          int     // End of Schema() method
    ExpectedTestFile string
    ExpectedTestFunc string
    MatchedPatterns  []string // Patterns that were tried
    SuggestedFix     string
}
```

**Files to Modify**:
- `tfprovidertest.go`: Update `runBasicTestAnalyzer()` to include line numbers
- `cmd/validate/main.go`: Add line number extraction

---

### 2. Test Pattern Coverage Analysis

**Goal**: Report which test patterns were tried and which matched/failed.

**Proposed Output**:
```
Resource 'private_key' - Test Pattern Analysis:
   Patterns Tried:
     ✓ TestAcc* - No match
     ✓ TestResource* - No match
     ✓ TestDataSource* - No match
     ✓ Test*_ - Matched: TestPrivateKeyRSA, TestPrivateKeyECDSA

   File-Based Match:
     ✓ resource_private_key_test.go exists with 3 test functions

   Verdict: COVERED (via file-based matching)
```

**Implementation**:
```go
type PatternMatchResult struct {
    Pattern      string
    Matched      bool
    MatchedFuncs []string
    Reason       string
}

type TestCoverageAnalysis struct {
    ResourceName     string
    PatternResults   []PatternMatchResult
    FileBasedMatch   bool
    TestFilePath     string
    TestFunctions    []string
    CoverageStatus   string // "covered", "partial", "missing"
}

func analyzeTestCoverage(resource *ResourceInfo, registry *ResourceRegistry) TestCoverageAnalysis {
    // Try each pattern and record results
    // Check file-based matching
    // Return comprehensive analysis
}
```

**New Settings**:
```yaml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # Report pattern matching details
        report-pattern-matching: true

        # Custom patterns to try (in order)
        test-name-patterns:
          - "TestAcc"
          - "TestResource"
          - "TestDataSource"
          - "Test*_"
```

---

### 3. SDKv2 Legacy Detection and Migration Warnings

**Goal**: Detect resources using deprecated SDKv2 and warn about migration to Plugin Framework.

#### Detection Strategy

**SDKv2 Indicators** (AST patterns to detect):

| Pattern | Detection Method |
|---------|------------------|
| Import `terraform-plugin-sdk/v2/helper/schema` | Check import declarations |
| Function returning `*schema.Resource` | Check function return types |
| Parameter `*schema.ResourceData` | Check function parameters |
| `d.Get()`, `d.Set()` calls | Check method calls in function bodies |
| `schema.TypeString`, `schema.TypeSet` | Check composite literal types |

**Plugin Framework Indicators**:

| Pattern | Detection Method |
|---------|------------------|
| Import `terraform-plugin-framework/*` | Check import declarations |
| Type implementing `resource.Resource` | Check interface implementations |
| Methods with `req resource.XxxRequest` | Check method signatures |
| `schema.StringAttribute{}` | Check composite literal types |

#### Implementation

```go
type SDKInfo struct {
    SDKVersion    string // "sdkv2", "framework", "mixed", "unknown"
    SDKImports    []string
    IsDeprecated  bool
    MigrationNote string
}

// Analyzer for SDK detection
var SDKDetectionAnalyzer = &analysis.Analyzer{
    Name: "tfprovider-sdk-detection",
    Doc:  "Detects SDKv2 usage and recommends migration to Plugin Framework",
    Run:  runSDKDetectionAnalyzer,
}

func runSDKDetectionAnalyzer(pass *analysis.Pass) (interface{}, error) {
    for _, file := range pass.Files {
        sdkInfo := detectSDKVersion(file)

        if sdkInfo.SDKVersion == "sdkv2" {
            pass.Reportf(file.Pos(),
                "⚠️ SDKv2 Detected: This file uses terraform-plugin-sdk/v2 which is in maintenance mode.\n"+
                "   Consider migrating to terraform-plugin-framework for:\n"+
                "   - Better type safety\n"+
                "   - Improved validation support\n"+
                "   - Native support for unknown values\n"+
                "   Migration guide: https://developer.hashicorp.com/terraform/plugin/framework/migrating")
        }
    }
    return nil, nil
}

func detectSDKVersion(file *ast.File) SDKInfo {
    info := SDKInfo{SDKVersion: "unknown"}

    // Check imports
    for _, imp := range file.Imports {
        path := strings.Trim(imp.Path.Value, `"`)

        if strings.Contains(path, "terraform-plugin-sdk/v2") {
            info.SDKVersion = "sdkv2"
            info.SDKImports = append(info.SDKImports, path)
            info.IsDeprecated = true
            info.MigrationNote = "SDKv2 is in maintenance mode. Migrate to Plugin Framework."
        }

        if strings.Contains(path, "terraform-plugin-framework") {
            if info.SDKVersion == "sdkv2" {
                info.SDKVersion = "mixed"
            } else {
                info.SDKVersion = "framework"
            }
            info.SDKImports = append(info.SDKImports, path)
        }
    }

    return info
}
```

#### Proposed Output

```
=== SDK Analysis for terraform-provider-hcp ===

Files using SDKv2 (deprecated): 12
Files using Plugin Framework: 46
Mixed files (both SDKs): 3

⚠️ SDKv2 Files Requiring Migration:

1. internal/provider/vaultradar/integration_connection.go
   SDK: terraform-plugin-sdk/v2/helper/schema
   Patterns detected:
     - Function returns *schema.Resource
     - Uses d.Get(), d.Set() methods
     - Schema uses schema.TypeString

   Migration Priority: HIGH
   Reason: Active development, should migrate for better maintainability

2. internal/provider/vaultradar/secret_manager.go
   SDK: terraform-plugin-sdk/v2/helper/schema
   ...

📚 Migration Resources:
   - https://developer.hashicorp.com/terraform/plugin/framework/migrating
   - https://developer.hashicorp.com/terraform/plugin/framework-benefits
```

#### New Settings

```yaml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # SDK detection
        enable-sdk-detection: true
        warn-sdkv2-usage: true
        sdk-detection-severity: "warning"  # "error", "warning", "info"

        # Exclude patterns for SDK detection
        sdk-detection-exclude:
          - "**/generated/**"
          - "**/vendor/**"
```

---

## Implementation Phases

### Phase 1: Enhanced Line Number Reporting (Low Effort)

**Tasks**:
1. Update `ResourceInfo` struct to store `SchemaStartLine` and `SchemaEndLine`
2. Modify `parseResources()` to extract line numbers from AST positions
3. Update all `pass.Reportf()` calls to include line numbers
4. Update standalone validator to include line numbers

**Estimated Effort**: 2-4 hours

**Files to Modify**:
- `tfprovidertest.go`: Lines 298-308 (ResourceInfo), Lines 571-623 (parseResources)
- `cmd/validate/main.go`: Add line number extraction

---

### Phase 2: Test Pattern Coverage Analysis (Medium Effort)

**Tasks**:
1. Create `PatternMatchResult` and `TestCoverageAnalysis` structs
2. Implement `analyzeTestCoverage()` function
3. Add `report-pattern-matching` setting
4. Update output format to include pattern analysis
5. Add tests for pattern matching logic

**Estimated Effort**: 4-6 hours

**New Functions**:
```go
func analyzeTestCoverage(resource *ResourceInfo, registry *ResourceRegistry, patterns []string) TestCoverageAnalysis
func formatPatternAnalysis(analysis TestCoverageAnalysis) string
```

---

### Phase 3: SDKv2 Detection and Warnings (Medium-High Effort)

**Tasks**:
1. Implement `detectSDKVersion()` function
2. Create `SDKDetectionAnalyzer`
3. Add SDKv2-specific pattern detection (function signatures, method calls)
4. Implement migration severity levels
5. Add configuration options
6. Create comprehensive tests

**Estimated Effort**: 6-10 hours

**New Analyzer**:
```go
var SDKDetectionAnalyzer = &analysis.Analyzer{
    Name: "tfprovider-sdk-detection",
    Doc:  "Detects SDKv2 usage and recommends migration",
    Run:  runSDKDetectionAnalyzer,
}
```

---

## SDKv2 vs Plugin Framework Detection Patterns

### Import-Based Detection

```go
// SDKv2 imports
"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

// Plugin Framework imports
"github.com/hashicorp/terraform-plugin-framework/resource"
"github.com/hashicorp/terraform-plugin-framework/datasource"
"github.com/hashicorp/terraform-plugin-framework/types"
"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
```

### Function Signature Detection

| SDK | Pattern | Example |
|-----|---------|---------|
| SDKv2 | Returns `*schema.Resource` | `func ResourceWidget() *schema.Resource` |
| SDKv2 | Parameter `*schema.ResourceData` | `func(d *schema.ResourceData, meta interface{})` |
| SDKv2 | Uses `schema.TypeString` | `Type: schema.TypeString` |
| Framework | Implements `resource.Resource` | `type widgetResource struct{}` |
| Framework | Method `Schema(ctx, req, resp)` | `func (r *widgetResource) Schema(...)` |
| Framework | Uses `schema.StringAttribute` | `schema.StringAttribute{Required: true}` |

### AST Detection Algorithm

```go
func detectSDKFromFile(file *ast.File) string {
    hasSDKv2Import := false
    hasFrameworkImport := false

    // 1. Check imports
    for _, imp := range file.Imports {
        path := strings.Trim(imp.Path.Value, `"`)
        if strings.Contains(path, "terraform-plugin-sdk/v2") {
            hasSDKv2Import = true
        }
        if strings.Contains(path, "terraform-plugin-framework") {
            hasFrameworkImport = true
        }
    }

    // 2. If both imports present, check method patterns
    if hasSDKv2Import && hasFrameworkImport {
        // Check for receiver methods (Framework) vs functions (SDKv2)
        return "mixed"
    }

    if hasSDKv2Import {
        return "sdkv2"
    }

    if hasFrameworkImport {
        return "framework"
    }

    return "unknown"
}
```

---

## Success Criteria

### Phase 1: Line Numbers
- [ ] All issue reports include file path and line number
- [ ] Line numbers point to Schema() method definition
- [ ] Output format: `file.go:45` (compatible with IDE click-to-navigate)

### Phase 2: Pattern Analysis
- [ ] Report shows all patterns tried
- [ ] Report shows which patterns matched
- [ ] Report shows test functions that matched each pattern
- [ ] File-based matching clearly indicated

### Phase 3: SDKv2 Detection
- [ ] Correctly identifies SDKv2-only files
- [ ] Correctly identifies Plugin Framework files
- [ ] Correctly identifies mixed files
- [ ] Provides actionable migration guidance
- [ ] Configurable severity levels
- [ ] Zero false positives on Plugin Framework files

---

## Configuration Example

After all improvements:

```yaml
# .golangci.yml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # Existing analyzers
        enable-basic-test: true
        enable-update-test: true
        enable-import-test: true
        enable-error-test: true
        enable-state-check: true

        # File exclusions
        exclude-base-classes: true
        exclude-sweeper-files: true
        exclude-migration-files: true

        # Test patterns (in order of priority)
        test-name-patterns:
          - "TestAcc"
          - "TestResource"
          - "TestDataSource"
          - "Test*_"

        # New: Pattern matching analysis
        enable-file-based-matching: true
        report-pattern-matching: true

        # New: SDK detection
        enable-sdk-detection: true
        warn-sdkv2-usage: true
        sdk-detection-severity: "warning"

        # New: Enhanced output
        include-line-numbers: true
        verbose: false
```

---

## Sample Enhanced Output

```
=== Validation Results for terraform-provider-hcp ===
Time: 51.144208ms

Resources found: 46
Data sources found: 22
Test files found: 68

=== SDK Analysis ===
Plugin Framework: 43 files
SDKv2 (deprecated): 3 files
  ⚠️ internal/provider/vaultradar/integration_connection.go:1
  ⚠️ internal/provider/vaultradar/secret_manager.go:1
  ⚠️ internal/provider/vaultradar/radar_source.go:1

=== Test Coverage Issues ===

1. [tfprovider-resource-basic-test] Resource 'integration_connection' has no acceptance test
   Location: internal/provider/vaultradar/integration_connection.go:45
   Schema: lines 45-120

   Patterns Tried:
     ✗ TestAccIntegrationConnection* - No match
     ✗ TestResourceIntegrationConnection* - No match
     ✗ File-based: integration_connection_test.go - Not found

   Expected:
     File: integration_connection_test.go
     Function: TestAccIntegrationConnection_basic

   SDK: SDKv2 (consider migrating to Plugin Framework)

=== Summary ===
Resources with tests: 35/46 (76%)
Data sources with tests: 16/22 (73%)
SDKv2 files needing migration: 3
```

---

---

## Phase 4: Go-Native Test Detection (Replace Filename-Based Detection)

**Goal**: Move away from filename-based detection (`*_test.go`) to proper Go-native test detection using function signatures.

### Current Problem

The current implementation relies on filename patterns:
```go
// Current approach - filename based
if strings.HasSuffix(filePath, "_test.go") {
    // Parse as test file
}
if strings.HasPrefix(baseName, "resource_") {
    // Assume it's a resource
}
```

This is fragile and doesn't follow Go conventions.

### Go-Native Test Detection

Go identifies tests by **function signature**, not filename. The proper approach:

#### Test Function Signatures

| Type | Signature | Prefix |
|------|-----------|--------|
| Test | `func TestXxx(t *testing.T)` | `Test` |
| Benchmark | `func BenchmarkXxx(b *testing.B)` | `Benchmark` |
| Example | `func ExampleXxx()` | `Example` |
| Fuzz | `func FuzzXxx(f *testing.F)` | `Fuzz` |

#### AST-Based Detection Pattern

```go
import (
    "go/ast"
    "go/types"
)

// Detect test function by signature, not filename
func isTestFunc(fn *ast.FuncDecl, info *types.Info) bool {
    name := fn.Name.Name

    // Must start with "Test" and next char must not be lowercase
    if !strings.HasPrefix(name, "Test") {
        return false
    }
    if len(name) > 4 && unicode.IsLower(rune(name[4])) {
        return false // "Testfoo" is invalid, "TestFoo" or "Test_foo" is valid
    }

    // Must not be a method (no receiver)
    if fn.Recv != nil {
        return false
    }

    // Must have exactly one parameter
    params := fn.Type.Params
    if params == nil || len(params.List) != 1 {
        return false
    }

    // Parameter must be *testing.T
    return isTestingT(params.List[0], info)
}

// Check if parameter type is *testing.T
func isTestingT(field *ast.Field, info *types.Info) bool {
    // Use type information for accurate detection
    if info != nil {
        typ := info.TypeOf(field.Type)
        if ptr, ok := typ.(*types.Pointer); ok {
            if named, ok := ptr.Elem().(*types.Named); ok {
                obj := named.Obj()
                return obj.Pkg().Path() == "testing" && obj.Name() == "T"
            }
        }
    }

    // Fallback: AST-based detection
    starExpr, ok := field.Type.(*ast.StarExpr)
    if !ok {
        return false
    }

    selExpr, ok := starExpr.X.(*ast.SelectorExpr)
    if !ok {
        return false
    }

    ident, ok := selExpr.X.(*ast.Ident)
    if !ok {
        return false
    }

    return ident.Name == "testing" && selExpr.Sel.Name == "T"
}
```

#### Detect Acceptance Test Functions

For Terraform providers, we need to detect acceptance tests specifically:

```go
// Terraform acceptance test detection
func isAcceptanceTest(fn *ast.FuncDecl, info *types.Info) bool {
    if !isTestFunc(fn, info) {
        return false
    }

    // Check function body for resource.Test() or resource.UnitTest() calls
    return hasResourceTestCall(fn.Body)
}

func hasResourceTestCall(body *ast.BlockStmt) bool {
    if body == nil {
        return false
    }

    found := false
    ast.Inspect(body, func(n ast.Node) bool {
        call, ok := n.(*ast.CallExpr)
        if !ok {
            return true
        }

        sel, ok := call.Fun.(*ast.SelectorExpr)
        if !ok {
            return true
        }

        ident, ok := sel.X.(*ast.Ident)
        if !ok {
            return true
        }

        // Look for resource.Test, resource.UnitTest, resource.ParallelTest
        if ident.Name == "resource" {
            switch sel.Sel.Name {
            case "Test", "UnitTest", "ParallelTest":
                found = true
                return false
            }
        }

        // Also check for acctest.VcrTest (SDKv2 pattern)
        if ident.Name == "acctest" && sel.Sel.Name == "VcrTest" {
            found = true
            return false
        }

        return true
    })

    return found
}
```

### SDKv2 vs Plugin Framework Test Detection

#### SDKv2 Test Patterns (Legacy)

```go
func detectSDKv2Test(fn *ast.FuncDecl) bool {
    if fn.Body == nil {
        return false
    }

    hasSDKv2Pattern := false
    ast.Inspect(fn.Body, func(n ast.Node) bool {
        call, ok := n.(*ast.CallExpr)
        if !ok {
            return true
        }

        // Pattern 1: acctest.VcrTest(t, resource.TestCase{...})
        if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
            if ident, ok := sel.X.(*ast.Ident); ok {
                if ident.Name == "acctest" && sel.Sel.Name == "VcrTest" {
                    hasSDKv2Pattern = true
                    return false
                }
            }
        }

        // Pattern 2: resource.ComposeTestCheckFunc(...)
        if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
            if ident, ok := sel.X.(*ast.Ident); ok {
                if ident.Name == "resource" && sel.Sel.Name == "ComposeTestCheckFunc" {
                    hasSDKv2Pattern = true
                    return false
                }
            }
        }

        return true
    })

    return hasSDKv2Pattern
}
```

**SDKv2 Test Indicators**:
- `acctest.VcrTest()` wrapper
- `resource.ComposeTestCheckFunc()` for check composition
- `resource.TestCheckResourceAttr()` style checks
- Custom `testAccXxxExists()` check functions
- `CheckDestroy` with custom destroy verification function

#### Plugin Framework Test Patterns

```go
func detectPluginFrameworkTest(fn *ast.FuncDecl) bool {
    if fn.Body == nil {
        return false
    }

    hasFrameworkPattern := false
    ast.Inspect(fn.Body, func(n ast.Node) bool {
        call, ok := n.(*ast.CallExpr)
        if !ok {
            return true
        }

        // Pattern 1: resource.UnitTest(t, resource.TestCase{...})
        if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
            if ident, ok := sel.X.(*ast.Ident); ok {
                if ident.Name == "resource" && sel.Sel.Name == "UnitTest" {
                    hasFrameworkPattern = true
                    return false
                }
            }
        }

        // Pattern 2: statecheck.ExpectKnownValue(...)
        if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
            if ident, ok := sel.X.(*ast.Ident); ok {
                if ident.Name == "statecheck" {
                    hasFrameworkPattern = true
                    return false
                }
            }
        }

        // Pattern 3: plancheck.ExpectKnownValue(...)
        if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
            if ident, ok := sel.X.(*ast.Ident); ok {
                if ident.Name == "plancheck" {
                    hasFrameworkPattern = true
                    return false
                }
            }
        }

        return true
    })

    return hasFrameworkPattern
}
```

**Plugin Framework Test Indicators**:
- `resource.UnitTest()` or `resource.Test()` without VCR wrapper
- `ConfigStateChecks: []statecheck.StateCheck{}`
- `ConfigPlanChecks: resource.ConfigPlanChecks{}`
- `statecheck.ExpectKnownValue()` with `knownvalue.*` predicates
- `plancheck.ExpectKnownValue()` for plan validation
- `CheckDestroy: nil` (common for stateless resources)

### Analyzer Settings Update

```yaml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # Existing analyzers
        enable-basic-test: true
        enable-update-test: true
        enable-import-test: true
        enable-error-test: true
        enable-state-check: true

        # NEW: SDK detection for tests
        enable-sdk-detection: true
        warn-sdkv2-tests: true

        # NEW: Test detection mode
        test-detection-mode: "signature"  # "signature" (Go-native) or "filename" (legacy)
```

### New Analyzer: SDKv2 Test Warning

```go
var SDKv2TestAnalyzer = &analysis.Analyzer{
    Name: "tfprovider-sdkv2-test-warning",
    Doc:  "Warns about tests using SDKv2 patterns that should migrate to Plugin Framework",
    Run:  runSDKv2TestAnalyzer,
}

func runSDKv2TestAnalyzer(pass *analysis.Pass) (interface{}, error) {
    for _, file := range pass.Files {
        for _, decl := range file.Decls {
            fn, ok := decl.(*ast.FuncDecl)
            if !ok {
                continue
            }

            if !isTestFunc(fn, pass.TypesInfo) {
                continue
            }

            if detectSDKv2Test(fn) && !detectPluginFrameworkTest(fn) {
                pos := pass.Fset.Position(fn.Pos())
                pass.Reportf(fn.Pos(),
                    "test '%s' uses SDKv2 patterns\n"+
                    "  Location: %s:%d\n"+
                    "  Detected: acctest.VcrTest, ComposeTestCheckFunc\n"+
                    "  Suggestion: Migrate to Plugin Framework test patterns:\n"+
                    "    - Use ConfigStateChecks instead of Check\n"+
                    "    - Use statecheck.ExpectKnownValue instead of TestCheckResourceAttr\n"+
                    "    - Use ConfigPlanChecks for plan validation",
                    fn.Name.Name, pos.Filename, pos.Line)
            }
        }
    }

    return nil, nil
}
```

### Implementation Priority

| Phase | Task | Effort |
|-------|------|--------|
| 4.1 | Implement `isTestFunc()` with proper signature detection | Low |
| 4.2 | Implement `isAcceptanceTest()` to detect `resource.Test()` calls | Low |
| 4.3 | Implement `detectSDKv2Test()` for legacy test detection | Medium |
| 4.4 | Implement `detectPluginFrameworkTest()` for modern test detection | Medium |
| 4.5 | Add `SDKv2TestAnalyzer` to warn about legacy tests | Medium |
| 4.6 | Remove filename-based detection fallback | Low |

### Sample Output

```
=== Test Analysis for terraform-provider-google-beta ===

Test Functions Detected: 1,245
  - Plugin Framework tests: 312
  - SDKv2 tests (legacy): 933

⚠️ SDKv2 Test Warnings:

1. TestAccDataflowJob_basic uses SDKv2 patterns
   Location: services/dataflow/resource_dataflow_job_test.go:45
   Detected: acctest.VcrTest, ComposeTestCheckFunc
   Suggestion: Migrate to Plugin Framework test patterns

2. TestAccComputeInstance_basic uses SDKv2 patterns
   Location: services/compute/resource_compute_instance_test.go:123
   ...

📊 Test Coverage by SDK:
   Plugin Framework resources: 89% covered
   SDKv2 resources: 76% covered (consider migration)
```

---

## References

- [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework)
- [SDKv2 to Framework Migration](https://developer.hashicorp.com/terraform/plugin/framework/migrating)
- [Framework Benefits](https://developer.hashicorp.com/terraform/plugin/framework-benefits)
- [go/analysis Package](https://pkg.go.dev/golang.org/x/tools/go/analysis)
- [golang.org/x/tools/go/analysis/passes/tests](https://pkg.go.dev/golang.org/x/tools/go/analysis/passes/tests)
- [Go Testing Package](https://pkg.go.dev/testing)
- [golangci-lint Custom Linters](https://golangci-lint.run/plugins/module-plugins/)
