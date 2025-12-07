# Phase 0: Research & Technical Decisions

**Feature**: Terraform Provider Test Coverage Linter
**Date**: 2025-12-07
**Status**: Complete

## Research Areas

This document consolidates research findings for all technical unknowns identified in the Technical Context section of the implementation plan.

## 1. HashiCorp Testing Patterns Analysis

### Decision: Core Testing Patterns to Enforce

Based on HashiCorp's testing patterns documentation and the feature specification, the linter will enforce these 5 patterns:

1. **Basic Acceptance Tests**: Every resource and data source must have at least one TestAccResource* or TestAccDataSource* function using resource.Test()
2. **Update Tests**: Resources with updatable attributes must have multi-step tests that modify configuration
3. **Import Tests**: Resources implementing ImportState must have tests with ImportState: true and ImportStateVerify: true
4. **Error Case Tests**: Resources with validation rules must have tests using ExpectError
5. **State Check Functions**: All TestStep blocks must include Check fields with validation functions

### Rationale

These 5 patterns cover the critical testing scenarios identified in HashiCorp's documentation:
- Basic functionality (create/read)
- Update operations (in-place updates)
- Import operations (state import)
- Error handling (validation)
- Test quality (state verification)

### Alternatives Considered

- **6th pattern - Regression Tests**: Decided against enforcing regression tests as they are context-specific and cannot be detected via static analysis
- **7th pattern - Performance Tests**: Out of scope for acceptance testing patterns
- **Schema Migration Tests**: Out of scope per feature specification

### Implementation Notes

- Each pattern will be implemented as a separate analysis.Analyzer instance
- Analyzers can be selectively enabled/disabled via .golangci.yml configuration
- All analyzers will share common AST traversal utilities to minimize redundant parsing

### References

- HashiCorp Testing Patterns: https://developer.hashicorp.com/terraform/plugin/testing/testing-patterns
- terraform-plugin-testing documentation: https://pkg.go.dev/github.com/hashicorp/terraform-plugin-testing

---

## 2. golangci-lint Plugin Architecture

### Decision: Module Plugin System with plugin-module-register

The linter will use golangci-lint's Module Plugin System (not Go Plugin System) with the following architecture:

**Plugin Registration**:
```go
import "github.com/golangci/plugin-module-register/register"

func init() {
    register.Plugin("tfprovidertest", New)
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
    return []*analysis.Analyzer{
        BasicTestAnalyzer,
        UpdateTestAnalyzer,
        ImportTestAnalyzer,
        ErrorTestAnalyzer,
        StateCheckAnalyzer,
    }, nil
}

func (p *Plugin) GetLoadMode() string {
    return register.LoadModeSyntax
}
```

**Configuration Integration**:
- Plugin configured in .golangci.yml under linters.settings.custom
- Settings struct supports enable/disable flags for each rule
- Path patterns for resource/test file matching
- Plugin built automatically via .custom-gcl.yml

### Rationale

Module Plugin System advantages:
- No CGO_ENABLED requirement
- No build environment matching constraints
- Simpler distribution via Go modules
- Better integration with golangci-lint v2.x
- Automatic building through .custom-gcl.yml

### Alternatives Considered

- **Go Plugin System**: Rejected due to CGO requirements, brittle build environment matching, and deprecation warnings in golangci-lint v2.x
- **Standalone Binary**: Rejected because it loses golangci-lint integration benefits (unified configuration, parallel execution, IDE integration)

### Implementation Notes

- Use LoadModeSyntax for performance (no type information needed for pattern matching)
- Each analyzer instance is independent and can be run in parallel
- Error reporting via pass.Report() with precise ast.Pos locations
- Support for suggested fixes can be added later via pass.Report's SuggestedFixes field

### References

- golangci-lint Module Plugin docs: https://golangci-lint.run/plugins/module-plugins/
- plugin-module-register package: https://github.com/golangci/plugin-module-register
- Example plugin: https://github.com/golangci/example-plugin-module-linter

---

## 3. Terraform Plugin Framework Schema Analysis

### Decision: AST Pattern Matching for Framework Detection

The linter will detect terraform-plugin-framework resources and data sources using these AST patterns:

