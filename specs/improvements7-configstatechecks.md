# Improvements 7: ConfigStateChecks Detection

**Created:** 2025-12-08 01:13:08 UTC
**Completed:** 2025-12-08 01:20:00 UTC
**Status:** IMPLEMENTED
**Priority:** High

## Summary

The linter currently only detects the legacy `Check:` field in TestStep configurations for state verification. HashiCorp has introduced a newer `ConfigStateChecks:` field that uses the `statecheck.StateCheck` interface, which is the recommended approach for newer providers.

This causes false negatives in the report - tests using `ConfigStateChecks` are reported as missing state verification when they actually have it.

## Problem Statement

### Current Behavior

The BCM provider data source tests use `ConfigStateChecks`:

```go
Steps: []resource.TestStep{
    {
        Config: testAccCMDeviceCategoriesDataSourceConfig(),
        ConfigStateChecks: []statecheck.StateCheck{
            statecheck.ExpectKnownValue(
                "data.bcm_cmdevice_categories.test",
                tfjsonpath.New("id"),
                knownvalue.NotNull(),
            ),
        },
    },
}
```

But the linter reports these as missing Check (✗) because it only looks for the legacy pattern:

```go
Check: resource.ComposeTestCheckFunc(
    resource.TestCheckResourceAttr(...),
)
```

### Affected Report Output

```
DATA SOURCES
  NAME                   TESTS  Check  FILE
  cmdevice_categories    3      ✗      data_source_cmdevice_categories.go   <- FALSE NEGATIVE
  cmpart_entity_info     11     ✓      data_source_cmpart_entity_info.go    <- Has both patterns
```

## Requirements

### Phase 1: Parser Updates

- [x] Add detection for `ConfigStateChecks` field in TestStep
- [x] Add `HasConfigStateChecks bool` to `TestStepInfo` struct
- [x] Update `HasStateOrPlanCheck()` method to include ConfigStateChecks
- [x] Also added detection for `ConfigPlanChecks` field

### Phase 2: Report Updates

- [x] Update "Check" column to show ✓ if either `Check` or `ConfigStateChecks` is present
- [x] Update `ResourceCoverage.HasStateCheck` to include ConfigStateChecks
- [x] Summary counts now correctly account for ConfigStateChecks

### Phase 3: Testing

- [x] Verify BCM provider data sources now show ✓ for Check column (all 9 data sources)
- [x] Verify AAP provider still works correctly (all 5 data sources)
- [x] Run full reports against both providers - all passing

## Technical Details

### HashiCorp StateCheck Interface

The `ConfigStateChecks` field accepts `[]statecheck.StateCheck` which includes:

- `statecheck.ExpectKnownValue()` - Verify attribute has expected value
- `statecheck.ExpectKnownOutputValue()` - Verify output has expected value
- `statecheck.ExpectSensitiveValue()` - Verify attribute is marked sensitive
- `statecheck.ExpectNullValue()` - Verify attribute is null
- `statecheck.CompareValue()` - Compare values between steps

### Files to Modify

1. `/workspace/registry.go` - Add `HasConfigStateChecks` to TestStepInfo
2. `/workspace/parser.go` - Add detection logic for ConfigStateChecks
3. `/workspace/cmd/validate/main.go` - Update report generation (if needed)

## Expected Outcome

After implementation, the BCM provider report should show:

```
DATA SOURCES
  NAME                   TESTS  Check  FILE
  cmdevice_categories    3      ✓      data_source_cmdevice_categories.go   <- CORRECT
  cmpart_entity_info     11     ✓      data_source_cmpart_entity_info.go
```

## References

- [HashiCorp Testing Framework - State Checks](https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/state-checks)
- [terraform-plugin-testing statecheck package](https://pkg.go.dev/github.com/hashicorp/terraform-plugin-testing/statecheck)
