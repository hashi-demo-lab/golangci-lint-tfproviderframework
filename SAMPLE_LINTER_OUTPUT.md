# Sample Linter Output

## Standalone Validator Output

Running against HCP provider:

```
=== Validation Results for terraform-provider-hcp ===
Time: 51.144208ms

Resources found: 46
Data sources found: 22
Test files found: 68

⚠️  Found 13 potential issues:

1. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_twilio_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_twilio_deprecated.go

2. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_azure_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_azure_deprecated.go

3. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_aws_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_aws_deprecated.go

4. [tfprovider-resource-basic-test] Resource 'secret_manager' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultradar/secret_manager.go

5. [tfprovider-resource-basic-test] Resource 'type' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/packer/utils/resource_type.go

6. [tfprovider-resource-basic-test] Resource 'integration_subscription' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultradar/integration_subscription.go

7. [tfprovider-resource-basic-test] Resource 'radar_source' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultradar/radar_source.go

8. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_gcp_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_gcp_deprecated.go

9. [tfprovider-resource-basic-test] Resource 'integration_connection' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultradar/integration_connection.go

10. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_mongodbatlas_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_mongodbatlas_deprecated.go

11. [tfprovider-resource-basic-test] Resource 'vault_secrets_integration_confluent_deprecated' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/vaultsecrets/resource_vault_secrets_integration_confluent_deprecated.go

12. [tfprovider-datasource-basic-test] Data source 'source' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/packer/utils/base/data_source.go

13. [tfprovider-datasource-basic-test] Data source 'user_principal' has no acceptance test
   File: /workspace/validation/terraform-provider-hcp/internal/provider/iam/data_user_principal.go

=== Summary ===
Resources with tests: 35/46
Data sources with tests: 16/22
```

---

## Enhanced golangci-lint Output Format

The main module (`tfprovidertest.go`) produces enhanced diagnostics with location information:

### Example 1: Missing Test File

```
tfprovider-resource-basic-test: resource 'widget' has no acceptance test
  Resource: /path/to/internal/provider/resource_widget.go:45
  Expected test file: /path/to/internal/provider/resource_widget_test.go
  Expected test function: TestAccWidget_basic
```

### Example 2: Test File Without TestAcc Functions

```
tfprovider-resource-basic-test: resource 'gadget' has test file but no TestAcc functions
  Resource: /path/to/internal/provider/resource_gadget.go:89
  Test file: /path/to/internal/provider/resource_gadget_test.go
  Expected test function: TestAccGadget_basic
```

### Example 3: Data Source Missing Test

```
tfprovider-resource-basic-test: data source 'http' has no acceptance test
  Data source: /path/to/internal/provider/data_source_http.go:32
  Expected test file: /path/to/internal/provider/data_source_http_test.go
  Expected test function: TestAccDataSourceHttp_basic
```

---

## Verbose Mode Output

When `verbose: true` is set in `.golangci.yml`, the output includes detailed diagnostic information:

```
tfprovider-resource-basic-test: resource 'private_key' has no acceptance test

  Resource Location:
    resource: /path/to/internal/provider/resource_private_key.go:89

  Test Files Searched:
    - resource_private_key_test.go (found)

  Test Functions Found:
    - TestPrivateKeyRSA (line 45) - NOT MATCHED (missing 'Acc' prefix)
    - TestPrivateKeyECDSA (line 89) - NOT MATCHED (missing 'Acc' prefix)

  Why Not Matched:
    Expected pattern: TestAcc* or TestAccResource*
    Found pattern: TestPrivateKey* (non-standard)

  Suggested Fix:
    Option 1: Rename tests to follow convention (TestAccPrivateKey_basic)
    Option 2: Configure custom test patterns in .golangci.yml:
      test-name-patterns:
        - "TestPrivateKey"
```

---

## Clean Output (No Issues)

When all resources have proper test coverage:

```
=== Validation Results for terraform-provider-time ===
Time: 7.586833ms

Resources found: 4
Data sources found: 0
Test files found: 8

✅ No issues found - all resources have proper test coverage!
=== Summary ===
Resources with tests: 4/4
Data sources with tests: 0/0
```

---

## Configuration Example

To customize the linter behavior, add to `.golangci.yml`:

```yaml
linters-settings:
  custom:
    tfprovidertest:
      settings:
        # Analyzer toggles (all enabled by default)
        enable-basic-test: true       # Check every resource has TestAcc* function
        enable-update-test: true      # Check updatable attrs have multi-step tests
        enable-import-test: true      # Check ImportState resources have import tests
        enable-error-test: true       # Check validation rules have ExpectError tests
        enable-state-check: true      # Check test steps have Check functions

        # NEW: SDK detection (Phase 4)
        enable-sdk-detection: true    # Detect SDKv2 vs Plugin Framework
        warn-sdkv2-usage: true        # Warn about SDKv2 resources
        warn-sdkv2-tests: true        # Warn about SDKv2 test patterns

        # File exclusions (all default to true)
        exclude-base-classes: true
        exclude-sweeper-files: true
        exclude-migration-files: true

        # Test detection mode
        test-detection-mode: "signature"  # "signature" (Go-native) or "filename" (legacy)

        # Test naming patterns (used with filename mode)
        test-name-patterns:
          - "TestAcc"
          - "TestResource"
          - "TestDataSource"
          - "Test*_"

        # Enable file-based fallback matching
        enable-file-based-matching: true

        # Enable verbose diagnostic output
        verbose: false
```

---

## Analyzer Reference

| Analyzer | Setting | What It Checks |
|----------|---------|----------------|
| `tfprovider-resource-basic-test` | `enable-basic-test` | Every resource/data source has `TestAcc*` function with `resource.Test()` |
| `tfprovider-resource-update-test` | `enable-update-test` | Resources with `Optional` attributes (no `RequiresReplace`) have ≥2 test steps |
| `tfprovider-resource-import-test` | `enable-import-test` | Resources with `ImportState` method have test steps with `ImportState: true` |
| `tfprovider-test-error-cases` | `enable-error-test` | Resources with `Required`/`Validators` have test steps with `ExpectError` |
| `tfprovider-test-check-functions` | `enable-state-check` | Non-import/error test steps have `Check` field with `TestCheckResourceAttr` etc. |
| `tfprovider-sdk-detection` | `enable-sdk-detection` | Detects SDKv2 imports and warns about migration |
| `tfprovider-sdkv2-test-warning` | `warn-sdkv2-tests` | Detects legacy SDKv2 test patterns (`ComposeTestCheckFunc`, `VcrTest`) |

---

## SDKv2 Detection Output (New)

When `enable-sdk-detection: true` and `warn-sdkv2-tests: true`:

```
=== SDK Analysis for terraform-provider-google-beta ===

Files using SDKv2 (deprecated): 12
Files using Plugin Framework: 46
Mixed files (both SDKs): 3

⚠️ SDKv2 Resource Warnings:

1. resource 'dataflow_job' uses SDKv2
   Location: services/dataflow/resource_dataflow_job.go:45
   Detected imports: github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema
   Suggestion: Migrate to Plugin Framework for better type safety

⚠️ SDKv2 Test Warnings:

1. TestAccDataflowJob_basic uses SDKv2 test patterns
   Location: services/dataflow/resource_dataflow_job_test.go:89
   Detected patterns:
     - acctest.VcrTest() wrapper
     - resource.ComposeTestCheckFunc()
     - resource.TestCheckResourceAttr()
   Suggestion: Migrate to Plugin Framework test patterns:
     - Use ConfigStateChecks instead of Check
     - Use statecheck.ExpectKnownValue instead of TestCheckResourceAttr
     - Use ConfigPlanChecks for plan validation

📊 Summary:
   Plugin Framework resources: 46 (79%)
   SDKv2 resources: 12 (21%) - consider migration
   Plugin Framework tests: 312 (25%)
   SDKv2 tests: 933 (75%) - recommend migration
```
