# Feature Specification: Terraform Provider Test Coverage Linter

**Feature Branch**: `001-tfprovider-test-linter`
**Created**: 2025-12-07
**Status**: Draft
**Input**: User description: "Research and design a custom golangci-lint plugin that identifies gaps in test coverage for Terraform providers built with the terraform-plugin-framework. The linter should enforce best practice test patterns as defined in HashiCorp's testing patterns documentation."

## User Scenarios & Testing

### User Story 1 - Identify Untested Resources (Priority: P1)

As a Terraform provider maintainer, I need to identify resources and data sources that lack acceptance tests so that I can ensure basic functionality is verified before releasing new versions.

**Why this priority**: This is the foundation of test coverage. Without basic acceptance tests, providers cannot be trusted in production environments. This delivers immediate value by surfacing the most critical gaps.

**Independent Test**: Can be fully tested by running the linter on a provider with at least one resource that has no corresponding acceptance test file. The linter should report the missing test and deliver actionable feedback to create a TestAccResource function.

**Acceptance Scenarios**:

1. **Given** a Terraform provider with a resource schema defined in `internal/provider/resource_widget.go`, **When** the linter runs and no corresponding `internal/provider/resource_widget_test.go` file exists, **Then** the linter reports "tfprovider-resource-basic-test: resource 'widget' has no acceptance test file"

2. **Given** a Terraform provider with a resource test file `resource_widget_test.go`, **When** the file exists but contains no `TestAccResourceWidget*` function using `resource.Test()`, **Then** the linter reports "tfprovider-resource-basic-test: resource 'widget' has no basic acceptance test function"

3. **Given** a Terraform provider with a data source schema in `data_source_account.go`, **When** no corresponding acceptance test exists, **Then** the linter reports "tfprovider-datasource-basic-test: data source 'account' has no acceptance test"

4. **Given** a well-tested resource with proper acceptance tests, **When** the linter runs, **Then** no false positive warnings are generated for that resource

---

### User Story 2 - Enforce Update Test Coverage (Priority: P2)

As a Terraform provider maintainer, I need to verify that resources with updatable attributes have update tests so that I can ensure in-place updates work correctly without forcing resource replacement.

**Why this priority**: Update testing prevents critical bugs where configuration changes unexpectedly destroy and recreate infrastructure. This builds on P1 by adding depth to existing test coverage.

**Independent Test**: Can be fully tested by analyzing a resource schema for updatable attributes (those without `RequiresReplace: true` or `PlanModifiers` that force replacement) and checking if the test file contains multi-step tests that modify configuration. Delivers value by preventing update-related bugs.

**Acceptance Scenarios**:

1. **Given** a resource with updatable attributes like `description` or `tags`, **When** the acceptance test only has a single-step `TestStep`, **Then** the linter reports "tfprovider-resource-update-test: resource 'widget' has updatable attributes but no update test coverage"

2. **Given** a resource where all attributes use `RequiresReplace: true`, **When** the linter analyzes the schema, **Then** no update test warning is generated (updates not applicable)

3. **Given** a resource with multi-step acceptance tests that modify configurations in subsequent steps, **When** the linter runs, **Then** no warning is generated for missing update tests

---

### User Story 3 - Validate Import State Testing (Priority: P2)

As a Terraform provider maintainer, I need to ensure resources that implement ImportState have corresponding import tests so that users can safely import existing infrastructure into Terraform state.

**Why this priority**: Import functionality is critical for adopting Terraform in existing environments. Missing import tests can lead to state inconsistencies. This has equal priority to update testing as both prevent production issues.

**Independent Test**: Can be fully tested by detecting if a resource implements the `ImportState` method and checking if any `TestStep` includes `ImportState: true`. Delivers value by ensuring import parity with create operations.

**Acceptance Scenarios**:

1. **Given** a resource that implements the `ImportState` method, **When** no test step includes `ImportState: true` and `ImportStateVerify: true`, **Then** the linter reports "tfprovider-resource-import-test: resource 'widget' implements ImportState but has no import test coverage"

2. **Given** a resource with import tests using `ImportState: true` and `ImportStateVerify: true`, **When** the linter runs, **Then** no import test warning is generated

