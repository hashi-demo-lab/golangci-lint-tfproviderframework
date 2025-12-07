# Golangci-lint Plugin Development Constitution

## Core Principles

### I. Test-Driven Development (NON-NEGOTIABLE)
TDD is mandatory for all linter development. The Red-Green-Refactor cycle must be strictly enforced:
- **RED**: Write failing tests first using `analysistest.Run()` with `// want "message"` comments in testdata
- **GREEN**: Implement minimal code to pass tests
- **REFACTOR**: Improve code quality while keeping tests green

Tests must be written and approved before implementation begins. No code merges without passing test coverage.

### II. go/analysis API First
All linters MUST use the `golang.org/x/tools/go/analysis` framework:
- Provides unified interface for IDE, metalinter, and code review integration
- Enables automatic CLI generation with `singlechecker.Main()`
- Supports analyzer dependency graphs for optimized execution
- Required for golangci-lint integration (non-go/analysis linters rejected)

### III. Module Plugin System Preferred
Use the Module Plugin System over Go Plugin System:
- Configure via `.custom-gcl.yml` for automatic builds
- Define plugins in `.golangci.yml` under `linters.settings.custom` with `type: "module"`
- Avoids CGO_ENABLED requirements and build environment matching constraints
- Supports both Go proxy modules and local paths

### IV. Latest golangci-lint Version (NON-NEGOTIABLE)
Always use the latest stable version of golangci-lint:
- **Current Version**: v2.7.1 (as of December 2025)
- Pin version explicitly in `.custom-gcl.yml` and CI configurations
- Update promptly when new versions are released
- Use v2 configuration format (`version: "2"` in `.golangci.yml`)
- Install via: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`

### V. Project Structure Standards
Follow the official golangci module plugin layout (ref: [example-plugin-module-linter](https://github.com/golangci/example-plugin-module-linter)):
```
.
├── {lintername}.go           # Core analyzer with plugin registration
├── {lintername}_test.go      # Tests using analysistest
├── testdata/
│   └── src/testlintdata/
│       └── {testcase}/       # Test fixtures with // want comments
├── go.mod
├── go.sum
├── .golangci.example.yml     # Configuration template for consumers
└── README.md
```

### VI. Plugin Registration Pattern
Use the `plugin-module-register` package for golangci-lint integration:
```go
import "github.com/golangci/plugin-module-register/register"

func init() {
    register.Plugin("{lintername}", New)
}

type Plugin struct {
    settings Settings
}

func New(settings any) (register.LinterPlugin, error) {
    s, err := register.DecodeSettings[Settings](settings)
    if err != nil {
        return nil, err
    }
    return &Plugin{settings: s}, nil
}

func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
    return []*analysis.Analyzer{Analyzer}, nil
}

func (p *Plugin) GetLoadMode() string {
    return register.LoadModeSyntax  // or LoadModeTypesInfo
}
```

### VII. Analyzer Implementation Pattern
Every analyzer must implement:
- `analysis.Analyzer` struct with Name, Doc, Run, and Requires fields
- Run function accepting `*analysis.Pass` and returning `(interface{}, error)`
- Use `ast.Inspect()` for AST traversal with type assertions
- Report diagnostics via `pass.Report()` with precise positions
- Choose appropriate load mode: `LoadModeSyntax` (fast) or `LoadModeTypesInfo` (with types)

## Required Dependencies

Based on the official example, use these Go module dependencies:
```
require (
    github.com/golangci/plugin-module-register v0.1.1
    github.com/stretchr/testify v1.10.0
    golang.org/x/tools v0.31.0
)
```

Minimum Go version: **Go 1.23.0+**

## Testing Standards

### Acceptance Test Requirements
- Use `analysistest.Run()` for all linter tests
- Create testdata directories with real code samples
- Embed expected diagnostics as `// want "message"` inline comments
- Test both positive cases (should report) and negative cases (should pass)
- Validate suggested fixes when implementing `-fix` support
- Use `runtime.Caller()` to dynamically locate testdata directories

### Test Implementation Pattern
```go
func TestAnalyzer(t *testing.T) {
    analysistest.Run(t, testdataDir(), Analyzer, "testlintdata/{testcase}")
}

func testdataDir() string {
    _, filename, _, _ := runtime.Caller(0)
    return filepath.Join(filepath.Dir(filename), "testdata")
}
```

### Test File Organization
```
testdata/
└── src/
    └── testlintdata/
        └── {testcase}/
            └── example.go    # Contains // want comments for expected diagnostics
```

## Configuration Standards

### .golangci.yml Requirements (v2 Format)
```yaml
version: "2"

linters:
  default: none
  enable:
    - {lintername}
  settings:
    custom:
      {lintername}:
        type: module
        description: The description of the linter
        original-url: github.com/org/{lintername}
        settings:
          key: value
```

### .custom-gcl.yml Format
```yaml
version: v2.7.1
plugins:
  - module: 'github.com/org/linter'
    import: 'github.com/org/linter/analyzer'
    version: v1.0.0
```

## Quality Gates

### Pre-Merge Requirements
1. All `analysistest` tests pass
2. Linter runs successfully against target codebase
3. No regressions in existing linter behavior
4. Documentation updated (README, inline comments)
5. Configuration examples provided in `.golangci.example.yml`

### Performance Standards
- Use `ast.Inspect()` with early returns for targeted traversal
- Choose `LoadModeSyntax` when type information not needed
- Leverage analyzer Facts for cross-package analysis
- Profile complex analyzers to ensure reasonable execution time

## Anti-Patterns to Avoid

- **Never** skip the RED phase - tests must fail before implementation
- **Never** use Go Plugin System for new development (legacy only)
- **Never** implement analyzers without `go/analysis` framework
- **Never** hardcode paths or environment-specific values
- **Never** report diagnostics without precise source positions
- **Never** ignore analyzer dependency ordering
- **Never** use outdated golangci-lint versions - always use latest stable (v2.x)
- **Never** use `LoadModeTypesInfo` when `LoadModeSyntax` suffices

## Governance

This constitution supersedes all other development practices for golangci-lint plugin development. Amendments require:
1. Documentation of proposed change
2. Team review and approval
3. Migration plan for existing plugins

All code reviews must verify compliance with these principles. Use AGENTS.md for runtime development guidance.

**Version**: 1.1.0 | **Ratified**: 2025-12-07 | **Last Amended**: 2025-12-07
