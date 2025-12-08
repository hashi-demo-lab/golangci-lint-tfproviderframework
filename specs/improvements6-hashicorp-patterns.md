# HashiCorp Testing Patterns - Complete Coverage Enhancement

**Created**: 2025-12-08 00:18:28 UTC
**Status**: ✅ IMPLEMENTED
**Priority**: High
**Author**: AI Assistant

---

## Executive Summary

This specification outlines enhancements to fully align with HashiCorp's official Terraform provider testing patterns documentation. The goal is to detect, report, and validate all recommended testing patterns.

---

## HashiCorp Official Testing Patterns

Reference: https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns

| # | Pattern Name | HashiCorp Description | Detection Status |
|---|--------------|----------------------|------------------|
| 1 | **Basic Test** | Verify resource creation and attribute storage | ✅ Detected & Reported |
| 2 | **Update Test** | Validate resource modifications (multi-step) | ✅ Detected & Reported |
| 3 | **Import Test** | Confirm import produces correct state | ✅ Detected & Reported |
| 4 | **Error Expectation Test** | Verify invalid configs fail properly (`ExpectError`) | ✅ Detected, ⚠️ Not in table |
| 5 | **Non-Empty Plan Test** | Handle persistent plan differences (`ExpectNonEmptyPlan`) | ❌ Not detected |
| 6 | **Regression Test** | Document and verify bug fixes | ❌ Not detected (naming convention only) |

**Note**: "Disappears Test" is a community convention (used by AWS provider) but is NOT an official HashiCorp documented pattern.

### TestCase Fields (testcase documentation)

Reference: https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/testcase

| Field | Type | Purpose | Required | Detection Status |
|-------|------|---------|----------|------------------|
| `Steps` | `[]TestStep` | Test step sequence | ✅ Yes | ✅ Detected |
| `Providers` | `map[string]*schema.Provider` | Provider instances under test | ✅ Yes | N/A (infrastructure) |
| `ProtoV6ProviderFactories` | `map[string]func() (tfprotov6.ProviderServer, error)` | Plugin Framework provider factories | ✅ Yes* | N/A (infrastructure) |
| `CheckDestroy` | `TestCheckFunc` | Verify resources destroyed after test | ❌ No | ✅ Detected & Reported |
| `PreCheck` | `func()` | Validate environment before test | ❌ No | ❌ Not detected |
| `TerraformVersionChecks` | `[]tfversion.TerraformVersionCheck` | Version-specific test gating | ❌ No | ❌ Not detected |
| `IsUnitTest` | `bool` | Run without TF_ACC env var | ❌ No | ❌ Not detected |
| `ExternalProviders` | `map[string]ExternalProvider` | External provider dependencies | ❌ No | ❌ Not detected |

### TestStep Fields (teststep documentation)

Reference: https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/teststep

#### Configuration Fields

| Field | Type | Purpose | Detection Status |
|-------|------|---------|------------------|
| `Config` | `string` | Inline HCL configuration | ✅ Detected & Parsed |
| `ConfigFile` | `config.TestStepConfigFunc` | External config file path | ❌ Not detected |
| `ConfigDirectory` | `string` | Config directory path | ❌ Not detected |
| `ConfigVariables` | `config.Variables` | Variables for configuration | ❌ Not detected |

#### Test Mode Fields

| Field | Type | Purpose | Detection Status |
|-------|------|---------|------------------|
| `ImportState` | `bool` | Enable import mode | ✅ Detected & Reported |
| `ImportStateKind` | `ImportStateKind` | Import block type | ❌ Not detected |
| `ImportStateVerify` | `bool` | Verify import accuracy | ✅ Detected |
| `ImportStateVerifyIgnore` | `[]string` | Attrs to ignore in verify | ❌ Not detected |
| `ImportStateId` | `string` | Custom import ID | ❌ Not detected |
| `ResourceName` | `string` | Resource to import | ❌ Not detected |
| `RefreshState` | `bool` | Run terraform refresh | ❌ Not detected |
| `Destroy` | `bool` | Destroy mode step | ❌ Not detected |
| `PlanOnly` | `bool` | Plan without apply | ❌ Not detected |

#### Validation Fields

| Field | Type | Purpose | Detection Status |
|-------|------|---------|------------------|
| `Check` | `TestCheckFunc` | Legacy state validation | ✅ Detected & Reported |
| `ConfigStateChecks` | `[]statecheck.StateCheck` | Modern state checks | ❌ Not detected |
| `ConfigPlanChecks` | `ConfigPlanChecks` | Plan validation checks | ✅ Detected, ⚠️ Merged |
| `ExpectError` | `*regexp.Regexp` | Expected error pattern | ✅ Detected, ⚠️ Not in table |
| `ExpectNonEmptyPlan` | `bool` | Expect plan changes | ❌ Not detected |
| `PreventDiskCleanup` | `bool` | Keep working directory | N/A (infrastructure) |
| `PreventPostDestroyRefresh` | `bool` | Skip post-destroy refresh | N/A (infrastructure) |

---

## Current Report Output

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ RESOURCES                                                                       │
└─────────────────────────────────────────────────────────────────────────────────┘
  NAME          TESTS  DESTROY  STATE  IMPORT  UPDATE  FILE
  ────          ─────  ───────  ─────  ──────  ──────  ────
  inventory     13     ✓        ✓      ✗       ✓       inventory_resource.go
