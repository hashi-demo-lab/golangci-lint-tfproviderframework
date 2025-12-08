# Architecture Improvements Specification

**Created:** 2025-12-08
**Reviewed:** 2025-12-08 02:13:33 UTC
**Implemented:** 2025-12-08
**Status:** PARTIALLY IMPLEMENTED - 4 of 7 recommendations completed

---

## Overview

Based on a review of the provided source code, specifically parser.go, registry.go, analyzer.go, and the project structure, here are recommendations for architectural improvements to enhance maintainability, testability, and scalability.

1. Modularize Package Structure
   Currently, the core logic resides in a flat structure within the root package tfprovidertest. As the project has grown to ~6,000 LOC, this creates tight coupling and makes the API surface area unclear.

Recommendation: Refactor the root package into distinct sub-packages under an internal/ directory (to hide implementation details) or pkg/ (if reusable).

internal/discovery: Move parser.go and AST analysis logic here. This separates finding code from analyzing it.

internal/registry: Move registry.go here. This package would define the data model (ResourceInfo, TestFunctionInfo) and storage.

internal/matching: Move linker.go and utils.go (related to naming conventions) here.

internal/analysis: Move the actual check logic (analyzer.go) here.

Benefit: This enforces separation of concerns. For example, the parser shouldn't necessarily know about linker logic, and the registry should just be a dumb store.

2. Refactor parser.go using the Strategy Pattern
   The parseResources function in parser.go is becoming a complex monolith. It currently implements four distinct strategies (Schema method, Factory functions, Metadata method, Action factories) inside one function body.

Recommendation: Define a DiscoveryStrategy interface.

Go

type DiscoveryStrategy interface {
Discover(file *ast.File, fset *token.FileSet) []\*ResourceInfo
}
Implement separate strategies:

SchemaMethodStrategy

FrameworkFactoryStrategy (for New... functions)

MetadataMethodStrategy

Benefit: This makes parser.go easier to read and test. Adding a new way to detect resources (e.g., scanning for struct tags in the future) becomes adding a new struct rather than modifying a massive loop.

3. Decompose the "God Object" Registry
   The ResourceRegistry struct in registry.go handles storage, linking, concurrency locking, and coverage calculation. It knows too much about the relationships between objects.

Recommendation: Split responsibilities.

Storage: Keep Registry strictly for thread-safe storage of definitions.

Calculator: Create a CoverageCalculator service that takes a read-only view of the registry and computes the ResourceCoverage structs.

Linker: The Linker struct already exists, but it should be the only place where relationships between Tests and Resources are mutated, rather than the Registry having mixed mutation methods.

4. Standardize Configuration Loading
   The Settings struct handles configuration, but the logic for applying these settings is scattered. For instance, parser.go takes a ParserConfig struct which is manually constructed from Settings in analyzer.go.

Recommendation: Use a "Configuration Object" pattern that is passed down through the context or constructor chain.

Create a config package.

Ensure that ParserConfig and Settings are aligned so you don't need manual mapping code like:

Go

config := ParserConfig{
CustomHelpers: settings.CustomTestHelpers,
LocalHelpers: localHelpers,
TestNamePatterns: settings.TestNamePatterns,
}
Benefit: Reduces the risk of a setting added to Settings being ignored because it wasn't manually mapped to ParserConfig.

5. Improve State Management in Analyzers
   In analyzer.go, there is a globalCache map protected by a mutex. This is used to share the Registry between the five different analyzers (BasicTest, UpdateTest, etc.) to avoid re-parsing the AST 5 times.

Recommendation: While this works for golangci-lint plugins, it relies on global state. A cleaner approach for the architectural design (if the linter framework allows) is to create a "Runner" or "Coordinator" analyzer that:

Runs once.

Builds the Registry.

Outputs the Registry as its "Fact" (result).

Have the other 5 analyzers depend on the Coordinator analyzer and consume its output.

Benefit: This removes the need for globalCache, sync.Once, and manual cache clearing logic, leveraging the analysis framework's built-in dependency management instead.

6. Remove Shell Script Dependencies
   The root directory contains scripts like fix_settings.sh which contains embedded Go code in a heredoc. This indicates a complex build or generation process that is brittle.