3. **Given** a resource that does not implement `ImportState`, **When** the linter analyzes the resource, **Then** no import test warning is generated

---

### User Story 4 - Enforce Error Case Testing (Priority: P3)

As a Terraform provider maintainer, I need to verify that resources have tests for invalid configurations so that users receive clear error messages when misusing the provider.

**Why this priority**: Error testing improves user experience but is less critical than functional correctness. This can be implemented after core test coverage is established.

**Independent Test**: Can be fully tested by checking if test files contain at least one `TestStep` with `ExpectError: regexp.MustCompile(...)`. Delivers value by ensuring validation logic is tested.

**Acceptance Scenarios**:

1. **Given** a resource with validation rules in the schema (Required fields, validators, ConflictsWith), **When** no test includes `ExpectError`, **Then** the linter reports "tfprovider-test-error-cases: resource 'widget' has validation rules but no error case tests"

2. **Given** a resource with error case tests using `ExpectError`, **When** the linter runs, **Then** no error case warning is generated

3. **Given** a simple resource with no validation rules, **When** the linter analyzes the resource, **Then** no error case warning is required (advisory only)

---

### User Story 5 - Validate Test Quality with State Checks (Priority: P3)

As a Terraform provider maintainer, I need to ensure acceptance tests use proper state check functions so that tests actually validate the resource behavior rather than just verifying apply succeeds.

**Why this priority**: Test quality is important but assumes tests already exist. This is an enhancement to existing tests rather than identifying missing coverage.

**Independent Test**: Can be fully tested by analyzing `TestStep` blocks to ensure they include `Check` fields with functions like `resource.TestCheckResourceAttr`, `resource.TestCheckResourceAttrSet`, etc. Delivers value by preventing weak tests that don't validate outcomes.

**Acceptance Scenarios**:

1. **Given** an acceptance test with `TestStep` blocks, **When** a step has no `Check` field or uses `resource.ComposeTestCheckFunc` with no check functions, **Then** the linter reports "tfprovider-test-check-functions: test step for resource 'widget' has no state validation checks"

2. **Given** a test using proper check functions like `resource.TestCheckResourceAttr(...)`, **When** the linter runs, **Then** no test quality warning is generated

---

### Edge Cases

- What happens when a resource file contains multiple resource type definitions (composite providers)?
  - The linter should analyze each resource schema independently and report coverage for each type

- How does the system handle resources in nested packages or non-standard directory structures?
  - The linter should support configurable path patterns to locate resource schemas and test files

- What happens when test files use non-standard naming conventions (e.g., `widget_acceptance_test.go` instead of `resource_widget_test.go`)?
  - The linter should support configurable regex patterns for matching test files to resource schemas

- How does the system handle generated code or vendored dependencies?
  - The linter should respect standard Go build tags and exclude patterns (e.g., `//go:generate`, vendor directories)

- What happens when analyzing providers with both terraform-plugin-framework and terraform-plugin-sdk resources?
  - The linter focuses only on framework resources; SDK resources are out of scope and should be ignored

- How does the system handle data sources that are read-only wrappers around APIs with no testable attributes?
  - The linter should still require basic acceptance tests but may allow reduced check function coverage for data sources

- What happens when running the linter on providers with thousands of resources (large-scale validation)?
  - The linter should support parallel analysis and provide performance metrics to ensure it completes in reasonable time (under 5 minutes for 500+ resources)

## Requirements

### Functional Requirements

