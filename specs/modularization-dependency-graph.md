# Package Modularization - Dependency Graph

**Date:** 2025-12-08
**Status:** Planning Document for Recommendation 1

This document visualizes the current and proposed dependency structure to support the modularization effort.

## Current Flat Structure Dependencies

```
┌─────────────────────────────────────────────────────────────┐
│                    tfprovidertest (root package)            │
│                         ~9,010 LOC                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  tfprovidertest.go (157 LOC) ◄── Entry Point              │
│       │                                                     │
│       ├─► analyzer.go (353 LOC)                           │
│       │       ├─► registry.go (396 LOC)                   │
│       │       ├─► coverage.go (131 LOC)                   │
│       │       ├─► diagnostics.go (174 LOC)                │
│       │       ├─► parser.go (1,206 LOC)                   │
│       │       │       ├─► registry.go                      │
│       │       │       └─► utils.go (709 LOC)              │
│       │       ├─► linker.go (349 LOC)                     │
│       │       │       ├─► registry.go                      │
│       │       │       ├─► utils.go                         │
│       │       │       └─► settings.go (141 LOC)           │
│       │       └─► settings.go                              │
│       │                                                     │
│       └─► report.go (90 LOC)                              │
│                                                             │
│  All files in same package - no enforced boundaries        │
│  Any file can import any other file                        │
│  High coupling, difficult to reason about                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Proposed Modular Structure Dependencies

```
┌──────────────────────────────────────────────────────────────────┐
│                        Root Package                              │
│                      tfprovidertest                              │
└──────────────────────────────────────────────────────────────────┘
                               │
                               │
                ┌──────────────┴──────────────┐
                │                             │
                ▼                             ▼
    ┌───────────────────┐         ┌──────────────────────┐
    │  pkg/config/      │         │  tfprovidertest.go   │
    │  settings.go      │◄────────│  (Plugin Entry)      │
    │  (141 LOC)        │         │  (157 LOC)           │
    │                   │         └──────────────────────┘
    │  PUBLIC API       │                   │
    └───────────────────┘                   │
                                            │
                                            ▼
                              ┌──────────────────────┐
                              │  internal/analysis/  │
                              │  analyzer.go         │
                              │  coverage.go         │
                              │  diagnostics.go      │
                              │  report.go           │
                              │  (748 LOC)           │
                              └──────────────────────┘
                                   │        │
                    ┌──────────────┼────────┼──────────────┐
                    │              │        │              │
                    ▼              ▼        ▼              ▼
    ┌───────────────────┐  ┌──────────────────┐  ┌──────────────────┐
    │ internal/         │  │ internal/        │  │ internal/        │
    │ discovery/        │  │ registry/        │  │ matching/        │
    │ parser.go         │  │ registry.go      │  │ linker.go        │
    │ (1,206 LOC)       │  │ models.go        │  │ utils.go         │
    │                   │  │ (~600 LOC)       │  │ (1,058 LOC)      │
    └───────────────────┘  └──────────────────┘  └──────────────────┘
            │                       ▲                      │
            │                       │                      │
            └───────────────────────┴──────────────────────┘
                       (Uses models from registry)
```

## Detailed Dependency Flow

### Layer 1: Root Package
```
tfprovidertest.go
  └─> Imports: pkg/config, internal/analysis
  └─> Responsibilities:
      - Plugin registration with golangci-lint
      - Create analyzer instances
      - Pass settings to analyzers
```

### Layer 2: Public Configuration
```
pkg/config/settings.go
  └─> Imports: Standard library only
  └─> Responsibilities:
      - Settings struct definition
      - Default configuration
      - Configuration validation
  └─> Used by: All internal packages
```

### Layer 3: Analysis Orchestration
```
internal/analysis/analyzer.go
  ├─> Imports:
  │   ├─> internal/registry (data models)
  │   ├─> internal/discovery (buildRegistry)
  │   ├─> internal/matching (Linker)
  │   └─> pkg/config (Settings)
  └─> Responsibilities:
      - Run analyzer logic
      - Coordinate between discovery, registry, matching
      - Cache management (globalCache)
      - Report findings via analysis.Pass

internal/analysis/coverage.go
  ├─> Imports: internal/registry
  └─> Responsibilities:
      - Calculate test coverage statistics
      - Identify untested resources

internal/analysis/diagnostics.go
  ├─> Imports: internal/registry, internal/matching
  └─> Responsibilities:
      - Build diagnostic messages
      - Format verbose output
      - Generate suggested fixes

internal/analysis/report.go
  ├─> Imports: Standard library only
  └─> Responsibilities:
      - Report formatting
      - Severity calculation
