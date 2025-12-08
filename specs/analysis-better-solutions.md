# Better Solutions Analysis: Test Matching Architecture

**Created:** 2025-12-08
**Purpose:** Deep analysis of current architecture and novel approaches for test-to-resource matching.

---

## Critical Finding: Priority Inversion Bug

**The current implementation has a priority inversion bug:**

In `registry.go` (line 17-18):
```go
// MatchTypeInferred indicates the match was found by parsing the HCL config (Highest Priority).
// This is the most reliable match type as it extracts resource names directly from test configurations.
```

But in `linker.go` (lines 59-86), the actual priority is:
```go
// Strategy 1: Function name extraction (highest confidence) → 1.0
// Strategy 2: File proximity (high confidence) → 0.9
// Strategy 3: Inferred Content Matching (medium confidence) → 0.8  ← WRONG!
```

**The comments say Inferred should be highest, but the code gives it lowest priority.**

This single fix would improve matching significantly because Inferred Content matching is based on **actual HCL parsing**, not naming conventions.

---

## Current Architecture Analysis

### What Works Well

1. **HCL Regex Parsing** - `ResourceTypeRegex` correctly extracts resource types from Config strings:
   ```go
   var ResourceTypeRegex = regexp.MustCompile(`(?:resource|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)
   ```

2. **Multi-strategy fallback** - Having multiple strategies provides resilience

3. **Configurable patterns** - Recent improvements made patterns configurable

### What Doesn't Work

1. **Priority is backwards** - Naming heuristics trump actual content analysis
2. **Pattern explosion** - Every new naming convention needs new patterns
3. **No test classification** - Provider tests, function tests, resource tests all mixed
4. **No data source detection in HCL** - Regex only matches `resource` and `action`, not `data`

---

## Better Solution 1: Fix Priority Inversion (Quick Win)

**Change:** Make InferredContent the PRIMARY strategy.

```go
// In linker.go - Corrected priority order:
func (l *Linker) LinkTestsToResources() {
    // Strategy 1: Inferred Content (HIGHEST - based on actual HCL)
    if len(fn.InferredResources) > 0 {
        // ... match using parsed config
        Confidence: 1.0,  // Highest confidence
    }

    // Strategy 2: Function name (fallback)
    if !matchFound {
        // ... match using function name
        Confidence: 0.9,
    }

    // Strategy 3: File proximity (fallback)
    if !matchFound {
        // ... match using file name
        Confidence: 0.8,
    }
}
```

**Impact:** Many false negatives fixed immediately because content matching already works.

---

## Better Solution 2: Extend HCL Regex for Data Sources

**Current regex only matches `resource` and `action`:**
```go
var ResourceTypeRegex = regexp.MustCompile(`(?:resource|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)
```

**Improved regex to also match `data` blocks:**
```go
var ResourceTypeRegex = regexp.MustCompile(`(?:resource|data|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)
```

This would capture data source usage in tests, improving matching for data source tests.

---

## Better Solution 3: Test Classification (Go-Native Approach)

**Problem:** All tests treated equally, but they have different purposes.

**Solution:** Classify tests by analyzing what they configure:

```go
type TestCategory int

const (
    TestCategoryResource TestCategory = iota  // Configures resource/data blocks
    TestCategoryProvider                       // Only configures provider block
    TestCategoryFunction                       // Tests provider functions
    TestCategoryIntegration                    // Infrastructure/integration tests
)

func ClassifyTest(fn *TestFunctionInfo) TestCategory {
    // Check InferredResources - if has resources, it's a resource test
    if len(fn.InferredResources) > 0 {
        return TestCategoryResource
    }

    // Check function name patterns
    if strings.Contains(fn.Name, "ProviderConfig") ||
       strings.Contains(fn.Name, "ProviderFunction") ||
       strings.Contains(fn.Name, "ProviderMeta") {
        return TestCategoryProvider
    }

    // Check file path
    if strings.Contains(fn.FilePath, "/functions/") ||
       strings.Contains(fn.FilePath, "function_") {
        return TestCategoryFunction
    }

    // Check for function tests by looking at Config content
    if hasProviderFunctionCall(fn) {
        return TestCategoryFunction
    }

    return TestCategoryIntegration  // Default for unclassifiable
}
```

**Benefits:**
- Provider tests → reported separately, not as orphans
- Function tests → tracked as their own category
- Integration tests → excluded from resource coverage
- Resource tests → properly matched to resources

---

## Better Solution 4: Config-Driven Mapping File

**Problem:** Pattern matching is fragile and requires code changes.

**Solution:** Allow providers to define explicit mappings in a config file.

**File: `.tfprovidertest.yaml` in provider root:**

```yaml
# Test classification rules
test-classification:
  provider-tests:
    - "TestAccProviderConfig_*"
    - "TestAccProviderFunction_*"
    - "TestAccProvider*"
  function-tests:
    - "Test*Parse_*"
    - "Test*Function_*"
  exclude:
    - "TestAcc*_disappears"  # Cleanup tests

# Resource name aliases (handles naming mismatches)
resource-aliases:
  eda_event_stream: eda_eventstream
  apigee_sharedflow_deployment: apigee_shared_flow_deployment

# Base class test inheritance
base-class-tests:
  resource_iam_binding:
    tested-by:
      - group_iam_binding
      - project_iam_binding
      - organization_iam_binding

# File pattern overrides (provider-specific conventions)
file-patterns:
  prefixes:
    - pattern: "iam_"
      type: resource
      strip-suffixes: ["_generated", "_gen"]
    - pattern: "fw_"  # Framework tests
      type: provider

# Custom helper function patterns
helper-functions:
  - "acctest.VcrTest"
  - "testhelper.AccTest"
```

**Benefits:**
- Explicit, no guessing
- Provider-specific customization
- No code changes for new patterns
- Self-documenting provider conventions
- Can be validated and versioned with provider code

---

## Better Solution 5: Go-Native AST Analysis for Provider Functions

**Problem:** Provider Functions (Terraform 1.6+) not tracked.

**Solution:** Extend the registry to track Functions as first-class entities.

```go
// In registry.go
type ResourceKind int

const (
    ResourceKindResource ResourceKind = iota
    ResourceKindDataSource
    ResourceKindAction
    ResourceKindFunction  // NEW
)

// In parser.go - detect function definitions
func parseProviderFunctions(file *ast.File, filePath string) []*registry.ResourceInfo {
    var functions []*registry.ResourceInfo

    // Look for function.Function implementations
    // Pattern: func NewXxxFunction() function.Function
    ast.Inspect(file, func(n ast.Node) bool {
        funcDecl, ok := n.(*ast.FuncDecl)
        if !ok {
            return true
        }

        // Check if returns function.Function
        if isProviderFunctionFactory(funcDecl) {
            name := extractFunctionName(funcDecl.Name.Name)
            functions = append(functions, &registry.ResourceInfo{
                Name:     name,
                Kind:     registry.ResourceKindFunction,
                FilePath: filePath,
            })
        }
        return true
    })

    return functions
}
```

---

## Better Solution 6: Content-First Matching with Full HCL Parse

**Current:** Simple regex to extract resource types
**Better:** Full HCL parse for comprehensive analysis

```go
import (
    "github.com/hashicorp/hcl/v2"
    "github.com/hashicorp/hcl/v2/hclparse"
)

func parseTestConfig(configStr string) (*TestConfigAnalysis, error) {
    parser := hclparse.NewParser()
    file, diags := parser.ParseHCL([]byte(configStr), "test.tf")
    if diags.HasErrors() {
        return nil, diags
    }

    analysis := &TestConfigAnalysis{
        Resources:   make(map[string]bool),
        DataSources: make(map[string]bool),
        Provider:    nil,
        Functions:   make([]string, 0),
    }

    // Extract all blocks
    content, _ := file.Body.Content(&hcl.BodySchema{
        Blocks: []hcl.BlockHeaderSchema{
            {Type: "resource", LabelNames: []string{"type", "name"}},
            {Type: "data", LabelNames: []string{"type", "name"}},
            {Type: "provider", LabelNames: []string{"name"}},
        },
    })

    for _, block := range content.Blocks {
        switch block.Type {
        case "resource":
            analysis.Resources[block.Labels[0]] = true
        case "data":
            analysis.DataSources[block.Labels[0]] = true
        case "provider":
            analysis.Provider = &block.Labels[0]
        }
    }

    // Also detect function calls in expressions
    analysis.Functions = extractFunctionCalls(file)

    return analysis, nil
}
```

**Benefits:**
- Proper HCL parsing, not regex
- Extracts ALL configuration details
- Handles complex HCL (variables, locals, etc.)
- Future-proof for new HCL features

---

## Better Solution 7: Semantic Analysis with go/types

**Most Go-native approach:** Use Go's type checker for semantic understanding.

```go
import (
    "go/types"
    "golang.org/x/tools/go/packages"
)

func analyzeTestPackage(pkgPath string) (*TestPackageAnalysis, error) {
    cfg := &packages.Config{
        Mode: packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
    }

    pkgs, err := packages.Load(cfg, pkgPath)
    if err != nil {
        return nil, err
    }

    analysis := &TestPackageAnalysis{}

    for _, pkg := range pkgs {
        for _, file := range pkg.Syntax {
            for _, decl := range file.Decls {
                funcDecl, ok := decl.(*ast.FuncDecl)
                if !ok || !strings.HasPrefix(funcDecl.Name.Name, "Test") {
                    continue
                }

                // Use type info to find resource.TestCase usage
                testCases := findTestCaseUsage(funcDecl, pkg.TypesInfo)
                for _, tc := range testCases {
                    // Extract Config field value using type resolution
                    configValue := resolveConfigValue(tc, pkg.TypesInfo)
                    resources := parseHCLConfig(configValue)

                    analysis.Tests = append(analysis.Tests, TestAnalysis{
                        Name:      funcDecl.Name.Name,
                        Resources: resources,
                    })
                }
            }
        }
    }

    return analysis, nil
}
```

**Benefits:**
- Full semantic understanding
- Resolves variables, constants, function calls
- Works with complex test patterns
- Type-safe analysis

**Drawbacks:**
- More complex implementation
- Requires building the package (slower)
- May not work with all provider structures

---

## Recommended Implementation Path

### Phase 1: Quick Wins (Immediate)

1. **Fix priority inversion** - Make InferredContent primary strategy
2. **Extend regex for data sources** - Add `data` to ResourceTypeRegex
3. **Add test classification** - Separate provider/function/resource tests

### Phase 2: Configuration (Short-term)

4. **Implement config file support** - `.tfprovidertest.yaml`
5. **Add resource aliasing** - Handle naming mismatches via config
6. **Add base-class mapping** - Document proxy testing

### Phase 3: Go-Native Enhancements (Medium-term)

7. **Full HCL parsing** - Replace regex with proper parser
8. **Provider Function tracking** - New ResourceKind
9. **Semantic analysis** - go/types for complex cases

---

## Impact Comparison

| Solution | Implementation Effort | Impact | Maintainability |
|----------|----------------------|--------|-----------------|
| Fix priority inversion | 5 min | HIGH | Excellent |
| Extend regex for data | 5 min | MEDIUM | Good |
| Test classification | 2 hrs | HIGH | Good |
| Config file support | 4 hrs | HIGH | Excellent |
| Full HCL parsing | 8 hrs | MEDIUM | Excellent |
| Provider Functions | 4 hrs | MEDIUM | Good |
| Semantic analysis | 16 hrs | LOW | Complex |

---

## Conclusion

The **single most impactful change** is fixing the priority inversion bug - making InferredContent the primary matching strategy. This requires changing ~10 lines of code and would immediately improve matching accuracy.

The **most sustainable long-term solution** is the config file approach (`.tfprovidertest.yaml`) because it:
- Moves provider-specific knowledge out of the tool
- Allows providers to document their conventions
- Eliminates the pattern explosion problem
- Can be validated and tested with the provider

The **most Go-native approach** is combining test classification (AST-based) with full HCL parsing, which leverages Go's strengths in static analysis.

**Recommended order:**
1. Fix priority inversion (5 min, HIGH impact)
2. Add test classification (2 hrs, HIGH impact)
3. Implement config file (4 hrs, HIGH impact, best ROI)
4. Full HCL parsing (8 hrs, MEDIUM impact, good for accuracy)
