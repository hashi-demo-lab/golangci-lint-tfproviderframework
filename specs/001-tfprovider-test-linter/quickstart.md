# Quickstart Guide: Terraform Provider Test Coverage Linter

**Version**: 1.0.0
**Target Audience**: Terraform provider maintainers
**Prerequisites**: Go 1.23.0+, golangci-lint v2.7.1+

---

## Installation

### Step 1: Install golangci-lint

```bash
# Install latest golangci-lint (v2.7.1+)
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Verify installation
golangci-lint --version
# Expected: golangci-lint has version 2.7.1 or higher
```

### Step 2: Configure Plugin

Create `.golangci.yml` in your provider repository root:

```yaml
version: "2"

linters:
  default: none
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

---

## First Run

### Basic Usage

```bash
# Run the linter on your provider
golangci-lint run ./...

# Run only tfprovidertest linter
golangci-lint run --enable-only tfprovidertest ./...

# Run on specific directory
golangci-lint run --enable-only tfprovidertest ./internal/provider/
```

### Expected Output

```
internal/provider/resource_widget.go:45:1: resource 'widget' has no acceptance test file [tfprovider-resource-basic-test]
internal/provider/resource_config.go:67:1: resource 'config' has updatable attributes but no update test coverage [tfprovider-resource-update-test]
internal/provider/resource_server.go:89:1: resource 'server' implements ImportState but has no import test coverage [tfprovider-resource-import-test]
```

---

## Understanding Diagnostics

### Diagnostic Format

Each diagnostic includes:
- **File and line**: Where the issue is detected
- **Message**: What's missing or incorrect
- **Category**: Which linting rule triggered (in brackets)

### The 5 Linting Rules

#### 1. tfprovider-resource-basic-test

**What it checks**: Every resource/data source has at least one acceptance test

**Example violation**:
```
resource 'widget' has no acceptance test file
```

**How to fix**:
```go
// Create internal/provider/resource_widget_test.go
package provider

import (
    "testing"
    "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

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

func testAccResourceWidgetConfig(name string) string {
    return fmt.Sprintf(`
resource "example_widget" "test" {
  name = %[1]q
}
`, name)
}
```

#### 2. tfprovider-resource-update-test

**What it checks**: Resources with updatable attributes have multi-step tests

**Example violation**:
```
resource 'config' has updatable attributes but no update test coverage
```

**How to fix**:
```go
func TestAccResourceConfig_update(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceConfigConfig("initial", "Initial description"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_config.test", "description", "Initial description"),
                ),
            },
            {
                Config: testAccResourceConfigConfig("initial", "Updated description"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_config.test", "description", "Updated description"),
                ),
            },
        },
    })
}
```

#### 3. tfprovider-resource-import-test

**What it checks**: Resources implementing ImportState have import tests

**Example violation**:
```
resource 'server' implements ImportState but has no import test coverage
```

**How to fix**:
```go
func TestAccResourceServer_import(t *testing.T) {
    resource.Test(t, resource.TestCase{
        ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
        Steps: []resource.TestStep{
            {
                Config: testAccResourceServerConfig("test-server"),
                Check: resource.ComposeTestCheckFunc(
                    resource.TestCheckResourceAttr("example_server.test", "name", "test-server"),
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

#### 4. tfprovider-test-error-cases

**What it checks**: Resources with validation rules have error case tests

**Example violation**:
```
resource 'network' has validation rules but no error case tests
```

**How to fix**:
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

#### 5. tfprovider-test-check-functions

**What it checks**: Test steps include state validation checks

**Example violation**:
```
test step for resource 'database' has no state validation checks
```

**How to fix**:
```go
// BAD: No Check field
{
    Config: testAccResourceDatabaseConfig("test"),
}

// GOOD: Include Check with validation functions
{
    Config: testAccResourceDatabaseConfig("test"),
    Check: resource.ComposeTestCheckFunc(
        resource.TestCheckResourceAttr("example_database.test", "name", "test"),
        resource.TestCheckResourceAttrSet("example_database.test", "id"),
    ),
}
```

---

## Configuration Examples

### Disable Specific Rules

Only check for basic tests and update tests:

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

For non-standard project structures:

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

### Exclude Generated Code

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

---

## Integration with CI/CD

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
        run: |
          go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

      - name: Run linters
        run: golangci-lint run --enable-only tfprovidertest ./...
```

### GitLab CI

```yaml
lint:
  image: golang:1.23
  script:
    - go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
    - golangci-lint run --enable-only tfprovidertest ./...
```

---

## Troubleshooting

### "unknown linter 'tfprovidertest'"

**Cause**: Plugin not installed or .custom-gcl.yml missing

**Solution**:
1. Verify .custom-gcl.yml exists in repository root
2. Check module path is accessible: `go get github.com/example/tfprovidertest@v1.0.0`
3. Run `golangci-lint cache clean`

### "no linters enabled"

**Cause**: .golangci.yml doesn't enable tfprovidertest

**Solution**:
```yaml
linters:
  enable:
    - tfprovidertest  # Add this line
```

### False Positives

**Cause**: Non-standard naming or structure

**Solution**: Adjust path patterns in settings:
```yaml
settings:
  resource-path-pattern: "your/custom/pattern/*.go"
  test-file-pattern: "your/test/pattern/*_test.go"
```

### Performance Issues

**Cause**: Large provider codebase (500+ resources)

**Solution**:
1. Run on specific directories: `golangci-lint run ./internal/provider/`
2. Exclude unnecessary paths via `exclude-paths`
3. Use parallel execution: `golangci-lint run --concurrency 4`

---

## Next Steps

1. **Run the linter**: `golangci-lint run --enable-only tfprovidertest ./...`
2. **Review diagnostics**: Identify missing test coverage
3. **Add missing tests**: Follow the examples above for each rule
4. **Integrate with CI**: Add linter to your CI/CD pipeline
5. **Customize configuration**: Adjust settings for your project needs

---

## Reference Providers

See these HashiCorp providers for well-tested examples:

- **terraform-provider-time**: Small, well-tested (6 resources, 3 data sources)
- **terraform-provider-tls**: Crypto resources with comprehensive tests
- **terraform-provider-http**: Simple data source example

Study their test patterns at:
- https://github.com/hashicorp/terraform-provider-time
- https://github.com/hashicorp/terraform-provider-tls
- https://github.com/hashicorp/terraform-provider-http

---

## Support

- **Issues**: Report bugs or false positives at github.com/example/tfprovidertest/issues
- **Discussions**: Ask questions in GitHub Discussions
- **Documentation**: Full docs at github.com/example/tfprovidertest/blob/main/README.md