```

### Layer 4: Core Logic Packages

#### Discovery (Resource/Test Finding)
```
internal/discovery/parser.go
  ├─> Imports:
  │   ├─> internal/registry (ResourceInfo, TestFunctionInfo)
  │   ├─> internal/matching (name utilities)
  │   └─> pkg/config (Settings)
  └─> Responsibilities:
      - Parse AST to find resources
      - Parse AST to find tests
      - Build registry from parsed data
      - Apply exclusion patterns
```

#### Registry (Data Storage)
```
internal/registry/models.go
  ├─> Imports: Standard library only
  └─> Responsibilities:
      - Define data structures
      - ResourceInfo, TestFunctionInfo, etc.
      - Enums: ResourceKind, MatchType

internal/registry/registry.go
  ├─> Imports: internal/registry/models
  └─> Responsibilities:
      - Thread-safe storage
      - Resource registration
      - Test function registration
      - Query methods
```

#### Matching (Test-to-Resource Linking)
```
internal/matching/linker.go
  ├─> Imports:
  │   ├─> internal/registry (ResourceRegistry)
  │   └─> pkg/config (Settings)
  └─> Responsibilities:
      - Implement matching strategies
      - Inferred content matching
      - Function name matching
      - File proximity matching
      - Fuzzy matching (optional)

internal/matching/utils.go
  ├─> Imports: Standard library only
  └─> Responsibilities:
      - String utilities (toSnakeCase, toTitleCase)
      - Name extraction (ExtractResourceFromFuncName)
      - File path utilities
      - Pattern matching
```

## Dependency Rules (Enforced by Package Boundaries)

### Allowed Dependencies
```
✓ tfprovidertest.go → pkg/config
✓ tfprovidertest.go → internal/analysis
✓ internal/analysis → pkg/config
✓ internal/analysis → internal/registry
✓ internal/analysis → internal/discovery
✓ internal/analysis → internal/matching
✓ internal/discovery → pkg/config
✓ internal/discovery → internal/registry
✓ internal/discovery → internal/matching
✓ internal/matching → pkg/config
✓ internal/matching → internal/registry
✓ All packages → Standard library
✓ All packages → golang.org/x/tools/go/analysis
```

### Prohibited Dependencies (Would Create Cycles)
```
✗ internal/registry → internal/discovery
✗ internal/registry → internal/matching
✗ internal/registry → internal/analysis
✗ internal/matching → internal/discovery
✗ internal/discovery → internal/analysis
✗ pkg/config → Any internal package
✗ pkg/config → tfprovidertest
```

## Import Graph Validation Commands

After each migration phase, run these commands to verify correctness:

```bash
# Check for circular dependencies
go list -f '{{.ImportPath}}: {{join .Deps "\n"}}' ./... | grep -A5 "^workspace"

# Verify no internal packages import from root
grep -r "import.*tfprovidertest\"" internal/ && echo "ERROR: internal imports root" || echo "OK"

# Verify no pkg/ imports from internal/
grep -r "import.*internal/" pkg/ && echo "ERROR: pkg imports internal" || echo "OK"

# Verify clean build
go build ./...

# Verify all tests pass
go test ./...

# Check for unexpected dependencies
go mod graph | grep workspace
```

## Migration Validation Checklist

After completing all phases:

- [ ] No circular dependencies detected
- [ ] All internal/ packages import only from:
  - Other internal/ packages (following rules above)
  - pkg/config
  - Standard library
  - External dependencies (analysis framework)
- [ ] pkg/config imports only from standard library
- [ ] tfprovidertest.go imports only from:
  - pkg/config
  - internal/analysis
  - External dependencies (plugin register)
- [ ] All tests pass
- [ ] Plugin loads in golangci-lint
- [ ] No performance regression

## Benefits of This Structure

1. **Clear Layering:**
   - Root → Analysis → Discovery/Registry/Matching
   - No upward dependencies
   - Easy to reason about data flow

2. **Separation of Concerns:**
   - Discovery: "How do we find things?"
   - Registry: "Where do we store things?"
   - Matching: "How do we link things?"
   - Analysis: "What do we report?"

3. **Independent Testing:**
   - Test matching logic without parser
   - Test parser without analyzer
   - Test coverage calculation without discovery

4. **Future Extensibility:**
   - Add new discovery strategies in internal/discovery
   - Add new matching strategies in internal/matching
   - Add new analyzers in internal/analysis
   - No impact on other packages

5. **Enforced Boundaries:**
   - Can't accidentally create circular dependencies
   - Compiler prevents invalid imports
   - Clear API surface for each package
