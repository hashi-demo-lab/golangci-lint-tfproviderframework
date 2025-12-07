# Terraform Provider Test Coverage Linter

A golangci-lint plugin that identifies test coverage gaps in Terraform providers built with terraform-plugin-framework.

## Features

This linter enforces HashiCorp's testing best practices by detecting:

1. **Basic Test Coverage**: Resources and data sources without acceptance tests
2. **Update Test Coverage**: Resources with updatable attributes lacking multi-step update tests
3. **Import Test Coverage**: Resources implementing `ImportState` without import tests
4. **Error Case Testing**: Resources with validation rules missing error case tests
5. **State Check Quality**: Test steps without proper state validation functions

## Installation

### Prerequisites

- Go 1.23.0 or higher
- golangci-lint v2.7.1 or higher

### Step 1: Install golangci-lint

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
golangci-lint --version  # Should be v2.7.1 or higher
```

### Step 2: Configure the Plugin

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

### Step 3: Create .custom-gcl.yml

This enables automatic plugin building:

```yaml
version: v2.7.1
plugins:
  - module: 'github.com/example/tfprovidertest'
    import: 'github.com/example/tfprovidertest'
    version: v1.0.0
```

## Usage

### Basic Usage

```bash
# Run all linters including tfprovidertest
golangci-lint run ./...

# Run only tfprovidertest
golangci-lint run --enable-only tfprovidertest ./...

# Run on specific directory
golangci-lint run --enable-only tfprovidertest ./internal/provider/
```

### Example Output

```
internal/provider/resource_widget.go:45:1: resource 'widget' has no acceptance test file [tfprovider-resource-basic-test]
internal/provider/resource_config.go:67:1: resource 'config' has updatable attributes but no update test coverage [tfprovider-resource-update-test]
internal/provider/resource_server.go:89:1: resource 'server' implements ImportState but has no import test coverage [tfprovider-resource-import-test]
```

## Linting Rules

### 1. tfprovider-resource-basic-test

**What it checks**: Every resource and data source has at least one acceptance test.

**Fix**: Create a test file with a `TestAcc*` function using `resource.Test()`.

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

### 2. tfprovider-resource-update-test

**What it checks**: Resources with updatable attributes have multi-step tests.

**Fix**: Add a test with multiple steps that modify configuration.

```go
func TestAccResourceConfig_update(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceConfigConfig("initial"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_config.test", "description", "initial"),
                ),
            },
            {
                Config: testAccResourceConfigConfig("updated"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_config.test", "description", "updated"),
                ),
            },
        },
    })
}
```

### 3. tfprovider-resource-import-test

**What it checks**: Resources implementing `ImportState` have import tests.

**Fix**: Add a test step with `ImportState: true` and `ImportStateVerify: true`.

```go
func TestAccResourceServer_import(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceServerConfig("test"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_server.test", "name", "test"),
                ),
            },
            {
                ResourceName:      "example_server.test",
                ImportState:       true,
                ImportStateVerify: true,
            },
        },
    })
}
```

### 4. tfprovider-test-error-cases

**What it checks**: Resources with validation rules have error case tests.

**Fix**: Add a test with `ExpectError`.

```go
func TestAccResourceNetwork_invalidConfig(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config:      testAccResourceNetworkConfig(""),
                ExpectError: regexp.MustCompile("name cannot be empty"),
            },
        },
    })
}
```

### 5. tfprovider-test-check-functions

**What it checks**: Test steps include state validation checks.

**Fix**: Add `Check` field with validation functions.

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

### Disable Specific Rules

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          enable-basic-test: true
          enable-update-test: true
          enable-import-test: false
          enable-error-test: false
          enable-state-check: false
```

### Custom Path Patterns

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          resource-path-pattern: "pkg/resources/*.go"
          data-source-path-pattern: "pkg/datasources/*.go"
          test-file-pattern: "tests/*_acceptance_test.go"
```

### Exclude Paths

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          exclude-paths:
            - "vendor/"
            - "internal/provider/generated/"
            - "**/*_generated.go"
```

### Test Matching Strategies

The linter uses three matching strategies to associate tests with resources (in priority order):

1. **Function Name Matching** (highest confidence): Extracts resource name from test function name
   - `TestAccWidget_basic` → matches `widget` resource
   - `TestAccDataSourceHTTP_basic` → matches `http` data source
   - `TestAccAWSInstance_update` → matches `instance` resource (strips provider prefix)

2. **File Proximity Matching** (medium confidence): Matches based on file naming conventions
   - `resource_widget_test.go` → matches `widget` resource
   - `data_source_http_test.go` → matches `http` data source
   - `widget_resource_test.go` → matches `widget` resource

3. **Fuzzy Matching** (optional, low confidence): Uses Levenshtein distance for approximate matches
   - Disabled by default to avoid false positives
   - Enable with `enable-fuzzy-matching: true`

### Custom Test Helpers

If your provider uses custom test helper functions that wrap `resource.Test()`:

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          custom-test-helpers:
            - "testhelper.AccTest"
            - "internal.RunAccTest"
```

### Custom Test Name Patterns

For non-standard test naming conventions:

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          test-name-patterns:
            - "TestAcc"
            - "TestResource"
            - "TestDataSource"
            - "TestIntegration"
```

### Verbose Diagnostics

Enable detailed output to understand why tests aren't matching:

