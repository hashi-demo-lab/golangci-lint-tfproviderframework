# Improvements 9: Universal Provider Structure Support

**Created:** 2025-12-08
**Status:** SPECIFICATION
**Priority:** HIGH

## Executive Summary

The current linter has rigid assumptions about provider structure that cause it to fail on many real-world Terraform providers. Analysis of 8 providers revealed that only 3 (AAP, BCM, HTTP) work correctly. This document specifies changes needed to support any provider regardless of directory structure or naming conventions.

## Problem Analysis

### Provider Compatibility Matrix

| Provider | Resources Found | Tests Found | Issue |
|----------|----------------|-------------|-------|
| **AAP** | 5 + 5 DS + 3 Actions | All matched | ✅ Works |
| **BCM** | 6 + 9 DS + 1 Action | All matched | ✅ Works |
| **HTTP** | 1 DS | All matched | ✅ Works |
| **Time** | 4 | All matched | ✅ Works |
| **Google Beta** | 0 | N/A | ❌ Service-based directory structure |
| **Helm** | 0 | 54 orphans | ❌ Non-standard type naming |
| **HCP** | 0 | N/A | ❌ Subdirectory-organized resources |
| **TLS** | 5 + 2 DS | 0 matched | ❌ Test pattern matching failures |

### Root Causes

#### Issue 1: Directory Structure Assumptions

**Current behavior:** Linter expects `internal/provider/` or `<provider-name>/` directory structure.

**Failing patterns:**
- **Google Beta**: `google-beta/services/<service>/` (174 service subdirectories)
- **HCP**: `internal/provider/<service>/` (waypoint/, vaultsecrets/, etc.)
- **Helm**: `helm/` (flat structure, no `internal/`)

#### Issue 2: Type Naming Assumptions

**Current behavior:** Discovery strategies require types ending in `Resource`, `DataSource`, or `Action`.

```go
// Current detection (parser.go:176-178)
isDataSource := strings.HasSuffix(recvType, "DataSource")
isResource := strings.HasSuffix(recvType, "Resource")
```

**Failing patterns:**
- **Helm**: `HelmRelease` (ends in "Release"), `HelmTemplate` (ends in "Template")
- **TLS**: `privateKeyEphemeralResource` (ephemeral but ends in "Resource")

#### Issue 3: Factory Function Assumptions

**Current behavior:** Expects `NewXxxResource` or `NewXxxDataSource` patterns.

```go
// Current detection (parser.go:230-232)
isDataSource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "DataSource")
isResource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "Resource")
```

**Failing patterns:**
- **Helm**: `NewHelmRelease()`, `NewHelmTemplate()` - no "Resource"/"DataSource" keyword
- **Google**: `ResourceComputeDisk()` - prefix instead of suffix

#### Issue 4: Test Pattern Assumptions

**Current behavior:** Test functions must match `TestAcc*`, `TestResource*`, or `TestDataSource*`.

**Failing patterns:**
- **TLS**: `TestPrivateKeyRSA` (no underscore, no TestAcc prefix)
- **Google**: Generated tests may use different patterns

## Recommended Solution

### Phase 1: Recursive Directory Scanning

**Goal:** Support any directory structure by recursively scanning for Go files.

**Changes:**

1. **Remove hardcoded path assumptions** in `cmd/validate/main.go`
2. **Add recursive scanning option** with configurable depth
3. **Support explicit path specification** via `-path` flag

```go
// New configuration options
type ScanConfig struct {
    RootPath       string   // Provider root directory
    ScanPaths      []string // Explicit paths to scan (optional)
    Recursive      bool     // Enable recursive scanning (default: true)
    MaxDepth       int      // Maximum recursion depth (default: 10)
    ExcludePaths   []string // Paths to exclude (e.g., vendor/, testdata/)
}
```

**Implementation:**
```go
func findGoPackages(root string, config ScanConfig) []string {
    var packages []string
    filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if d.IsDir() {
            // Check for .go files in directory
            if hasGoFiles(path) && !isExcluded(path, config) {
                packages = append(packages, path)
            }
        }
        return nil
    })
    return packages
}
```

### Phase 2: Interface-Based Resource Detection

**Goal:** Detect resources by interface implementation, not naming conventions.

**Strategy: Detect terraform-plugin-framework interface compliance**

A valid resource must have:
- `Metadata(context.Context, resource.MetadataRequest, *resource.MetadataResponse)` method
- `Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)` method

A valid data source must have:
- `Metadata(context.Context, datasource.MetadataRequest, *datasource.MetadataResponse)` method
- `Schema(context.Context, datasource.SchemaRequest, *datasource.SchemaResponse)` method

**New detection strategy:**

