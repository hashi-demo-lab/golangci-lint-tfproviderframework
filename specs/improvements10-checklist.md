# Improvements 10: Google Provider Gaps - Implementation Checklist

**Created:** 2025-12-08
**Status:** COMPLETE
**Key Improvement:** All patterns are now CONFIGURABLE via settings (not hardcoded)

## Phase 1: Filter Schema False Positives

- [x] Add `isNestedSchemaType()` function to `parser.go`
- [x] Integrate filter into `parseResources()` or `ReturnTypeStrategy`
- [x] Make patterns configurable via `NestedSchemaPatterns` setting
- [x] Rebuild and test against Google Beta
- [x] Verify reduction in false positive resources
- [x] Update reports

**Expected Impact:** Remove ~288 false positives from untested count
**Actual Impact:** Removed 299 false positives (Resources: 1419→1120, Untested: 505→210)

---

## Phase 2: Add IAM File Pattern Recognition

- [x] Add `iam_` prefix handling in `ExtractResourceNameFromPath()`
- [x] Handle `_generated` suffix removal for IAM files
- [x] Make patterns configurable via `TestFilePrefixPatterns`, `TestFileSuffixStrip` settings
- [x] Rebuild and test against Google Beta
- [x] Verify IAM test files now match resources (via file proximity)
- [x] Update reports

**Expected Impact:** Match ~200+ IAM test files to their resources
**Actual Impact:** Matched 379 IAM tests via file proximity (Orphans: 689→310)

---

## Phase 3: Add IAM Function Name Pattern

- [x] Add IAM keyword detection in `matchResourceByName()`
- [x] Handle `Binding`, `Member`, `Policy` suffix removal
- [x] Handle `Generated` suffix (without underscore)
- [x] Make keywords configurable via `FunctionNameKeywordsToStrip` setting
- [x] Rebuild and test against Google Beta
- [x] Verify orphan test count reduction
- [x] Update reports

**Expected Impact:** Match ~488 IAM test functions to parent resources
**Actual Impact:** Orphans reduced from 310 to 249 (61 additional matches via function name)

---

## Phase 4: Extended Suffix Recognition

- [x] Add `_withCondition` suffix pattern
- [x] Add `_withAndWithoutCondition` suffix pattern
- [x] Add other extended suffixes (`_emptyCondition`, `_example`, `_simple`, `_advanced`)
- [x] Make suffixes configurable via `TestFunctionSuffixes` setting
- [x] Rebuild and test against Google Beta
- [x] Final report generation for all providers

**Expected Impact:** Better matching for conditional test variants
**Actual Impact:** Stable results (249 orphans) - suffixes already work in existing patterns

---

## Final Validation

- [x] Build succeeds with all changes
- [x] Test against all 8 providers
- [x] Document final coverage statistics
- [x] Compare before/after metrics

---

## Progress Log

| Phase | Started | Completed | Notes |
|-------|---------|-----------|-------|
| 1 | 2025-12-08 | 2025-12-08 | Removed 299 schema false positives, made configurable |
| 2 | 2025-12-08 | 2025-12-08 | File pattern recognition, made configurable |
| 3 | 2025-12-08 | 2025-12-08 | Function name keyword stripping, made configurable |
| 4 | 2025-12-08 | 2025-12-08 | Extended suffixes added, made configurable |

## Metrics Tracking

| Metric | Before | After Phase 1 | After Phase 2 | After Phase 3 | After Phase 4 |
|--------|--------|---------------|---------------|---------------|---------------|
| Google Resources | 1419 | 1120 | 1120 | 1120 | 1120 |
| Google Untested | 505 | 210 | 210 | 209 | 209 |
| Google Orphan Tests | 682 | 689 | 310 | 249 | 249 |
| Google Coverage % | 64% | 81% | 81% | 81% | 81% |

## New Configuration Options Added

All patterns are now configurable via the Settings struct:

```yaml
# Test file pattern extraction
test-file-prefix-patterns:
  - "resource_:false"
  - "data_source_:true"
  - "iam_:false"  # IAM test files
test-file-suffix-patterns:
  - "_resource:false"
  - "_data_source:true"
test-file-suffix-strip:
  - "_generated"
  - "_gen"

# Resource filtering
nested-schema-patterns:
  - "*_schema"
  - "*_schema_*"

# Function name matching
function-name-keywords-to-strip:
  - "IamBinding"
  - "IamMember"
  - "IamPolicy"
  - "Iam"
  - "Generated"

# Test function suffixes (empty = use defaults)
test-function-suffixes: []
```

## Summary

Successfully implemented all 4 phases with **fully configurable** patterns (no hardcoded strings):

1. **Schema filtering**: Uses pattern matching with wildcards
2. **File patterns**: Configurable prefix/suffix patterns with data source flags
3. **Function keywords**: Configurable CamelCase keywords to strip
4. **Test suffixes**: Configurable snake_case suffixes

The implementation is generic and provider-agnostic - patterns can be customized for any provider's naming conventions.
