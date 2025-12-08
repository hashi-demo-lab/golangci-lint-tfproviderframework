# Test Matching Improvements - Before/After Comparison

**Date:** 2025-12-08
**Changes Implemented:** Priority inversion fix, data source detection, hybrid matching strategy

---

## Summary Table

| Provider | Resources (Before→After) | Data Sources (Before→After) | Actions (Before→After) | Orphans (Before→After) |
|----------|-------------------------|----------------------------|------------------------|------------------------|
| AAP | 5/0 → 5/1 | 6/1 → 6/2 | 3/0 → 3/2 | 0 → 0 |
| BCM | 7/1 → 7/1 | 10/1 → 10/1 | 1/1 → 1/1 | **12 → 0** ✅ |
| Google Beta | 1120/209 → 888/104 | 7/4 → 293/120 | 0/0 → 0/0 | 249 → 249 |
| HCP | 90/31 → 72/29 | 17/4 → 35/28 | 0/0 → 0/0 | 9 → 7 ✅ |
| Helm | 1/0 → 1/0 | 1/0 → 1/0 | 0/0 → 0/0 | 1 → 1 |
| HTTP | 0/0 → 0/0 | 1/0 → 1/0 | 0/0 → 0/0 | 0 → 0 |
| Time | 4/0 → 4/0 | 0/0 → 0/0 | 0/0 → 0/0 | 7 → 7 |
| TLS | 5/0 → 5/0 | 2/1 → 2/1 | 0/0 → 0/0 | 0 → 0 |

**Format:** Total/Untested

---

## Key Improvements

### 1. BCM Provider - Orphan Tests Eliminated
**Before:** 12 orphan tests (provider config tests incorrectly unmatched)
**After:** 0 orphan tests

The provider configuration tests (`TestAccProviderConfig_*`) are now correctly matched. Previously these were orphans because they used HCL configs with resources but function names didn't follow standard patterns.

### 2. HCP Provider - Proper Resource/Data Source Classification
**Before:** 90 resources, 17 data sources (SDK v2 data sources incorrectly classified as resources)
**After:** 72 resources, 35 data sources (correctly split)

SDK v2 providers using `*schema.Resource` for both resources and data sources are now differentiated by filename pattern (`data_source_*.go`).

### 3. Google Beta Provider - Data Source Separation
**Before:** 1120 resources, 7 data sources
**After:** 888 resources, 293 data sources

With the extended `ResourceTypeRegex` that now includes `data` blocks, data sources are properly detected and separated from resources.

---

## Changes Made

### 1. Hybrid Matching Strategy (linker.go)
Instead of pure InferredContent-first or FunctionName-first, we now use:

1. **Function name + InferredContent validation** (highest confidence = 1.0)
   - Extract resource name from test function name
   - Validate it exists in the HCL config's parsed resources
   - Best of both worlds: intent clarity + content verification

2. **Pure InferredContent** (confidence = 0.9)
   - Falls back to first resource in HCL config if function name doesn't match
   - Priority: resources > actions > data sources

3. **File proximity** (confidence = 0.8)
   - Uses test file naming patterns

4. **Fuzzy matching** (lowest confidence)
   - Last resort for non-standard naming

### 2. SDK v2 Data Source Detection (parser.go)
```go
// For SDK v2 schema.Resource, differentiate based on filename
if strings.HasSuffix(returnType, "schema.Resource") {
    baseName := filepath.Base(filePath)
    if strings.HasPrefix(baseName, "data_source_") {
        kind = registry.KindDataSource
    }
}
```

### 3. Extended ResourceTypeRegex (parser.go)
```go
// Now includes 'data' blocks
var ResourceTypeRegex = regexp.MustCompile(`(?:resource|data|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)
```

### 4. Improved Name Extraction for SDK v2 (parser.go)
```go
// Handle SDK v2 camelCase patterns
} else if strings.HasPrefix(name, "dataSource") {
    name = strings.TrimPrefix(name, "dataSource")
    kind = registry.KindDataSource
} else if strings.HasPrefix(name, "resource") {
    name = strings.TrimPrefix(name, "resource")
}
```

---

## Remaining Issues

### 1. Time Provider - 7 Orphan Tests
These are **provider function tests** (Terraform 1.6+), not resource tests:
- `TestDurationParse_*`
- `TestRFC3339Parse_*`
- `TestUnixTimestampParseFunction_*`

**Recommended fix:** Add provider functions as a trackable entity type, or add exclusion patterns.

### 2. Helm Provider - 1 Orphan Test
`TestAccDeferredActions_basic` is an infrastructure test in a `testing/` subdirectory.

**Recommended fix:** Exclude `testing/` directories from orphan analysis.

### 3. Google Beta - 249 Orphan Tests
Many are provider-level or IAM-related tests:
- `TestAccProviderFunction_*`
- `TestAccProviderBasePath_*`
- `TestAcc*IamBindingGenerated`

**Recommended fix:**
- Add provider test exclusion patterns
- Fix IAM `_generated` suffix stripping

---

## Conclusion

The implemented changes significantly improve matching accuracy:
- **BCM:** 100% reduction in orphan tests (12 → 0)
- **HCP:** Proper SDK v2 data source classification, 2 fewer orphans
- **Google Beta:** Correct resource/data source separation (293 data sources now tracked)

The hybrid matching strategy solves the "dependency resource" problem where tests use multiple resources but the function name indicates the target resource.