```go
// InterfaceComplianceStrategy detects resources by checking method signatures
type InterfaceComplianceStrategy struct{}

func (s *InterfaceComplianceStrategy) Detect(file *ast.File, fset *token.FileSet) []*ResourceInfo {
    var resources []*ResourceInfo

    // Find all type declarations
    for _, decl := range file.Decls {
        genDecl, ok := decl.(*ast.GenDecl)
        if !ok || genDecl.Tok != token.TYPE {
            continue
        }

        for _, spec := range genDecl.Specs {
            typeSpec := spec.(*ast.TypeSpec)
            typeName := typeSpec.Name.Name

            // Check if type has required methods
            methods := findMethodsForType(file, typeName)

            if hasResourceInterface(methods) {
                // Extract TypeName from Metadata method body
                resourceName := extractTypeNameFromMetadata(file, typeName)
                resources = append(resources, &ResourceInfo{
                    Name: resourceName,
                    Kind: KindResource,
                    // ...
                })
            }

            if hasDataSourceInterface(methods) {
                // ...
            }
        }
    }
    return resources
}

func hasResourceInterface(methods map[string]*ast.FuncDecl) bool {
    required := []string{"Metadata", "Schema", "Create", "Read", "Delete"}
    for _, m := range required[:3] { // At minimum Metadata + Schema
        if _, ok := methods[m]; !ok {
            return false
        }
    }
    return true
}
```

### Phase 3: Enhanced Factory Function Detection

**Goal:** Detect factory functions by return type, not naming.

**Current limitation:** Only checks function name for keywords.

**New approach:** Analyze return type from function signature.

```go
// ReturnTypeStrategy detects resources by factory function return types
func (s *ReturnTypeStrategy) Detect(file *ast.File, fset *token.FileSet) []*ResourceInfo {
    var resources []*ResourceInfo

    for _, decl := range file.Decls {
        funcDecl, ok := decl.(*ast.FuncDecl)
        if !ok || funcDecl.Type.Results == nil {
            continue
        }

        // Check return type
        for _, result := range funcDecl.Type.Results.List {
            returnType := typeToString(result.Type)

            // Check if returns resource.Resource or datasource.DataSource
            if isResourceReturnType(returnType) {
                // Parse function body for TypeName
                resourceName := extractResourceNameFromFactory(funcDecl)
                if resourceName != "" {
                    resources = append(resources, &ResourceInfo{
                        Name: resourceName,
                        Kind: determineKind(returnType),
                        // ...
                    })
                }
            }
        }
    }
    return resources
}

func isResourceReturnType(typeName string) bool {
    resourceTypes := []string{
        "resource.Resource",
        "*schema.Resource",
        "datasource.DataSource",
        "*schema.DataSource",
    }
    for _, rt := range resourceTypes {
        if strings.HasSuffix(typeName, rt) {
            return true
        }
    }
    return false
}
```

### Phase 4: Provider Registration Analysis

**Goal:** Extract resource list from provider registration.

**Google pattern:**
```go
// provider_mmv1_resources.go
var handwrittenResources = map[string]*schema.Resource{
    "google_compute_disk": compute.ResourceComputeDisk(),
    "google_storage_bucket": storage.ResourceStorageBucket(),
}
```

**HCP/Helm pattern:**
```go
func (p *Provider) Resources(ctx context.Context) []func() resource.Resource {
    return []func() resource.Resource{
        NewHelmRelease,
        NewHelmTemplate,
    }
}
```

**New strategy:**

```go
// ProviderRegistrationStrategy extracts resources from provider registration
func (s *ProviderRegistrationStrategy) Detect(file *ast.File, fset *token.FileSet) []*ResourceInfo {
    var resources []*ResourceInfo

    // Pattern 1: Map literal with resource names as keys
    // var resources = map[string]*schema.Resource{"google_compute_disk": ...}
    for _, decl := range file.Decls {
        if mapLit := findResourceMapLiteral(decl); mapLit != nil {
            for _, elt := range mapLit.Elts {
                kv := elt.(*ast.KeyValueExpr)
                resourceName := extractStringLiteral(kv.Key)
                resources = append(resources, &ResourceInfo{
                    Name: resourceName,
                    Kind: KindResource,
                })
            }
        }
    }

    // Pattern 2: Resources() method returning slice of factory functions
    // func (p *Provider) Resources(ctx) []func() resource.Resource { return []func(){NewX, NewY} }
    for _, decl := range file.Decls {
        if funcDecl := findResourcesMethod(decl); funcDecl != nil {
            factories := extractFactoryFunctions(funcDecl)
            for _, factory := range factories {
                // Resolve factory to actual resource
                resourceName := resolveFactoryToResourceName(file, factory)
                resources = append(resources, &ResourceInfo{
                    Name: resourceName,
                    Kind: KindResource,
                })
            }
        }
    }

    return resources
}
```

### Phase 5: Flexible Test Pattern Matching

