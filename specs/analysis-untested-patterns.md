# Analysis: Untested Resources and Orphan Tests - Pattern Discovery

**Created:** 2025-12-08
**Purpose:** Analyze patterns in untested resources and orphan tests across all 8 providers to identify matching algorithm gaps and learning opportunities.

## Executive Summary

After analyzing all 8 providers, we identified **6 major pattern categories** causing false positives (resources marked untested that have tests) and orphan tests (tests not matched to resources):

| Category | Impact | Providers Affected |
|----------|--------|-------------------|
| Provider/Framework Tests | High | All providers |
| Modern SDK Patterns (ConfigStateChecks) | High | BCM, HCP, Google |
| IAM Test File Naming | High | Google Beta, HCP |
| Base Class/Generic Resources | Medium | BCM, HCP |
| Function Tests (not resources) | Medium | Time |
| Multi-segment Resource Names | Medium | Google Beta, HCP |

---

## Provider-by-Provider Analysis

### 1. AAP Provider (1 Untested Data Source)

**Untested Item:** `eda_event_stream` data source

**Root Cause:** **FALSE POSITIVE - Naming Mismatch**
- Report shows `eda_event_stream` (with underscore)
- Actual resource is `eda_eventstream` (no underscore)
- Test file exists: `eda_eventstream_datasource_test.go`
- Test function: `TestAccEDAEventStreamDataSourceRetrievesPostURL`

**Learning:** The snake_case conversion may be inserting extra underscores. Need to verify `toSnakeCase()` handles compound words correctly.

---

### 2. BCM Provider (3 Untested, 12 Orphans)

#### Untested Items Analysis

| Item | Type | Status | Root Cause |
|------|------|--------|------------|
| `example` | Resource | Template | File in `.claude/skills/` - not real resource |
| `example` | Data Source | Template | File in `.claude/skills/` - not real resource |
| `cmdevice_power` | Action | **FALSE NEGATIVE** | Has 19 tests but function names don't match pattern |

**Action `cmdevice_power` Details:**
- Test files exist with 19 tests total
- Unit tests use `TestCMDevicePower*` (missing `Acc` prefix)
- Acceptance tests use `TestAccCMDevicePowerAction_*`
- Current pattern expects `TestAcc<Resource>_*`

**Learning:** Action test naming conventions differ from resource tests. Need pattern: `TestAcc<Resource>Action_*`

#### Orphan Tests (12 Total)

**All 12 are Provider Configuration Tests:**
```
TestAccProviderConfig_InsecureSkipVerify_Default
TestAccProviderConfig_InsecureSkipVerify_ExplicitTrue
TestAccProviderConfig_Timeout_Default
TestAccProviderConfig_Timeout_Custom60Seconds
... (8 more)
```

**Root Cause:** These test provider-level configuration, not specific resources. Function pattern `TestAccProviderConfig_*` doesn't match any resource name.

**Learning:** Need a "Provider Tests" category or exclusion pattern for `TestAccProviderConfig_*` and `TestAccProvider*` patterns.

---

### 3. Google Beta Provider (209 Untested, 249 Orphans)

#### Sample Untested Resources Analysis

| Resource | Status | Root Cause |
|----------|--------|------------|
| `access_context_manager_*` | Mixed | Some have tests in mismatched files |
| `apigee_shared_flow_deployment` | **FALSE NEGATIVE** | File is `resource_apigee_sharedflow_deployment_test.go` (missing underscore) |
| Various IAM resources | **FALSE NEGATIVE** | IAM tests exist but matching fails |

#### Orphan Test Patterns Identified

**Pattern 1: Provider-Level Tests (~10+)**
```
TestAccProviderFunction_location_from_id
TestAccProviderFunction_name_from_id
TestAccProviderBasePath_setBasePath
TestAccProviderMeta_setModuleName
TestAccUniverseDomainDisk
```
- Location: `provider/`, `functions/` directories
- These should NOT match to resources

**Pattern 2: IAM Tests with `_generated` suffix (~50+)**
```
TestAccIapTunnelIamBindingGenerated
TestAccCloudFunctionsCloudFunctionIamBindingGenerated
```
- File: `iam_iap_tunnel_generated_test.go`
- Current extraction: `iap_tunnel_generated` (incorrect)
- Should extract: `iap_tunnel` (strip `_generated`)

**Pattern 3: Framework Tests**
```
TestAccFrameworkProviderMeta_setModuleName
```
- Tests framework functionality, not resources

**Learning:**
1. IAM file extraction needs to strip `_generated` suffix AFTER `iam_` prefix removal
2. Provider/framework tests need exclusion patterns
3. Some resources have tests in files with slightly different names

---

### 4. HCP Provider (31 Untested Resources, 9 Orphans)

