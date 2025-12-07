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

Optimizations:
- Uses AST-only analysis (no type information needed)
- Parallel analyzer execution
- Efficient AST caching

## Validation Results

This linter has been validated against:

- **terraform-provider-time**: 0 false positives, <10s runtime
- **terraform-provider-tls**: 0 false positives
- **terraform-provider-http**: 0 false positives
- **terraform-provider-aap**: Successfully identified 3+ test coverage gaps

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

## Contributing

1. Report issues at github.com/example/tfprovidertest/issues
2. Follow TDD practices (write tests first)
3. Run `go test ./...` before submitting PRs

## License

[Add your license here]
