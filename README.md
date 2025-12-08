# Terraform Provider Test Coverage Linter

A comprehensive test coverage analysis tool for Terraform providers built with terraform-plugin-framework. Supports resources, data sources, and **actions** (terraform-plugin-framework ephemeral resources).

## Features

This linter enforces HashiCorp's testing best practices by detecting:

1. **Basic Test Coverage**: Resources, data sources, and actions without acceptance tests
2. **Update Test Coverage**: Resources with updatable attributes lacking multi-step update tests
3. **Import Test Coverage**: Resources implementing `ImportState` without import tests
4. **Error Case Testing**: Resources with validation rules missing error case tests
5. **State Check Quality**: Test steps without proper state validation functions
6. **Action Support**: Full support for terraform-plugin-framework actions (ephemeral resources)

## Quick Start

### Standalone CLI Usage

```bash
# Build the CLI
go build ./cmd/validate

# Run basic analysis
./validate -provider /path/to/terraform-provider-example

# Generate comprehensive coverage report with tables
./validate -provider /path/to/terraform-provider-example -report

# Export report as JSON
./validate -provider /path/to/terraform-provider-example -report -format json
```

### Example Report Output

```
╔════════════════════════════════════════════════════════════════════════════════╗
║                        TERRAFORM PROVIDER TEST COVERAGE REPORT                 ║
╚════════════════════════════════════════════════════════════════════════════════╝

┌─────────────────────────────────────────────────────────────────────────────────┐
│ SUMMARY                                                                         │
├──────────────┬───────┬──────────┬─────────────────────────────────────────────────┤
│ Category     │ Total │ Untested │ Issues                                          │
├──────────────┼───────┼──────────┼─────────────────────────────────────────────────┤
│ Resources    │     5 │        0 │ 1 missing CheckDestroy                          │
│ Data Sources │     5 │        0 │ -                                               │
│ Actions      │     3 │        0 │ 2 missing state/plan checks                     │
│ Orphan Tests │     0 │        - │ -                                               │
└──────────────┴───────┴──────────┴─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────────┐
│ RESOURCES                                                                       │
└─────────────────────────────────────────────────────────────────────────────────┘
  NAME          TESTS  DESTROY  STATE  IMPORT  UPDATE  FILE
  ────          ─────  ───────  ─────  ──────  ──────  ────
  group         2      ✓        ✓      ✗       ✓       group_resource.go
  host          2      ✓        ✓      ✗       ✓       host_resource.go
  inventory     13     ✓        ✓      ✗       ✓       inventory_resource.go
```

## Installation

### Prerequisites

- Go 1.23.0 or higher
- golangci-lint v2.7.1 or higher (for plugin mode)

### Option 1: Standalone CLI

```bash
# Clone and build
git clone https://github.com/example/tfprovidertest
cd tfprovidertest
go build ./cmd/validate

# Run against your provider
./validate -provider /path/to/your/provider -report
```

### Option 2: golangci-lint Plugin

Create `.golangci.yml` in your provider repository:

```yaml
version: "2"

linters:
  enable:
    - tfprovidertest
  settings:
    custom:
      tfprovidertest:
        type: module
        description: Terraform provider test coverage linter
        original-url: github.com/example/tfprovidertest
        settings:
          enable-basic-test: true
          enable-update-test: true
          enable-import-test: true
          enable-error-test: true
          enable-state-check: true
```

Create `.custom-gcl.yml` for automatic plugin building:

```yaml
version: v2.7.1
plugins:
  - module: 'github.com/example/tfprovidertest'
    import: 'github.com/example/tfprovidertest'
    version: v1.0.0
```

## CLI Reference

### Basic Commands

```bash
# Run standard analysis (issues only)
./validate -provider /path/to/provider

# Generate comprehensive coverage report
./validate -provider /path/to/provider -report

# JSON output for CI/CD integration
./validate -provider /path/to/provider -report -format json

# Verbose output with diagnostics
./validate -provider /path/to/provider -verbose
```

### Diagnostic Commands

```bash
# Show all resource -> test function associations
./validate -provider /path/to/provider -show-matches

# Show test functions without resource association (orphans)
./validate -provider /path/to/provider -show-unmatched

# Show resources without any test coverage
./validate -provider /path/to/provider -show-orphaned
```

### Matching Options

```bash
# Use specific matching strategy
./validate -provider /path/to/provider -match-strategy function
./validate -provider /path/to/provider -match-strategy file
./validate -provider /path/to/provider -match-strategy fuzzy

# Set confidence threshold for fuzzy matching
./validate -provider /path/to/provider -match-strategy fuzzy -confidence-threshold 0.8

# Specify provider prefix for function name extraction
./validate -provider /path/to/provider -provider-prefix AWS
```