- **FR-001**: Linter MUST detect resources defined using terraform-plugin-framework that lack corresponding acceptance test files
- **FR-002**: Linter MUST identify resources with acceptance test files that do not contain `TestAccResource*` functions using `resource.Test()`
- **FR-003**: Linter MUST detect data sources defined using terraform-plugin-framework that lack corresponding acceptance test functions
- **FR-004**: Linter MUST analyze resource schemas to identify updatable attributes (those without RequiresReplace or replacement-forcing PlanModifiers)
- **FR-005**: Linter MUST detect resources with updatable attributes that lack multi-step update tests
- **FR-006**: Linter MUST identify resources implementing `ImportState` method that lack import test coverage using `ImportState: true` and `ImportStateVerify: true`
- **FR-007**: Linter MUST detect resources with schema validation rules that lack error case tests using `ExpectError`
- **FR-008**: Linter MUST identify acceptance tests with `TestStep` blocks that lack `Check` fields with state validation functions
- **FR-009**: Linter MUST ignore resources defined using terraform-plugin-sdk (v2 or earlier) as they are out of scope
- **FR-010**: Linter MUST provide actionable error messages that include the resource name, missing test type, and guidance on how to add the test
- **FR-011**: Linter MUST support configuration via `.golangci.yml` to enable/disable specific rules
- **FR-012**: Linter MUST integrate as a golangci-lint plugin module that can be loaded via the plugin system
- **FR-013**: Linter MUST respect Go build tags and exclude vendored dependencies from analysis
- **FR-014**: Linter MUST complete analysis of providers with up to 500 resources within 5 minutes on standard development hardware
- **FR-015**: Linter MUST support configurable path patterns for locating resource schemas and test files to handle non-standard project structures
- **FR-016**: Linter MUST support configurable regex patterns for matching test file names to resource types
- **FR-017**: Linter MUST generate zero false positives when analyzing well-tested resources in the target validation providers (terraform-provider-http, terraform-provider-tls, terraform-provider-time, terraform-provider-aap)
- **FR-018**: Each linting rule MUST have corresponding test cases following TDD practices

### Key Entities

- **Resource Schema**: Represents a Terraform resource definition created using terraform-plugin-framework schema.Resource interface, containing attributes, validators, and lifecycle hooks
- **Data Source Schema**: Represents a Terraform data source definition created using terraform-plugin-framework schema.DataSource interface, containing read-only attributes
- **Acceptance Test**: A Go test function using terraform-plugin-testing's resource.Test() framework, containing one or more TestStep configurations
- **TestStep**: A single step in an acceptance test defining Terraform configuration, state checks, and test behavior (import, error expectations, etc.)
- **Updatable Attribute**: A resource schema attribute that can be changed without forcing resource replacement, identified by absence of RequiresReplace or replacement-forcing PlanModifiers
- **Linting Rule**: A specific check enforcing a testing pattern (e.g., tfprovider-resource-basic-test, tfprovider-resource-update-test)
- **Linter Configuration**: Settings in `.golangci.yml` that enable/disable rules and configure path patterns for analysis
- **State Check Function**: Functions from terraform-plugin-testing like resource.TestCheckResourceAttr that validate Terraform state after apply/plan operations

## Success Criteria

### Measurable Outcomes

- **SC-001**: Linter correctly identifies 100% of untested resources in validation target providers (resources lacking acceptance test files or functions)
- **SC-002**: Linter generates zero false positives when run against well-tested resources in terraform-provider-http, terraform-provider-tls, terraform-provider-time, and terraform-provider-aap
- **SC-003**: Linter completes analysis of terraform-provider-time (approximately 6 resources and 3 data sources) in under 10 seconds
- **SC-004**: Linter completes analysis of providers with 500+ resources within 5 minutes on hardware with 4 CPU cores and 8GB RAM
- **SC-005**: All linting rules have corresponding test cases with 100% coverage of rule logic (TDD compliance verified)
- **SC-006**: Linter integrates successfully with golangci-lint v1.50+ and can be loaded as a plugin module
- **SC-007**: 95% of linter error messages include actionable guidance that developers can use to add missing tests without additional documentation lookup
- **SC-008**: Linter correctly distinguishes between terraform-plugin-framework and terraform-plugin-sdk resources, analyzing only framework-based code
- **SC-009**: Users can enable/disable individual linting rules via .golangci.yml configuration
- **SC-010**: Linter identifies at least 3 real test coverage gaps in terraform-provider-aap (community provider baseline validation)

## Assumptions

