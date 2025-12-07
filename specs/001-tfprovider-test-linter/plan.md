# Implementation Plan: Terraform Provider Test Coverage Linter

**Branch**: `001-tfprovider-test-linter` | **Date**: 2025-12-07 | **Spec**: /workspace/specs/001-tfprovider-test-linter/spec.md
**Input**: Feature specification from `/specs/001-tfprovider-test-linter/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

This feature implements a custom golangci-lint plugin that identifies test coverage gaps in Terraform providers built with terraform-plugin-framework. The linter enforces HashiCorp's testing best practices by detecting missing acceptance tests, update tests, import tests, error case tests, and validating test quality through state check functions. It integrates as a golangci-lint module plugin using the go/analysis framework and follows strict TDD practices.

## Technical Context

**Language/Version**: Go 1.23.0+
**Primary Dependencies**:
  - golang.org/x/tools/go/analysis (AST analysis framework)
  - github.com/golangci/plugin-module-register v0.1.1 (plugin integration)
  - golang.org/x/tools/go/analysis/analysistest (testing framework)
  - github.com/stretchr/testify v1.10.0 (test assertions)
**Storage**: N/A (static analysis tool, no persistent storage)
**Testing**: analysistest.Run() with testdata fixtures and // want comments
**Target Platform**: Linux, macOS, Windows (cross-platform Go tooling)
**Project Type**: Single project (golangci-lint plugin module)
**Performance Goals**:
  - Analyze 500+ resources within 5 minutes
  - Complete terraform-provider-time (6 resources, 3 data sources) in under 10 seconds
**Constraints**:
  - Must integrate with golangci-lint v2.7.1+ module plugin system
  - Must use go/analysis framework (required for golangci-lint)
  - Must support LoadModeSyntax for performance (no type information required)
  - Zero false positives on well-tested providers
**Scale/Scope**:
  - 5 distinct linting rules (basic test, update test, import test, error test, state checks)
  - Validation against 4 target providers (http, tls, time, aap)
  - Support for providers with 500+ resources

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Test-Driven Development (NON-NEGOTIABLE)
**Status**: PASS
- All linting rules will be implemented with TDD using analysistest.Run()
- Test fixtures in testdata/ with // want comments will be created before implementation
- FR-018 explicitly requires test cases for each linting rule
- Red-Green-Refactor cycle will be enforced

### II. go/analysis API First
**Status**: PASS
- Project explicitly requires golang.org/x/tools/go/analysis framework
- FR-012 mandates golangci-lint plugin integration (requires go/analysis)
- Plan includes analysis.Analyzer implementation with Run function

### III. Module Plugin System Preferred
**Status**: PASS
- Using Module Plugin System (not Go Plugin System)
- Configuration via .custom-gcl.yml for automatic builds
- Plugin registration via github.com/golangci/plugin-module-register v0.1.1
- No CGO_ENABLED requirements

### IV. Latest golangci-lint Version (NON-NEGOTIABLE)
**Status**: PASS
- Targeting golangci-lint v2.7.1 (latest stable as of December 2025)
- FR-012 requires golangci-lint v1.50+ minimum, using latest v2.x
- Will use v2 configuration format in .golangci.yml
- Version will be pinned in .custom-gcl.yml

### V. Project Structure Standards
**Status**: PASS
- Following official example-plugin-module-linter layout
- Core analyzer file, test file, testdata/ directory structure planned
- .golangci.example.yml configuration template included
- README.md for documentation

### VI. Plugin Registration Pattern
**Status**: PASS
- Will use plugin-module-register package
- init() function with register.Plugin()
- Plugin struct with BuildAnalyzers() and GetLoadMode() methods
- Settings decoded via register.DecodeSettings[Settings]()

### VII. Analyzer Implementation Pattern
**Status**: PASS
- Will implement analysis.Analyzer struct with Name, Doc, Run, Requires
- Run function with *analysis.Pass parameter
- ast.Inspect() for AST traversal
- pass.Report() for diagnostics with precise positions
- LoadModeSyntax chosen for performance (no type information needed)

**OVERALL GATE STATUS**: PASS - All constitutional requirements satisfied. Proceed to Phase 0.

---

## Post-Design Constitution Re-Check

**Date**: 2025-12-07
**Status**: PASS - All design artifacts conform to constitutional requirements

### Design Artifact Review

**research.md**:
- Decisions grounded in official documentation (HashiCorp patterns, golangci-lint docs)
- All technical unknowns resolved with rationale
- Module Plugin System confirmed over Go Plugin System
- AST-based analysis without type information (LoadModeSyntax)

**data-model.md**:
- Entities follow go/analysis framework patterns
- ResourceRegistry implements shared state pattern
- Analyzer instances are independent (parallelizable)
- TDD-friendly design with clear validation rules

**contracts/**:
- analyzer-interface.go defines go/analysis-compliant interfaces
- configuration-schema.yaml uses golangci-lint v2 format
- All 5 analyzers implement analysis.Analyzer contract
- Plugin registration follows plugin-module-register pattern

**quickstart.md**:
- TDD approach documented (write tests first)
- Examples use analysistest.Run() pattern
- Configuration examples use v2 format
- Installation instructions target golangci-lint v2.7.1+

### Constitutional Compliance Verification

1. **TDD**: Design includes testdata/ structure with // want comments before implementation
2. **go/analysis**: All analyzers implement analysis.Analyzer interface
3. **Module Plugin System**: Plugin registration uses plugin-module-register v0.1.1
4. **Latest golangci-lint**: Targets v2.7.1, uses v2 configuration format
5. **Project Structure**: Follows example-plugin-module-linter layout exactly
6. **Plugin Registration**: Uses init() with register.Plugin() pattern
7. **Analyzer Implementation**: LoadModeSyntax, ast.Inspect(), pass.Report() patterns

**FINAL STATUS**: PASS - Ready for Phase 2 (Task Generation via /speckit.tasks)

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

Following the official golangci-lint module plugin structure:

```text
/workspace/
├── tfprovidertest.go              # Core analyzer implementation
│                                  # - Implements 5 analysis.Analyzer instances
│                                  # - Resource/DataSource detection logic
│                                  # - Test file matching and validation
│                                  # - Plugin registration via init()
│
├── tfprovidertest_test.go         # Analyzer tests using analysistest
│                                  # - TestBasicTestCoverage
│                                  # - TestUpdateTestCoverage
│                                  # - TestImportTestCoverage
│                                  # - TestErrorTestCoverage
│                                  # - TestStateCheckValidation
│
├── testdata/                      # Test fixtures for analysistest
│   └── src/
│       └── testlintdata/
│           ├── basic_missing/     # Resources with no acceptance tests
│           │   ├── resource_widget.go
│           │   └── resource_widget_test.go  # Missing TestAccResourceWidget
│           ├── basic_passing/     # Well-tested resources
│           │   ├── resource_account.go
│           │   └── resource_account_test.go
│           ├── update_missing/    # Resources needing update tests
│           │   ├── resource_config.go
│           │   └── resource_config_test.go  # Single-step only
│           ├── import_missing/    # Resources with ImportState but no test
│           │   ├── resource_server.go
│           │   └── resource_server_test.go
│           ├── error_missing/     # Resources with validators, no error tests
│           │   ├── resource_network.go
│           │   └── resource_network_test.go
│           └── checks_missing/    # Tests with no Check functions
│               ├── resource_database.go
│               └── resource_database_test.go
│
├── go.mod                         # Module definition
│                                  # - require golang.org/x/tools v0.31.0
│                                  # - require github.com/golangci/plugin-module-register v0.1.1
│                                  # - require github.com/stretchr/testify v1.10.0
│
├── go.sum                         # Dependency checksums
│
├── .golangci.example.yml          # Configuration template for users
│                                  # - Example linter settings
│                                  # - Rule enable/disable examples
│                                  # - Path pattern configuration
│
├── .custom-gcl.yml                # Local development plugin configuration
│                                  # - version: v2.7.1
│                                  # - Plugin module path and import
│
└── README.md                      # Plugin documentation
                                   # - Installation instructions
                                   # - Configuration guide
                                   # - Linting rules reference
                                   # - Example outputs
```

**Structure Decision**: Single project (golangci-lint plugin module)

This follows the official example-plugin-module-linter pattern with all code in the repository root. The linter is a standalone Go module that integrates with golangci-lint's module plugin system. No nested src/ or internal/ directories are needed for this small, focused plugin.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

N/A - No constitutional violations. All requirements align with established patterns.
