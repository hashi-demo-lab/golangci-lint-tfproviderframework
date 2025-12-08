# Improvements 10: Google Provider Test Coverage Gaps Analysis

**Created:** 2025-12-08
**Status:** ANALYSIS COMPLETE
**Priority:** MEDIUM

## Executive Summary

After implementing universal provider support (recursive scanning, return type detection, generic `resource.TestCase` detection), the Google Beta provider now shows:
- **1419 resources detected** (up from 0)
- **914 resources with tests (64%)**
- **505 untested resources (36%)**
- **682 orphan tests**

Investigation reveals that **57% of "untested" resources are false positives** (nested schema types), and **71.6% of orphan tests** fail matching due to IAM test naming patterns.

## Root Cause Analysis

### Issue 1: False Positive Schema Types (~288 resources, 57% of untested)

**Problem:** The discovery parser extracts nested schema helper types as standalone resources.

**Examples:**
```
accelerators_schema                                    resource_dataproc_cluster.go
accessapproval_folder_settings_enrolled_services_schema resource_folder_access_approval_settings.go
apigee_api_product_attributes_schema                   resource_apigee_api_product.go
apikeys_key_restrictions_schema                        resource_apikeys_key.go
```

**Detection Pattern:** These types:
- End with `_schema` suffix
- Have deeply nested names (multiple underscores)
- Belong to parent resource files
- Never appear in test function names
- Always have 0 tests

### Issue 2: IAM Test Orphans (~488 tests, 71.6% of orphans)

**Problem:** IAM tests use non-standard naming patterns not recognized by the matcher.

**Test Function Pattern:**
```
TestAccComputeBackendBucketIamBindingGenerated
TestAccApigeeEnvironmentIamMemberGenerated
TestAccVertexAIEndpointIamPolicyGenerated_withCondition
```

**Test File Pattern:**
```
iam_compute_backend_bucket_generated_test.go
iam_apigee_environment_generated_test.go
```

**Why Matching Fails:**

1. **Function name extraction** doesn't recognize `Iam` as a special keyword
   - Extracts: `ComputeBackendBucketIamBinding` → fails to match `compute_backend_bucket`

2. **File proximity matching** only recognizes prefixes:
   - `resource_`, `data_source_`, `ephemeral_`
   - Missing: `iam_` prefix

3. **Suffix patterns** not recognized:
   - `_withCondition`, `_withAndWithoutCondition`
   - `Binding`, `Member`, `Policy` suffixes

### Issue 3: Genuinely Untested Resources (~217 resources, 43% of untested)

Categories:
- **Completely untested** (~60): No test files exist
- **Data sources** (~20): Lower testing priority
- **Settings resources** (~15): Tested via parent resources
- **Dry-run/variants** (~30): Specialized variants without dedicated tests
- **Other** (~92): Mixed reasons

## Recommended Solutions

### Phase 1: Filter Schema False Positives (High Impact, Low Effort)

Add filtering to exclude nested schema types from resource detection.

**Implementation in `parser.go`:**

```go
// isNestedSchemaType checks if a resource name represents a nested schema definition
// rather than a standalone resource. These should be excluded from coverage reports.
func isNestedSchemaType(name string, filePath string) bool {
    // Pattern 1: Names ending with _schema are nested type definitions
    if strings.HasSuffix(name, "_schema") {
        return true
    }

    // Pattern 2: Complex nested names with multiple path segments
    // e.g., "apigee_api_product_graphql_operation_group_operation_configs_schema"
    underscoreCount := strings.Count(name, "_")
    if underscoreCount > 5 && strings.Contains(name, "_schema") {
        return true
    }

    return false
}
```

**Expected Impact:** Remove ~288 false positives from untested count.

### Phase 2: Add IAM File Pattern Recognition (High Impact, Low Effort)

Extend file proximity matching to recognize IAM test files.

**Implementation in `matching/utils.go`:**