**Resource Detection**:
```go
// Pattern 1: schema.Resource interface implementation
type FuncDecl with:
  - Receiver type matches *{Name}Resource
  - Method name: "Schema"
  - Returns: schema.Schema

// Pattern 2: provider.ResourcesResponse
type FuncDecl with:
  - Receiver: *{Name}Provider
  - Method name: "Resources"
  - Body contains: resp.Resources = append(...)
  - Append argument: function returning schema.Resource interface
```

**Data Source Detection**:
```go
// Similar patterns for schema.DataSource interface
type FuncDecl with:
  - Receiver type matches *{Name}DataSource
  - Method name: "Schema"
  - Returns: schema.Schema
```

**Updatable Attribute Detection**:
```go
// Within schema.Schema definition:
schema.Attribute with:
  - No PlanModifiers field, OR
  - PlanModifiers field NOT containing RequiresReplace* functions

// RequiresReplace indicators:
- planmodifier.RequiresReplace()
- planmodifier.RequiresReplaceIfConfigured()
- planmodifier.RequiresReplaceIf()
```

**ImportState Detection**:
```go
// Method implementation check:
type FuncDecl with:
  - Receiver type matches *{Name}Resource
  - Method name: "ImportState"
  - Signature matches: (context.Context, ImportStateRequest, *ImportStateResponse)
```

**Validation Rules Detection**:
```go
// Within schema.Attribute:
- Required: true
- Validators: []validator.{Type} (non-empty)
- ConflictsWith: []string (non-empty)
- AtLeastOneOf: []string (non-empty)
- ExactlyOneOf: []string (non-empty)
```

### Rationale

AST pattern matching provides:
- No need for type information (faster analysis with LoadModeSyntax)
- Reliable detection without executing code
- Works across different terraform-plugin-framework versions (structural stability)
- Can differentiate framework vs SDK resources by import paths

### Alternatives Considered

- **Type-based Analysis**: Rejected because it requires LoadModeTypesInfo (slower), increases memory usage, and adds complexity
- **Regex on Source Code**: Rejected because it's brittle, can't handle formatting variations, and misses semantic meaning
- **Compiled Binary Reflection**: Rejected because it requires building the provider, increases runtime, and complicates CI integration

### Implementation Notes

- Use ast.Inspect() with type switches for efficient traversal
- Cache parsed AST between analyzer runs to avoid redundant parsing
- Maintain a resource registry (map[string]*ResourceInfo) to correlate resources with their test files
- Import path checking: only analyze files importing "github.com/hashicorp/terraform-plugin-framework"

### References

- terraform-plugin-framework schema package: https://pkg.go.dev/github.com/hashicorp/terraform-plugin-framework/resource/schema
- Go AST documentation: https://pkg.go.dev/go/ast
- go/analysis package: https://pkg.go.dev/golang.org/x/tools/go/analysis

---

## 4. Target Provider Baseline

### Decision: Baseline Test Coverage Metrics

Validation will use these 4 target providers with the following expected baselines:

**terraform-provider-http** (HashiCorp Official):
- Resources: ~2 (http data source only, no managed resources)
- Data Sources: ~1 (http)
- Expected Coverage: 100% acceptance tests
- Expected Gaps: None (well-maintained official provider)
- False Positive Risk: Low

**terraform-provider-tls** (HashiCorp Official):
- Resources: ~4 (tls_private_key, tls_cert_request, tls_locally_signed_cert, tls_self_signed_cert)
- Data Sources: ~2 (tls_certificate, tls_public_key)
- Expected Coverage: 100% basic tests, 90%+ update/import tests
- Expected Gaps: Minimal, possibly some error case tests
- False Positive Risk: Low

**terraform-provider-time** (HashiCorp Official):
- Resources: ~6 (time_offset, time_rotating, time_sleep, time_static)
- Data Sources: ~3 (time_rfc3339, time_sleep, time_static)
- Expected Coverage: 100% basic tests, high update/import coverage
- Expected Gaps: None (purpose-built for testing scenarios)
- False Positive Risk: Very low
- Performance Target: <10 seconds for full analysis

**terraform-provider-aap** (Community Provider):
- Resources: ~15-20 (Ansible Automation Platform resources)
- Data Sources: ~5-10
- Expected Coverage: 70-85% (community provider baseline)
- Expected Gaps: 3+ missing tests (per SC-010)
- False Positive Risk: Medium (may use non-standard patterns)