Recommendation: Move any logic required to generate or fix code into a Go //go:generate directive or a strictly typed Makefile. Avoid embedding Go source code inside shell scripts strings.

7. Clean Up Deprecated Fields
   The ResourceInfo struct in registry.go contains fields marked as "Deprecated" or "New" alongside each other:

Go

Kind ResourceKind // New: replaces IsDataSource
IsDataSource bool // Deprecated
IsAction bool // Deprecated
Recommendation: Since this is a specialized linter likely used internally or in a specific ecosystem, aggressively refactor to remove the deprecated boolean flags and rely solely on the Kind enum. This prevents logic bugs where Kind says one thing but IsDataSource says another.

---

## Implementation Checklist

### Priority Summary

| # | Recommendation | Priority | Effort | Status |
|---|----------------|----------|--------|--------|
| 7 | Clean Up Deprecated Fields | **HIGH** | Small (30-40 min) | [x] **COMPLETED** |
| 4 | Standardize Configuration Loading | **HIGH** | Medium (14 hrs) | [x] **Phase 1 COMPLETED** |
| 2 | Strategy Pattern for parser.go | **HIGH** | Medium (22-32 hrs) | [ ] Pending |
| 3 | Decompose Registry God Object | **HIGH** | Medium (6-8 hrs) | [x] **COMPLETED** |
| 5 | Improve State Management in Analyzers | **MEDIUM** | Small-Large | [ ] Pending |
| 6 | Remove Shell Script Dependencies | **LOW** | Small | [x] **COMPLETED** |
| 1 | Modularize Package Structure | **LOW** | Large | [ ] Pending |

---

### Recommendation 7: Clean Up Deprecated Fields ✅ COMPLETED
**Priority:** HIGH | **Effort:** Small (30-40 minutes)
**Completed:** 2025-12-08

#### Investigation Summary
- **Deprecated fields:** `IsDataSource` and `IsAction` in ResourceInfo struct (registry.go:254-255)
- **Total usages:** 28 across 4 files (parser.go, registry.go, analyzer.go, tests)
- **Risk:** Medium - backward compatibility inference logic could mask inconsistencies

#### Implementation Tasks

**Phase 1: Remove Deprecated Field Assignments (parser.go)**
- [x] Remove `IsDataSource: isDataSource` assignment (line 142)
- [x] Remove `IsDataSource: isDataSource` assignment (line 184)
- [x] Remove `IsAction: true` assignment (line 318)
- [x] Remove `IsAction: true` assignment (line 341)

**Phase 2: Replace Boolean Checks with Kind Checks (analyzer.go)**
- [x] Update line 118: `resource.IsDataSource` → `resource.Kind == KindDataSource`
- [x] Update line 152: skip data sources check
- [x] Update line 255: skip data sources check
- [x] Update line 298: skip data sources check
- [x] Update line 354: message building check

**Phase 3: Update Registry Logic (registry.go)**
- [x] Update line 520: GetResourcesMissingCheckDestroy (moved to coverage.go)
- [x] Update lines 572, 610, 647: BuildExpectedTestFunc, BuildVerboseDiagnosticInfo (moved to diagnostics.go)
- [x] Remove backward compatibility inference block (lines 77-83)
- [x] Remove field declarations (lines 254-255)

**Phase 4: Update Tests (tfprovidertest_test.go)**
- [x] Replace 8 IsDataSource assertions with Kind checks
- [x] Run full test suite to validate

---

### Recommendation 4: Standardize Configuration Loading (Phase 1 ✅ COMPLETED)
**Priority:** HIGH | **Effort:** Medium (14 hours)
**Phase 1 Completed:** 2025-12-08

#### Investigation Summary
- **Settings struct:** 24 fields in settings.go
- **ParserConfig struct:** Only 3 fields - missing most Settings
- **Critical Bug:** All 7 analyzers call `DefaultSettings()` instead of using passed configuration!
- **Unused Settings:** ResourcePathPattern, DataSourcePathPattern, TestFilePattern, ResourceNamingPattern, ProviderPrefix

#### Implementation Tasks