```yaml
linters:
  settings:
    custom:
      tfprovidertest:
        settings:
          verbose: true
```

This shows:
- Test files searched
- Test functions found and their match status
- Expected naming patterns
- Suggested fixes

## CI/CD Integration

### GitHub Actions

```yaml
name: Lint
on: [push, pull_request]
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - name: Install golangci-lint
        run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
      - name: Run linters
        run: golangci-lint run --enable-only tfprovidertest ./...
```

## Performance

- **Small providers** (6 resources): <10 seconds
- **Medium providers** (50 resources): <60 seconds
- **Large providers** (500 resources): <5 minutes

### Optimizations

- **Unified Registry Caching**: Registry is built once and shared across all 5 analyzers (5x performance improvement)
- **Function-First Indexing**: Test functions are indexed upfront for O(1) lookups
- **AST-Only Analysis**: No type information needed, reducing memory overhead
- **Parallel Analyzer Execution**: All analyzers run concurrently
- **Unified Definitions Map**: Single map for all resources/data sources eliminates redundant merging

## Validation Results

This linter has been validated against 7 Terraform providers:

| Provider | Resources | Data Sources | Basic Coverage Issues | Performance |
|----------|-----------|--------------|----------------------|-------------|
| terraform-provider-time | 4 | 0 | 0 | 30.5ms |
| terraform-provider-http | 0 | 1 | 1* | 12.9ms |
| terraform-provider-tls | 6 | 2 | 6* | 48.0ms |
| terraform-provider-aap | 6 | 2 | 8* | 35.3ms |
| terraform-provider-hcp | 11 | 0 | 11* | 49.9ms |
| terraform-provider-helm | 1 | 1 | 0 | 17.2ms |
| terraform-provider-google-beta | 1,262 | 290 | 745* | ~2.2s |

*Issues may include false positives due to non-standard test naming conventions or conditionally skipped tests.

### Key Findings

1. **terraform-provider-time**: Zero basic coverage issues - all resources have acceptance tests
2. **terraform-provider-helm**: Complete coverage with 46+ test scenarios
3. **terraform-provider-google-beta**: Successfully analyzed 1,552 resources in ~2.2s (705 resources/sec)
4. **terraform-provider-http/tls**: Uses non-standard test naming (`TestDataSource_*` vs `TestAccDataSource*`)
5. **terraform-provider-hcp**: Tests exist but are conditionally skipped via `t.Skip()`

### Recommendations

For providers using non-standard naming:
- Configure `test-name-patterns` in settings
- Use `exclude-patterns` for base classes (`base_*.go`)
- Use `exclude-sweeper-files: true` to skip test infrastructure files

See `/workspace/validation/VALIDATION_REPORT.md` for detailed analysis.

## Reference Providers

Study these well-tested providers for examples:

- [terraform-provider-time](https://github.com/hashicorp/terraform-provider-time)
- [terraform-provider-tls](https://github.com/hashicorp/terraform-provider-tls)
- [terraform-provider-http](https://github.com/hashicorp/terraform-provider-http)

## Troubleshooting

### "unknown linter 'tfprovidertest'"

1. Verify `.custom-gcl.yml` exists in repository root
2. Run `golangci-lint cache clean`
3. Check module is accessible: `go get github.com/example/tfprovidertest@v1.0.0`

### False Positives

Adjust path patterns in `.golangci.yml` settings to match your project structure.

### Performance Issues

1. Run on specific directories instead of `./...`
2. Use `exclude-paths` to skip unnecessary files
3. Increase concurrency: `golangci-lint run --concurrency 4`

## Architecture

### Core Components

| Component | File | Description |
|-----------|------|-------------|
| **ResourceRegistry** | `registry.go` | Thread-safe storage for resources, data sources, and test functions |
| **Linker** | `linker.go` | Associates test functions with resources using matching strategies |
| **Parser** | `parser.go` | Extracts resources and test functions from Go AST |
| **Analyzers** | `analyzer.go` | Five go/analysis analyzers for different test coverage checks |
| **Settings** | `settings.go` | Configuration management with sensible defaults |
| **Utils** | `utils.go` | Shared utilities for name extraction and pattern matching |

### Key Design Decisions

1. **Unified Registry**: Single `definitions` map stores all resources/data sources, with filtered views for backward compatibility
2. **Function-First Indexing**: Test functions are indexed globally first, then linked to resources via the Linker
3. **Registry Caching**: `sync.Once` ensures registry is built only once per analysis pass, shared across all 5 analyzers
4. **Configurable Matching**: Function name → File proximity → Fuzzy (optional) matching chain

### Public API

```go
// Create a new registry
registry := tfprovidertest.NewResourceRegistry()

// Register resources
registry.RegisterResource(&tfprovidertest.ResourceInfo{...})

// Get all definitions (resources + data sources)
definitions := registry.GetAllDefinitions()

// Get resource or data source by name
info := registry.GetResourceOrDataSource("widget")

// Link tests to resources
linker := tfprovidertest.NewLinker(registry, settings)
linker.LinkTestsToResources()
```

## Contributing

1. Report issues at github.com/example/tfprovidertest/issues
2. Follow TDD practices (write tests first)
3. Run `go test ./...` before submitting PRs
4. Ensure `go fmt` and `go vet` pass

## License

[Add your license here]