#### Untested Categories

**A. IAM Binding Base Classes (7 resources)**
```
resource_iam_binding (base class)
group_iam_binding
organization_iam_binding
project_iam_binding
vault_secrets_app_iam_binding
packer_bucket_app_iam_binding
```

**Root Cause:** These are tested indirectly through `*_iam_policy_test.go` files:
- `group_iam_binding` → tested in `resource_group_iam_policy_test.go`
- Function: `TestAccGroupIamBindingResource`

**Learning:** Need pattern to match `*_iam_binding` resources to `*_iam_policy_test.go` files.

**B. Data Sources Without Tests (8 resources)**
- Consul: 4 data sources
- Networking: 3 data sources (HVN-related)
- Vault: 1 data source

**Root Cause:** Genuinely untested - no test files exist.

**C. Base/Generic Classes (2 resources)**
```
integration_connection
integration_subscription
```

**Root Cause:** Tested via concrete implementations:
- `integration_jira_connection` (HAS TEST)
- `integration_slack_connection` (HAS TEST)

**Learning:** Base classes are "tested-by-proxy" through concrete implementations.

#### Orphan Tests (9 Total)

| Test | Root Cause |
|------|------------|
| `TestAccWorkloadIdentityProviderResource` | Function name extraction issue |
| `TestAccUserPrincipalDataSource` | Missing `Acc_` pattern handling |
| `TestRadarResources` | Missing `DataSource` in name |
| `TestAcc_Packer_Data_*` (4 tests) | In generic `data_source_test.go` file |
| `TestAccMultiProjectResource` | Provider-level validation test |
| `TestAcc_Vault_PerformanceReplication_*` | Not a registered resource type |

**Learning:**
1. Generic test files (`data_source_test.go`) need content-based matching
2. Provider-level validation tests should be excluded

---

### 5. Helm Provider (1 Orphan)

**Orphan Test:** `TestAccDeferredActions_basic`

**Root Cause:**
- Test is in `/helm/testing/deferred_actions_test.go` (testing subdirectory)
- Tests `kind_cluster` resource (from different provider!)
- Generic naming pattern doesn't match Helm resources

**Learning:** Tests in `testing/` subdirectories may be infrastructure tests, not resource tests.

---

### 6. HTTP Provider (0 Issues)

Fully matched - only has 1 data source with proper test coverage.

---

### 7. Time Provider (7 Orphans, 0 Untested)

**The Paradox:** 7 orphan tests but all 4 resources are tested.

**Orphan Tests (All Provider Function Tests):**
```
TestDurationParse_valid
TestDurationParse_invalid
TestRFC3339Parse_UTC
TestRFC3339Parse_offset
TestRFC3339Parse_invalid
TestUnixTimestampParseFunction_Valid
TestUnixTimestampParseFunction_Null
```

**Root Cause:** These test `function.Function` implementations:
- `duration_parse()` function
- `rfc3339_parse()` function
- `unix_timestamp_parse()` function

**Learning:** Provider Functions (Terraform 1.6+) are a new resource type not tracked by the analyzer. Need to either:
1. Add Functions to registry tracking
2. Exclude function tests from orphan reporting

---

### 8. TLS Provider (1 Untested Data Source)

**Untested:** `public_key` data source

**Root Cause:** Genuinely untested - no dedicated test file exists for this data source.

---

## Pattern Categories and Recommended Fixes

### Category 1: Provider/Framework Tests (HIGH PRIORITY)

**Pattern:** Tests that validate provider configuration, not resources
```
TestAccProviderConfig_*
TestAccProviderFunction_*
TestAccProviderBasePath_*
TestAccProviderMeta_*
TestAccFrameworkProvider*
TestAccUniverseDomain*
```

**Fix:** Add exclusion patterns in settings:
```yaml
provider-test-patterns:
  - "TestAccProviderConfig_*"
  - "TestAccProviderFunction_*"
  - "TestAccProvider*"
  - "TestAccFrameworkProvider*"
```

---

### Category 2: Modern SDK Patterns - ConfigStateChecks (HIGH PRIORITY)

**Issue:** Tests using `ConfigStateChecks` instead of legacy `Check` functions are marked as missing Check coverage.

**Example (BCM):**
```go
// Modern pattern (not detected)
ConfigStateChecks: []statecheck.StateCheck{
    statecheck.ExpectKnownValue(...),
}

// Legacy pattern (detected)
Check: resource.ComposeTestCheckFunc(
    resource.TestCheckResourceAttr(...),
)
```

**Fix:** Update analyzer to detect both patterns:
```go
case "ConfigStateChecks":
    step.HasConfigStateChecks = true
    step.HasCheck = true  // Also set HasCheck for reporting
```