- HashiCorp's testing patterns documentation at https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns remains the authoritative source for best practices
- Target providers use standard Go project structure with resource schemas in `internal/provider` or similar conventional paths
- Acceptance tests follow HashiCorp's naming convention of `TestAccResource*` and `TestAccDataSource*` function prefixes
- The golangci-lint plugin API remains stable across v1.50+ versions
- Development and testing will occur on systems with Go 1.21+ installed
- Test coverage analysis focuses on acceptance tests only; unit tests for provider business logic are out of scope
- Providers using terraform-plugin-framework v1.16.1+ are the primary target; earlier framework versions are best-effort support
- The linter will analyze Go AST (Abstract Syntax Tree) and source code structure but will not execute tests or require compiled binaries
- Maintainers running the linter have golangci-lint already installed and configured in their CI/CD pipelines

## Dependencies

- golangci-lint v1.50 or higher must be available as the plugin host
- Target providers must be using terraform-plugin-framework (not terraform-plugin-sdk)
- terraform-plugin-testing library v1.13.3+ must be available in analyzed provider codebases
- Example plugin (github.com/golangci/example-plugin-module-linter) available as architectural reference
- Access to HashiCorp testing patterns documentation for validation of patterns
- Access to target provider repositories for validation testing:
  - github.com/hashicorp/terraform-provider-http
  - github.com/hashicorp/terraform-provider-tls
  - github.com/hashicorp/terraform-provider-time
  - github.com/ansible/terraform-provider-aap

## Scope Boundaries

### In Scope

- Linting resource and data source test coverage for terraform-plugin-framework providers
- Detecting missing acceptance tests (basic, update, import, error cases)
- Validating test quality (state check functions)
- Integration as golangci-lint plugin module
- Configuration via .golangci.yml
- Support for standard and configurable project structures
- TDD-based rule implementation with comprehensive test coverage
- Validation against 4 target providers (3 HashiCorp official, 1 community)
- Performance optimization for large providers (500+ resources)
- Actionable error messages with remediation guidance

### Out of Scope

- Linting terraform-plugin-sdk (v2) based providers
- Validating provider business logic or implementation correctness
- Testing the tests themselves (mock validation, test execution)
- Linting Terraform configuration syntax (.tf files)
- Analyzing provider performance or efficiency
- Generating missing tests automatically (code generation)
- Integration with test coverage tools like go test -cover
- Validating HCL (HashiCorp Configuration Language) syntax in test configurations
- Supporting provider development frameworks other than terraform-plugin-framework
- Enforcing code style or formatting (defer to existing golangci-lint rules)
- Regression test detection beyond advisory checks (non-blocking recommendations)
- Provider documentation linting or validation
- Schema migration testing between provider versions

## Research Requirements

Before implementation planning, the following research must be completed:

1. **HashiCorp Testing Patterns Analysis**: Review the complete testing patterns documentation at https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns to extract all testable patterns beyond the 5 core patterns identified in objectives

2. **golangci-lint Plugin Architecture**: Study the example-plugin-module-linter implementation to understand:
   - Plugin interface contracts and required methods
   - AST analysis patterns for Go code
   - Error reporting and configuration integration
   - Plugin lifecycle and initialization

3. **Terraform Plugin Framework Schema Analysis**: Investigate how to programmatically detect:
   - Resource vs data source schemas
   - Updatable vs replacement-forcing attributes
   - ImportState implementation patterns
   - Schema validation rules and validators

4. **Target Provider Baseline**: Establish current test coverage baseline for all 4 target providers:
   - Count total resources and data sources
   - Count existing acceptance tests
   - Identify gaps the linter should detect
   - Document expected false positive scenarios (if any)

5. **Performance Constraints**: Benchmark Go AST analysis performance on large codebases to determine:
   - Expected analysis time per resource/data source
   - Memory requirements for large providers
   - Opportunities for parallel analysis

## Notes

- This specification focuses exclusively on terraform-plugin-framework; plugin-sdk support is explicitly excluded to maintain focused scope
- The linter is a static analysis tool and will not execute tests or require running providers
- Following TDD practices means writing test cases for each linting rule before implementing the rule logic
- The 3-clarification limit was not triggered; all requirements are specified with reasonable industry-standard defaults
- Validation against 4 diverse providers ensures the linter handles both HashiCorp official patterns and community provider variations