**Goal:** Support varied test naming conventions.

**Changes:**

1. **Relaxed test function detection:**
```go
func isAcceptanceTest(funcName string) bool {
    // Primary patterns (high confidence)
    if strings.HasPrefix(funcName, "TestAcc") ||
       strings.HasPrefix(funcName, "TestResource") ||
       strings.HasPrefix(funcName, "TestDataSource") {
        return true
    }

    // Secondary patterns (medium confidence)
    // Any Test* function in a *_test.go file that calls resource.Test()
    return strings.HasPrefix(funcName, "Test")
}

func isAcceptanceTestByContent(funcDecl *ast.FuncDecl) bool {
    // Check if function body contains resource.Test() or resource.ParallelTest()
    return containsResourceTestCall(funcDecl.Body)
}
```

2. **Content-based test classification:**
```go
func classifyTestFunction(funcDecl *ast.FuncDecl, file *ast.File) TestType {
    body := funcDecl.Body

    if containsCall(body, "resource.Test") || containsCall(body, "resource.ParallelTest") {
        return AcceptanceTest
    }
    if containsCall(body, "resource.UnitTest") {
        return UnitTest
    }
    return UnknownTest
}
```

### Phase 6: Configuration Options

**Goal:** Allow users to customize detection for their provider.

```yaml
# .golangci.yml
linters-settings:
  tfprovidertest:
    # Directory scanning
    scan-paths:
      - "internal/provider"
      - "google-beta/services"
    recursive: true
    exclude-paths:
      - "vendor"
      - "testdata"

    # Resource detection
    detection-strategies:
      - interface-compliance  # Check for Metadata+Schema methods
      - return-type          # Check factory function return types
      - provider-registration # Parse Resources() method
      - naming-convention    # Fallback to current behavior

    # Custom type patterns (regex)
    resource-type-patterns:
      - ".*Resource$"
      - ".*Release$"      # For Helm
      - ".*Template$"     # For Helm

    # Test patterns
    test-function-patterns:
      - "^TestAcc"
      - "^TestResource"
      - "^TestDataSource"
      - "^Test.*_"        # Any Test with underscore
```

## Implementation Plan

### Priority Order

1. **Phase 1: Recursive Directory Scanning** (Effort: Small)
   - Immediate value for Google, HCP
   - Low risk, additive change

2. **Phase 4: Provider Registration Analysis** (Effort: Medium)
   - High value for all providers
   - Extracts authoritative resource list

3. **Phase 2: Interface-Based Detection** (Effort: Medium)
   - Fixes Helm, catches edge cases
   - More robust than naming conventions

4. **Phase 5: Flexible Test Matching** (Effort: Small)
   - Fixes TLS orphan issue
   - Quick win

5. **Phase 3: Return Type Detection** (Effort: Medium)
   - Complements interface detection
   - Catches factory functions

6. **Phase 6: Configuration** (Effort: Small)
   - Allows user customization
   - Future-proofs the tool

### Success Criteria

After implementation, all 8 test providers should produce meaningful reports:

| Provider | Expected Resources | Expected Coverage |
|----------|-------------------|-------------------|
| AAP | 5 + 5 DS + 3 Actions | 100% |
| BCM | 6 + 9 DS + 1 Action | 100% |
| HTTP | 1 DS | 100% |
| Time | 4 | 100% |
| Google Beta | 1800+ resources | >90% |
| Helm | 1 + 1 DS | 100% |
| HCP | 11+ resources | >90% |
| TLS | 5 + 2 DS | 100% |

## Appendix: Provider Structure Examples

### Pattern A: Standard (AAP, BCM, HTTP, Time)
```
provider/
└── internal/provider/
    ├── resource_*.go
    ├── data_source_*.go
    └── *_test.go
```

### Pattern B: Service-Based (Google)
```
provider/
└── google-beta/
    ├── provider/
    │   └── provider.go
    └── services/
        ├── compute/
        │   ├── resource_compute_*.go
        │   └── resource_compute_*_test.go
        ├── storage/
        └── [170+ more services]
```

### Pattern C: Feature-Based (HCP)
```
provider/
└── internal/provider/
    ├── provider.go
    ├── waypoint/
    │   ├── resource_waypoint_*.go
    │   └── resource_waypoint_*_test.go
    ├── vaultsecrets/
    └── [more features]
```

### Pattern D: Flat (Helm)
```
provider/
└── helm/
    ├── provider.go
    ├── resource_helm_release.go
    ├── resource_helm_release_test.go
    ├── data_helm_template.go
    └── data_helm_template_test.go
```

### Pattern E: Mixed (TLS)
```
provider/
└── internal/provider/
    ├── provider.go
    ├── resource_*.go
    ├── data_source_*.go
    ├── ephemeral_*.go
    └── *_test.go  (various naming patterns)
```