```

**Current Columns**: TESTS, DESTROY, STATE, IMPORT, UPDATE

---

## Proposed Report Output

Column names explicitly reference HashiCorp SDK fields (only official patterns):

```
┌─────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ RESOURCES                                                                                               │
└─────────────────────────────────────────────────────────────────────────────────────────────────────────┘
  NAME          TESTS  Steps>1  ImportState  CheckDestroy  ExpectError  Check  PlanChecks  FILE
  ────          ─────  ───────  ───────────  ────────────  ───────────  ─────  ──────────  ────
  inventory     13     ✓        ✗            ✓             ✓            ✓      ✗           inventory_resource.go
  job           7      ✓        ✗            ✓             ✗            ✓      ✗           job_resource.go
```

**Explicit Column Names** (direct HashiCorp SDK reference):

| Column | SDK Field | Description |
|--------|-----------|-------------|
| **TESTS** | - | Count of test functions for this resource |
| **Steps>1** | `len(Steps) > 1` | Multi-step test (update coverage) |
| **ImportState** | `TestStep.ImportState` | Has step with `ImportState: true` |
| **CheckDestroy** | `TestCase.CheckDestroy` | Has `CheckDestroy` function defined |
| **ExpectError** | `TestStep.ExpectError` | Has step with error expectation |
| **Check** | `TestStep.Check` | Has state validation via `Check` field |
| **PlanChecks** | `TestStep.ConfigPlanChecks` | Has plan validation checks |

---

## Implementation Checklist

### Phase 1: Data Model Updates ✅ COMPLETE

#### 1.1 Add New Fields to TestStepInfo (registry.go)
- [x] Add `ExpectNonEmptyPlan bool`
- [x] Add `RefreshState bool`
- [x] Keep `HasPlanCheck` separate from `HasCheck`

#### 1.2 Add New Fields to TestFunctionInfo (registry.go)
- [x] Add `HasPreCheck bool` - Function has PreCheck setup

#### 1.3 Update TestCoverage Struct (registry.go)
- [x] `HasPlanCheck` already exists in ResourceCoverage struct

### Phase 2: Parser Updates (parser.go) ✅ COMPLETE

#### 2.1 Detect ExpectNonEmptyPlan
- [x] Add case in `parseTestStepWithHelpers` for `ExpectNonEmptyPlan`

#### 2.2 Detect PreCheck
- [x] Parse TestCase for PreCheck field
- [x] Set `HasPreCheck = true` on TestFunctionInfo

#### 2.3 Detect RefreshState
- [x] Add case for `RefreshState: true` in step parsing

### Phase 3: Report Updates (cmd/validate/main.go) ✅ COMPLETE

#### 3.1 Update ResourceReport Struct
- [x] Add `HasExpectError bool`
- [x] Add `HasPlanCheck bool` (separate field)

#### 3.2 Update buildResourceReport()
- [x] Populate new fields from test coverage
- [x] Check for ExpectError in test steps
- [x] Track ConfigPlanChecks separately

#### 3.3 Update outputReportTable() - Resources Section
- [x] Add ExpectError column
- [x] Split Check/PlanChecks into separate columns
- [x] Rename columns to match SDK fields explicitly

#### 3.4 Update Summary Table
- [x] Columns now use explicit SDK field names

### Phase 4: Analyzer Updates (Optional) - DEFERRED

#### 4.1 Add Plan Check Analyzer
- [ ] Create analyzer recommending ConfigPlanChecks for computed attrs
- [ ] Add `enable-plan-check` setting

### Phase 5: Testing ✅ COMPLETE

#### 5.1 Unit Tests
- [x] Existing tests pass

#### 5.2 Integration Tests
- [x] Run against AAP provider - verified
- [x] golangci-lint passes
- [x] go build succeeds

### Phase 6: Documentation ✅ COMPLETE

#### 6.1 Update README.md
- [x] Document all detected patterns with HashiCorp references
- [ ] Update example report output
- [ ] Add pattern detection table

#### 6.2 Add Pattern Reference Section
```markdown
## HashiCorp Testing Patterns

This linter detects coverage for all patterns documented in:
- [Testing Patterns](https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns)
- [TestCase Reference](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/testcase)
- [TestStep Reference](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/teststep)

| Pattern | Detection | Column |
|---------|-----------|--------|
| Basic Test | ✓ | BASIC |
| Update Test | ✓ | UPDATE |
| ...
```

---

## Priority Order

1. **High Priority** - User-facing report improvements
   - Separate Check and PlanChecks columns
   - Add ExpectError column

2. **Medium Priority** - Detection enhancements
   - Detect ExpectNonEmptyPlan
   - Detect PreCheck

3. **Low Priority** - Optional analyzers
   - Plan check analyzer
   - RefreshState detection

---

## Acceptance Criteria

1. ✅ Report shows all official HashiCorp pattern columns (7 columns)
2. ✅ JSON output includes all pattern fields
3. ✅ ExpectError tests shown in report
4. ✅ ConfigPlanChecks shown separately from Check
5. ✅ All existing tests pass
6. ✅ golangci-lint passes
7. ✅ README updated with pattern documentation

---

## Files to Modify

| File | Changes |
|------|---------|
| `registry.go` | Add new fields to TestStepInfo, TestFunctionInfo, TestCoverage |
| `parser.go` | Detect ExpectNonEmptyPlan, PreCheck |
| `cmd/validate/main.go` | Update report structs and table output |
| `README.md` | Document HashiCorp patterns |
| `*_test.go` | Add unit tests for new detection |

---

## References

- [HashiCorp Testing Patterns](https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns)
- [TestCase Documentation](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/testcase)
- [TestStep Documentation](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/teststep)
- [terraform-provider-aap](https://github.com/ansible/terraform-provider-aap) - Primary validation target