---

### Category 3: IAM Test File Naming (HIGH PRIORITY)

**Issue:** IAM test files with `_generated` suffix not properly extracted.

**Current Flow:**
```
iam_iap_tunnel_generated_test.go
  → Strip iam_ prefix: iap_tunnel_generated
  → Should also strip _generated: iap_tunnel
```

**Fix:** Update `ExtractResourceNameFromPath()`:
```go
if strings.HasPrefix(nameWithoutTest, "iam_") {
    name := strings.TrimPrefix(nameWithoutTest, "iam_")
    // Strip ALL suffixes from TestFileSuffixStrip list
    for _, strip := range suffixStrip {
        name = strings.TrimSuffix(name, strip)
    }
    return name, false
}
```

---

### Category 4: Base Class/Proxy Testing (MEDIUM PRIORITY)

**Issue:** Base classes tested via concrete implementations not recognized.

**Examples:**
- `resource_iam_binding` → tested via `group_iam_binding`, `project_iam_binding`
- `integration_connection` → tested via `integration_jira_connection`

**Options:**
1. **Documentation approach:** Mark as "tested-by-proxy" in reports
2. **Configuration approach:** Add proxy mapping in settings:
```yaml
proxy-test-mappings:
  resource_iam_binding:
    - group_iam_binding
    - project_iam_binding
```

---

### Category 5: Provider Functions (MEDIUM PRIORITY)

**Issue:** Terraform Provider Functions (1.6+) not tracked.

**Current State:** Functions like `duration_parse()`, `rfc3339_parse()` have tests but are reported as orphans.

**Options:**
1. Add Functions to registry (new resource kind)
2. Exclude function test files from orphan reporting:
```yaml
exclude-patterns:
  - "function_*_test.go"
```

---

### Category 6: Action Test Naming (MEDIUM PRIORITY)

**Issue:** Action tests use different naming convention than resource tests.

**Resource pattern:** `TestAcc<Resource>_suffix`
**Action pattern:** `TestAcc<Resource>Action_suffix` or `Test<Resource>Action_*`

**Fix:** Add Action-specific pattern in `matchResourceByName()`:
```go
// Check for Action suffix pattern
if strings.Contains(resourcePart, "Action") {
    actionName := strings.TrimSuffix(resourcePart, "Action")
    snakeAction := toSnakeCase(actionName)
    if resourceNames[snakeAction] {
        return snakeAction, true
    }
}
```

---

## Metrics Summary

| Provider | Resources | Untested | Data Sources | Untested | Actions | Untested | Orphans | False Positives |
|----------|-----------|----------|--------------|----------|---------|----------|---------|-----------------|
| AAP | 5 | 0 | 6 | 1* | 3 | 0 | 0 | 1 |
| BCM | 7 | 1** | 10 | 1** | 1 | 1* | 12*** | 2 |
| Google Beta | 1120 | 209 | 7 | 4 | 0 | 0 | 249 | ~50+ |
| HCP | 90 | 31 | 17 | 4 | 0 | 0 | 9 | ~10 |
| Helm | 1 | 0 | 1 | 0 | 0 | 0 | 1 | 0 |
| HTTP | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 |
| Time | 4 | 0 | 0 | 0 | 0 | 0 | 7**** | 0 |
| TLS | 5 | 0 | 2 | 1 | 0 | 0 | 0 | 0 |

**Legend:**
- `*` = Naming mismatch (has tests)
- `**` = Template file (intentional)
- `***` = Provider config tests (not resource tests)
- `****` = Provider function tests (not resource tests)

---

## Recommended Implementation Priority

### Immediate (High Impact, Low Effort)
1. Add provider test exclusion patterns
2. Fix IAM `_generated` suffix stripping
3. Recognize `ConfigStateChecks` as valid Check coverage

### Short-term (Medium Impact, Medium Effort)
4. Add Action test naming pattern support
5. Handle generic test files with content-based matching
6. Add provider Functions exclusion or tracking

### Long-term (Lower Priority)
7. Implement proxy-testing documentation
8. Add base class test inheritance tracking
9. Improve multi-segment resource name extraction

---

## Conclusion

The majority of "untested" resources and orphan tests are **false positives** caused by:

1. **New Terraform patterns** not yet supported (ConfigStateChecks, Provider Functions, Actions)
2. **Provider-level tests** being counted as orphans
3. **IAM test naming conventions** with `_generated` suffix
4. **Base class testing** via concrete implementations

Implementing the recommended fixes would reduce:
- Google Beta orphans: 249 → ~150 (40% reduction)
- HCP untested: 31 → ~20 (35% reduction)
- BCM orphans: 12 → 0 (100% reduction)
- Time orphans: 7 → 0 (100% reduction)