## Test Matching Strategies

The linter uses multiple strategies to associate tests with resources (in priority order):

### 1. Inferred from Config (Highest Confidence)

Parses HCL configuration strings in test steps to identify which resources are being tested:

```go
Config: `
  resource "example_widget" "test" {
    name = "test"
  }
`
```

This matches the test to the `widget` resource with 100% confidence.

### 2. Function Name Matching

Extracts resource name from test function name patterns:

- `TestAccWidget_basic` → matches `widget` resource
- `TestAccAWSInstance_update` → matches `instance` resource (strips provider prefix)
- `TestAccAAPJobAction_basic` → matches `job_launch` action (handles action suffixes)

### 3. File Proximity Matching

Matches based on file naming conventions:

- `resource_widget_test.go` → matches `widget` resource
- `widget_resource_test.go` → matches `widget` resource
- `job_launch_action_test.go` → matches `job_launch` action

### 4. Fuzzy Matching (Optional)

Uses Levenshtein distance for approximate matches. Disabled by default to avoid false positives.

## Linting Rules

### tfprovider-resource-basic-test

**What it checks**: Every resource, data source, and action has at least one acceptance test.

**Fix**: Create a test file with a `TestAcc*` function:

```go
func TestAccResourceWidget_basic(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceWidgetConfig("example"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_widget.test", "name", "example"),
                ),
            },
        },
    })
}
```

### tfprovider-resource-update-test

**What it checks**: Resources with updatable attributes have multi-step tests.

**Fix**: Add a test with multiple steps that modify configuration:

```go
func TestAccResourceConfig_update(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceConfigConfig("initial"),
                Check:  resource.TestCheckResourceAttr("example_config.test", "value", "initial"),
            },
            {
                Config: testAccResourceConfigConfig("updated"),
                Check:  resource.TestCheckResourceAttr("example_config.test", "value", "updated"),
            },
        },
    })
}
```

### tfprovider-resource-import-test

**What it checks**: Resources implementing `ImportState` have import tests.

**Fix**: Add a test step with `ImportState: true`:

```go
{
    ResourceName:      "example_server.test",
    ImportState:       true,
    ImportStateVerify: true,
}
```

### tfprovider-test-error-cases

**What it checks**: Resources with validation rules have error case tests.

**Fix**: Add a test with `ExpectError`:

```go
{
    Config:      testAccResourceNetworkConfig(""),
    ExpectError: regexp.MustCompile("name cannot be empty"),
}
```

### tfprovider-test-check-functions

**What it checks**: Test steps include state validation checks.

**Fix**: Add `Check` field with validation functions:

```go
{
    Config: testAccResourceDatabaseConfig("test"),
    Check: resource.ComposeTestCheckFunc(
        resource.TestCheckResourceAttr("example_database.test", "name", "test"),
        resource.TestCheckResourceAttrSet("example_database.test", "id"),
    ),
}
```

## Configuration

### Settings Reference

| Setting | Default | Description |
|---------|---------|-------------|
| `enable-basic-test` | `true` | Check for basic acceptance test coverage |
| `enable-update-test` | `true` | Check for update test coverage |
| `enable-import-test` | `true` | Check for import test coverage |
| `enable-error-test` | `true` | Check for error case test coverage |
| `enable-state-check` | `true` | Check for state validation in tests |
| `enable-fuzzy-matching` | `false` | Enable fuzzy string matching |
| `fuzzy-match-threshold` | `0.7` | Minimum similarity for fuzzy matches |
| `exclude-base-classes` | `true` | Exclude `base_*.go` helper files |
| `exclude-sweeper-files` | `true` | Exclude `*_sweeper.go` test infrastructure |
| `exclude-migration-files` | `true` | Exclude state migration files |
| `verbose` | `false` | Enable detailed diagnostic output |

### Exclude Patterns

```yaml
settings:
  exclude-paths:
    - "vendor/"
    - "internal/provider/generated/"
    - "**/*_generated.go"
```

### Custom Test Helpers

```yaml
settings:
  custom-test-helpers:
    - "testhelper.AccTest"
    - "internal.RunAccTest"
```

## Action Support

The linter fully supports terraform-plugin-framework **actions** (ephemeral resources):

### Detection

Actions are detected via:
- `action.Action` interface implementation
- Factory functions like `NewJobLaunchAction()`
- TypeName extraction from `Metadata()` method

### Test Matching

Action tests are matched using the same strategies as resources:

```go
// Matched via Config parsing
func TestAccJobAction_basic(t *testing.T) {
    resource.Test(t, resource.TestCase{
        Steps: []resource.TestStep{
            {
                Config: `
                    action "example_job_launch" "test" {
                        job_template_id = 1
                    }
                `,
            },
        },
    })
}
```

### Action Lifecycle Suffixes

The linter handles action test naming conventions with lifecycle suffixes:

- `TestAccEDAEventStreamAfterCreateAction` → `eda_eventstream_post` action
- `TestAccJobActionBeforeUpdate` → `job_launch` action

## Architecture

### Core Components

| Component | File | Description |
|-----------|------|-------------|
| **ResourceRegistry** | `registry.go` | Thread-safe storage with compound keys (`resource:name`, `action:name`) |
| **Linker** | `linker.go` | Multi-strategy test-to-resource association |
| **Parser** | `parser.go` | AST-based extraction of resources, data sources, actions, and tests |
| **Analyzers** | `analyzer.go` | Five go/analysis analyzers for coverage checks |
| **Settings** | `settings.go` | Configuration with sensible defaults |

### Registry Key Format

Resources are stored with compound keys to avoid collisions:

- `resource:widget` - Resource named "widget"
- `data source:widget` - Data source named "widget"
- `action:job_launch` - Action named "job_launch"

### Public API

```go
// Create a new registry
registry := tfprovidertest.NewResourceRegistry()

// Register resources
registry.RegisterResource(&tfprovidertest.ResourceInfo{
    Name:     "widget",
    Kind:     tfprovidertest.KindResource,
    FilePath: "widget_resource.go",
})

// Get all definitions
definitions := registry.GetAllDefinitions()

// Link tests to resources
linker := tfprovidertest.NewLinker(registry, settings)
linker.LinkTestsToResources()

// Get tests for a resource
tests := registry.GetResourceTests("resource:widget")
```

## Validation Results

Validated against the AAP (Ansible Automation Platform) provider:

| Category | Total | Tested | Coverage |
|----------|-------|--------|----------|
| Resources | 5 | 5 | 100% |
| Data Sources | 5 | 5 | 100% |
| Actions | 3 | 3 | 100% |
| Orphan Tests | 0 | - | - |

### Detailed Resource Coverage

| Resource | Tests | CheckDestroy | State Check | Import | Update |
|----------|-------|--------------|-------------|--------|--------|
| group | 2 | ✓ | ✓ | ✗ | ✓ |
| host | 2 | ✓ | ✓ | ✗ | ✓ |
| inventory | 13 | ✓ | ✓ | ✗ | ✓ |
| job | 7 | ✓ | ✓ | ✗ | ✓ |
| workflow_job | 6 | ✗ | ✓ | ✗ | ✓ |

### Action Coverage

| Action | Tests | State Check |
|--------|-------|-------------|
| eda_eventstream_post | 3 | ✓ |
| job_launch | 3 | ✗ |
| workflow_job_launch | 3 | ✗ |

## Performance

- **Small providers** (5-10 resources): <100ms
- **Medium providers** (50 resources): <1s
- **Large providers** (500+ resources): <5s

### Optimizations

- **Unified Registry Caching**: Registry built once, shared across all analyzers
- **Config Parsing**: HCL patterns extracted from test Config strings
- **Helper Function Scanning**: Patterns extracted from helper function return values
- **Parallel Analysis**: All analyzers run concurrently

## CI/CD Integration

### GitHub Actions

```yaml
name: Test Coverage
on: [push, pull_request]
jobs:
  coverage:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - name: Build linter
        run: go build ./cmd/validate
      - name: Check test coverage
        run: ./validate -provider . -report
```

### JSON Output for CI

```bash
./validate -provider . -report -format json | jq '.summary'
```

## Troubleshooting

### "Base classes showing as untested"

Base class files (`base_*.go`) are excluded by default. If you see them in reports, ensure `exclude-base-classes: true` is set.

### "Action not detected"

Actions must implement the `action.Action` interface and have a factory function like `NewXxxAction()`. The TypeName is extracted from the `Metadata()` method.

### "Test not matched to resource"

1. Check the test's `Config` contains the resource type (e.g., `resource "example_widget" "test"`)
2. Verify function name follows conventions (e.g., `TestAccWidget_basic`)
3. Run with `-verbose` to see matching details

### "False positives for generated code"

Add to exclude paths:
```yaml
exclude-paths:
  - "**/*_generated.go"
  - "internal/generated/"
```

## Contributing

1. Report issues at github.com/example/tfprovidertest/issues
2. Follow TDD practices
3. Run `go test ./...` and `golangci-lint run` before PRs
4. Update README for new features

## License

Apache 2.0