**Phase 1: Fix Critical Bug - Analyzer Settings Bypass** ✅ COMPLETED
- [x] Fix runBasicTestAnalyzer - use passed settings (closure pattern in tfprovidertest.go)
- [x] Fix runUpdateTestAnalyzer - use passed settings
- [x] Fix runImportTestAnalyzer - use passed settings
- [x] Fix runErrorTestAnalyzer - use passed settings
- [x] Fix runDriftCheckAnalyzer - use passed settings
- [x] Fix runSweeperAnalyzer - use passed settings
- [x] Fix runStateCheckAnalyzer - use passed settings

**Implementation Note:** Used closure pattern in `tfprovidertest.go` where each analyzer is created dynamically via `createXxxAnalyzer()` methods that capture `p.settings` in the closure, ensuring settings are properly propagated.

**Phase 2: Extend ParserConfig** (Future)
- [ ] Add TestFilePattern field to ParserConfig
- [ ] Add ResourceNamingPattern field to ParserConfig
- [ ] Add ProviderPrefix field to ParserConfig
- [ ] Update buildRegistry mapping (lines 698-702)

**Phase 3: Add Validation** (Future)
- [ ] Add regex compilation checks for patterns
- [ ] Add cross-field validation
- [ ] Add "at least one analyzer enabled" check

**Phase 4: Testing** (Future)
- [ ] Update ParserConfig construction in tests
- [ ] Verify all settings are propagated correctly

---

### Recommendation 2: Strategy Pattern for parser.go
**Priority:** HIGH | **Effort:** Medium (22-32 hours / 3-4 days)

#### Investigation Summary
- **Function:** `parseResources()` (lines 85-355) - 270 LOC
- **Strategies:** 4 detection strategies with clear boundaries
- **Complexity:** 18+ cyclomatic complexity, 5 AST passes
- **Coupling:** High - shared state maps, cross-strategy updates

#### Strategy Boundaries
| Strategy | Lines | LOC | Description |
|----------|-------|-----|-------------|
| 1: Schema Method | 103-153 | 51 | Schema() methods on types |
| 2: Factory Function | 155-192 | 38 | New*DataSource/Resource functions |
| 3: Metadata Method | 194-249 | 56 | Metadata() with resp.TypeName |
| 4: Action Factory | 251-346 | 96 | New*Action factories |

#### Implementation Tasks

**Phase 1: Infrastructure**
- [ ] Define `DiscoveryStrategy` interface
- [ ] Define `CoordinatedStrategy` interface (for override logic)
- [ ] Create `DiscoveryResult` struct for state tracking
- [ ] Write integration test for full parseResources()

**Phase 2: Extract Strategies**
- [ ] Create `SchemaMethodStrategy` struct (lines 103-153)
- [ ] Create `FactoryFunctionStrategy` struct (lines 155-192)
- [ ] Create `MetadataMethodStrategy` struct (lines 194-249)
- [ ] Create `ActionFactoryStrategy` struct (lines 251-346)

**Phase 3: Refactor Main Function**
- [ ] Update parseResources() to use strategy pattern
- [ ] Implement priority-based execution
- [ ] Handle cross-strategy state coordination

**Phase 4: Testing & Documentation**
- [ ] Unit test each strategy in isolation
- [ ] Verify deduplication logic
- [ ] Update AGENTS.md documentation

---

### Recommendation 3: Decompose Registry God Object ✅ COMPLETED
**Priority:** HIGH | **Effort:** Medium (6-8 hours)
**Completed:** 2025-12-08

#### Investigation Summary
- **Methods:** 17 public methods across 4 responsibility areas
- **Duplicate code:** GetResourceCoverage/GetAllResourceCoverage are nearly identical (100+ lines)
- **Registry LOC:** 716 lines → target 400 lines after refactoring

#### Method Categories
| Category | Methods | Action |
|----------|---------|--------|
| Storage | 8 | Keep in Registry |
| Coverage Calculation | 5 | Move to CoverageCalculator |
| Diagnostics/Utilities | 7 | Move to diagnostics.go |

#### Implementation Tasks

**Phase 1: Extract Diagnostic Utilities** ✅ COMPLETED
- [x] Create `/workspace/diagnostics.go`
- [x] Move HasMatchingTestFile()
- [x] Move BuildExpectedTestPath()
- [x] Move BuildExpectedTestFunc()
- [x] Move ClassifyTestFunctionMatch()
- [x] Move BuildVerboseDiagnosticInfo()
- [x] Move FormatVerboseDiagnostic()
- [x] Move buildSuggestedFixes()