```go
func ExtractResourceNameFromPath(path string) (string, bool) {
    // ... existing patterns ...

    // Add IAM pattern: iam_<resource>_generated_test.go -> <resource>
    if strings.HasPrefix(nameWithoutTest, "iam_") {
        name := strings.TrimPrefix(nameWithoutTest, "iam_")
        // Remove _generated suffix if present
        name = strings.TrimSuffix(name, "_generated")
        return name, false // IAM tests are for resources, not data sources
    }

    // ... rest of function ...
}
```

**Expected Impact:** Match ~200+ IAM test files to their resources.

### Phase 3: Add IAM Function Name Pattern (High Impact, Medium Effort)

Extend function name matching to recognize IAM test patterns.

**Implementation in `matching/linker.go`:**

```go
func (l *Linker) MatchByFunctionName(funcName string, resourceNames map[string]bool) (string, float64) {
    // ... existing logic ...

    // Handle IAM patterns: TestAccResourceIamBindingGenerated
    if strings.Contains(extracted, "Iam") {
        // Remove IAM keyword and variants
        extracted = strings.Replace(extracted, "Iam", "", 1)
        extracted = strings.TrimSuffix(extracted, "Binding")
        extracted = strings.TrimSuffix(extracted, "Member")
        extracted = strings.TrimSuffix(extracted, "Policy")
        extracted = strings.TrimSuffix(extracted, "Generated")

        // Try matching again with cleaned name
        snakeName := toSnakeCase(extracted)
        if resourceNames[snakeName] {
            return snakeName, 0.95 // High confidence IAM match
        }
    }

    // ... rest of function ...
}
```

**Expected Impact:** Match ~488 IAM test functions to their parent resources.

### Phase 4: Extend Test Function Suffixes (Low Impact, Low Effort)

Add more suffix patterns to strip from test function names.

**Implementation in `matching/linker.go`:**

```go
var TestFunctionSuffixes = []string{
    // Existing
    "_basic", "_generated", "_complete", "_update", "_import",
    "_disappears", "_migrate", "_full", "_minimal",
    // Add for IAM and conditional tests
    "_binding", "_member", "_policy",
    "_withCondition", "_withAndWithoutCondition",
    "Generated", // Without underscore (IAM pattern)
}
```

**Expected Impact:** Better suffix stripping for edge cases.

## Implementation Priority

| Phase | Description | Impact | Effort | Priority |
|-------|-------------|--------|--------|----------|
| 1 | Filter schema false positives | High | Low | P1 |
| 2 | IAM file pattern recognition | High | Low | P1 |
| 3 | IAM function name pattern | High | Medium | P2 |
| 4 | Extended suffix recognition | Low | Low | P3 |

## Expected Results After Implementation

| Metric | Current | After Fixes | Improvement |
|--------|---------|-------------|-------------|
| Untested Resources | 505 | ~217 | -57% |
| Orphan Tests | 682 | ~194 | -72% |
| Coverage Accuracy | 64% | ~85% | +21% |

## Actual vs Reported Coverage

After implementing these fixes, the Google Beta provider coverage would be:

```
Total Resources: 1419 - 288 (schema types) = 1131 real resources
Tested Resources: 914 + ~200 (IAM matches) = ~1114
Actual Coverage: ~98% (vs 64% currently reported)
```

The true coverage gap is much smaller than currently reported due to:
1. False positive schema types inflating the untested count
2. IAM tests not matching their parent resources

## Files to Modify

1. `/workspace/internal/discovery/parser.go`
   - Add `isNestedSchemaType()` function
   - Filter schema types in `parseResources()`

2. `/workspace/internal/matching/utils.go`
   - Add `iam_` prefix handling in `ExtractResourceNameFromPath()`

3. `/workspace/internal/matching/linker.go`
   - Add IAM pattern handling in `MatchByFunctionName()`
   - Extend `TestFunctionSuffixes` list

4. `/workspace/parser_test.go`
   - Add tests for schema type filtering
   - Add tests for IAM pattern matching