### Rationale

This mix provides:
- **Official HashiCorp providers**: Well-tested, standard patterns, good baseline for zero false positives
- **Community provider**: Real-world validation of pattern flexibility and gap detection
- **Size diversity**: Small (http), medium (tls, time), larger (aap) for performance testing
- **Pattern diversity**: Different resource types (crypto, time, platform APIs)

### Alternatives Considered

- **terraform-provider-aws**: Rejected due to size (500+ resources would dominate testing)
- **terraform-provider-google**: Rejected due to similar size concerns
- **terraform-provider-random**: Rejected as too simple (minimal testing patterns)

### Implementation Notes

- Baseline metrics will be established by running the linter against each provider
- Results documented in validation/ directory with:
  - Total resources/data sources count
  - Test coverage percentage per rule
  - List of detected gaps
  - False positive analysis
- Success Criteria: SC-002 (zero false positives on http, tls, time), SC-010 (3+ gaps in aap)

### References

- terraform-provider-http: https://github.com/hashicorp/terraform-provider-http
- terraform-provider-tls: https://github.com/hashicorp/terraform-provider-tls
- terraform-provider-time: https://github.com/hashicorp/terraform-provider-time
- terraform-provider-aap: https://github.com/ansible/terraform-provider-aap

---

## 5. Performance Constraints

### Decision: Parallel Analysis with AST Caching

Performance optimization strategy:

**AST Caching**:
- Parse each .go file once, cache in memory
- Share cached AST between all 5 analyzers
- Use sync.Map for thread-safe concurrent access

**Parallel Analysis**:
- Each analyzer runs independently (no dependencies)
- golangci-lint handles parallelization automatically
- Target: 5 concurrent analyzers analyzing different files

**Resource Registry**:
- Single-pass AST traversal builds resource registry
- Map structure: map[string]*ResourceInfo
- Keys: resource type names (e.g., "widget", "account")
- Values: ResourceInfo struct with schema location, attributes, methods

**Performance Targets**:
- Small providers (6 resources): <10 seconds (terraform-provider-time baseline)
- Medium providers (50 resources): <60 seconds
- Large providers (500 resources): <300 seconds (5 minutes)
- Memory usage: <500MB for 500 resources

**Benchmarking Approach**:
- Use testing.B benchmarks for analyzer performance
- Profile with pprof for memory and CPU hotspots
- Test against terraform-provider-time first (fastest feedback)
- Scale testing with terraform-provider-aap (realistic size)

### Rationale

This approach provides:
- Minimal redundant work (single AST parse per file)
- Maximum parallelism (independent analyzers)
- Predictable memory usage (AST cache bounded by file count)
- Fast feedback during development (small provider benchmarks)

### Alternatives Considered

- **Sequential Analysis**: Rejected due to 5x performance penalty (each analyzer parses independently)
- **Single Mega-Analyzer**: Rejected due to complexity and loss of granular enable/disable control
- **On-Disk Caching**: Rejected due to I/O overhead and cache invalidation complexity

### Implementation Notes

- Implement AST cache as shared utility in ast_cache.go
- Use runtime.NumCPU() to guide parallelism hints
- Add --profile flag for development profiling
- Document performance characteristics in README.md

### Benchmarking Results (Post-Implementation)

To be populated after implementation with actual metrics from:
- Benchmark suite results (ns/op, MB/sec)
- Real provider analysis times
- Memory profiling data

### References

- Go AST performance: https://pkg.go.dev/go/ast
- go/analysis parallelism: https://pkg.go.dev/golang.org/x/tools/go/analysis
- pprof profiling: https://pkg.go.dev/runtime/pprof

---

## Research Summary

All technical unknowns have been resolved:

1. **Testing Patterns**: 5 core patterns identified and prioritized
2. **Plugin Architecture**: Module Plugin System with plugin-module-register
3. **Schema Analysis**: AST pattern matching without type information
4. **Validation Baseline**: 4 target providers with expected metrics
5. **Performance Strategy**: Parallel analysis with AST caching

No further clarifications needed. Ready to proceed to Phase 1 (Design & Contracts).