**Phase 2: Create CoverageCalculator** ✅ COMPLETED
- [x] Create `/workspace/coverage.go`
- [x] Create CoverageCalculator struct with registry reference
- [x] Move GetResourceCoverage()
- [x] Move GetAllResourceCoverage()
- [x] Move GetUntestedResources()
- [x] Move GetResourcesMissingStateChecks()
- [x] Move GetResourcesMissingCheckDestroy()
- [x] Consolidate duplicate coverage calculation logic

**Phase 3: Update Consumers** ✅ COMPLETED
- [x] Update analyzer.go to use CoverageCalculator
- [x] Add CoverageCalculator construction in buildRegistry()
- [x] Update type assertions and imports

**Phase 4: Testing** ✅ COMPLETED
- [x] Run full test suite
- [x] Add unit tests for CoverageCalculator isolation

---

### Recommendation 5: Improve State Management in Analyzers
**Priority:** MEDIUM | **Effort:** Small (immediate) to Large (long-term)

#### Investigation Summary
- **Global state:** 3 variables (globalCacheMu, globalCache, registryCache)
- **Analyzers:** 7 (not 5 as spec states)
- **Cache type:** Per-pass map with sync.Once pattern
- **Risk:** Memory leaks in long-running golangci-lint instances

#### Implementation Tasks

**Phase 1: Add Cache Cleanup (v1.1)**
- [ ] Add ClearRegistryCache() calls in tfprovidertest.go plugin lifecycle
- [ ] Document cache cleanup requirements
- [ ] Add tests for cleanup behavior

**Phase 2: TTL-Based Cache (v1.2)**
- [ ] Implement time-bounded cache with TTL
- [ ] Add automatic cleanup for stale entries
- [ ] Reduce lock contention

**Phase 3: Dependency Injection (v2.0 - Long-term)**
- [ ] Research analysis.Pass context propagation
- [ ] Design registry injection pattern
- [ ] Plan breaking API changes

---

### Recommendation 6: Remove Shell Script Dependencies ✅ COMPLETED
**Priority:** LOW | **Effort:** Small
**Completed:** 2025-12-08

#### Investigation Summary
- **Scripts found:** 2 (fix_settings.sh, apply_changes.sh)
- **Issue:** fix_settings.sh contains 34KB of embedded Go code in heredoc
- **Risk:** Brittle build process, hard to maintain

#### Implementation Tasks

- [x] Analyze if scripts are still needed - **Result: One-time migration utilities, no longer needed**
- [x] Remove shell scripts from repository - **Deleted fix_settings.sh and apply_changes.sh**
- [x] Update build documentation - **No changes needed, scripts were not part of build process**

**Implementation Note:** Both scripts were one-time migration utilities used during early development. They are not part of the regular build process and can be safely removed.

---

### Recommendation 1: Modularize Package Structure
**Priority:** LOW | **Effort:** Large

#### Investigation Summary
- **Current:** Flat structure with 15 .go files in root (~8,911 LOC)
- **Proposed:** internal/ subdirectories (discovery, registry, matching, analysis)
- **Risk:** Large refactoring with potential for breaking changes

#### Implementation Tasks (Future)

- [ ] Create internal/discovery package (parser.go, AST logic)
- [ ] Create internal/registry package (registry.go, data models)
- [ ] Create internal/matching package (linker.go, utils.go)
- [ ] Create internal/analysis package (analyzer.go, checks)
- [ ] Update all imports
- [ ] Maintain backward compatibility for public API
- [ ] Update documentation

---

## Recommended Implementation Order

1. **Week 1:** Recommendation 7 (Deprecated Fields) - Quick win, high impact
2. **Week 1-2:** Recommendation 4 Phase 1 (Fix Analyzer Settings Bug) - Critical bug
3. **Week 2-3:** Recommendation 3 (Registry Decomposition) - Reduces complexity
4. **Week 3-4:** Recommendation 4 Phases 2-4 (Complete Configuration)
5. **Week 5-6:** Recommendation 2 (Strategy Pattern) - Largest refactoring
6. **Ongoing:** Recommendations 5, 6, 1 as time permits
