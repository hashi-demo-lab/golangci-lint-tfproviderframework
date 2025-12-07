# Tasks: Terraform Provider Test Coverage Linter - Refactoring & Simplification

**Input**: Improvement feedback from `/workspace/specs/improvements3.md`
**Prerequisites**: Existing implementation in `/workspace/*.go` files
**Goal**: Reduce redundancy, improve performance, and simplify the codebase

**Tests**: All refactoring tasks must maintain existing test coverage - run full test suite after each change

**Organization**: Tasks grouped by improvement area from the feedback document

## Format: `[ID] [P?] [Area] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Area]**: Which improvement area this task belongs to (IMP1-IMP7)
- Include exact file paths in descriptions

---

## Phase 1: Critical Performance - Unified Registry Building (IMP1)

**Goal**: Eliminate repeated AST parsing overhead - currently 5 analyzers each call `buildRegistry()` independently, causing 5x parsing of the entire codebase.

**Current Problem** (analyzer.go):
```go
func runBasicTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
    registry := buildRegistry(pass, settings) // Scans all files
    // ...
}
func runUpdateTestAnalyzer(pass *analysis.Pass) (interface{}, error) {
    registry := buildRegistry(pass, settings) // Scans all files AGAIN
    // ...
}
```

### Tests for Phase 1 (TDD)

- [ ] T101 [P] [IMP1] Write test for registry caching mechanism in /workspace/analyzer_test.go
- [ ] T102 [P] [IMP1] Write benchmark comparing current vs cached registry building in /workspace/analyzer_test.go
- [ ] T103 [IMP1] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 1

**Option A: Single MasterAnalyzer (Simpler)**

- [ ] T104 [IMP1] Create MasterAnalyzer struct that wraps all 5 analyzers in /workspace/analyzer.go
- [ ] T105 [IMP1] Implement single `buildRegistry()` call in MasterAnalyzer.Run() in /workspace/analyzer.go
- [ ] T106 [IMP1] Pass shared registry to individual analyzer logic functions in /workspace/analyzer.go
- [ ] T107 [IMP1] Refactor `run*Analyzer` functions to accept registry parameter instead of building it in /workspace/analyzer.go
- [ ] T108 [IMP1] Update BuildAnalyzers() to return MasterAnalyzer in /workspace/tfprovidertest.go

**Option B: go/analysis Facts Mechanism (Idiomatic)**

- [ ] T109 [IMP1] Create RegistryBuilderAnalyzer that exports registry via Facts in /workspace/analyzer.go
- [ ] T110 [IMP1] Add `Requires: []*analysis.Analyzer{RegistryBuilderAnalyzer}` to all 5 analyzers in /workspace/analyzer.go
- [ ] T111 [IMP1] Import registry fact in each analyzer's Run function in /workspace/analyzer.go

### Verification for Phase 1

- [ ] T112 [IMP1] Verify all tests pass (green phase)
- [ ] T113 [IMP1] Run benchmark to confirm 5x performance improvement
- [ ] T114 [IMP1] Refactor for clarity (refactor phase)

**Checkpoint**: Registry is built once, shared across all analyzers

---

## Phase 2: Centralize File Name Parsing (IMP2)

**Goal**: Eliminate duplicate file name parsing logic between parser.go and linker.go

**Current Problem**:
- `parser.go:extractResourceNameFromFilePath()` - strips resource_, data_source_, _test.go
- `linker.go:matchByFileProximity()` - performs nearly identical string manipulation

### Tests for Phase 2 (TDD)

- [ ] T201 [P] [IMP2] Write unit tests for unified ExtractResourceNameFromPath() in /workspace/utils_test.go
- [ ] T202 [P] [IMP2] Test edge cases: resource_, data_source_, ephemeral_, _resource, _data_source suffixes
- [ ] T203 [IMP2] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 2

- [ ] T204 [IMP2] Create ExtractResourceNameFromPath() utility in /workspace/utils.go
- [ ] T205 [IMP2] Refactor parser.go:extractResourceNameFromFilePath() to use utility in /workspace/parser.go
- [ ] T206 [IMP2] Refactor linker.go:matchByFileProximity() to use utility in /workspace/linker.go
- [ ] T207 [IMP2] Remove duplicate code from both files

### Verification for Phase 2

- [ ] T208 [IMP2] Verify all tests pass (green phase)
- [ ] T209 [IMP2] Verify no behavior change via integration tests
- [ ] T210 [IMP2] Refactor for clarity (refactor phase)

**Checkpoint**: Single source of truth for file name parsing

---

## Phase 3: Unify Function Name Matching Logic (IMP3)

**Goal**: Consolidate function name matching between registry.go and linker.go

**Current Problem**:
- `registry.go:ClassifyTestFunctionMatch()` - hardcoded patterns for diagnostics
- `linker.go:matchResourceByName()` - different list of testFunctionPrefixes/Suffixes for linking

This creates inconsistent behavior where Linker may link a test, but Diagnostic says "Why Not Matched: pattern mismatch"

### Tests for Phase 3 (TDD)

- [ ] T301 [P] [IMP3] Write test verifying ClassifyTestFunctionMatch and matchResourceByName produce consistent results
- [ ] T302 [P] [IMP3] Test that linked tests always pass diagnostic classification
- [ ] T303 [IMP3] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 3

- [ ] T304 [IMP3] Create shared testFunctionPatterns constant/config in /workspace/utils.go
- [ ] T305 [IMP3] Refactor registry.go:ClassifyTestFunctionMatch() to use shared patterns in /workspace/registry.go
- [ ] T306 [IMP3] Refactor linker.go to use same shared patterns in /workspace/linker.go
- [ ] T307 [IMP3] Ensure diagnostic tool uses Linker's logic for consistency

### Verification for Phase 3

- [ ] T308 [IMP3] Verify all tests pass (green phase)
- [ ] T309 [IMP3] Verify linked tests never show "pattern mismatch" in diagnostics
- [ ] T310 [IMP3] Refactor for clarity (refactor phase)

**Checkpoint**: Consistent matching behavior across linking and diagnostics

---

## Phase 4: Remove Legacy testFiles Map (IMP4)

**Goal**: Eliminate redundant testFiles map from ResourceRegistry

**Current Problem** (registry.go):
```go
testFiles      map[string]*TestFileInfo   // Legacy: 1:1 relationship
resourceTests  map[string][]*TestFunctionInfo // New: 1:N relationship
```

The transition from "File-Based" to "Function-Based" linking left both maps. Now `testFiles` is redundant because `resourceTests` can express the 1:N relationship properly.

### Tests for Phase 4 (TDD)

- [ ] T401 [P] [IMP4] Write tests verifying GetResourceTests returns all linked tests in /workspace/registry_test.go
- [ ] T402 [P] [IMP4] Write tests for backward-compatible behavior without testFiles in /workspace/registry_test.go
- [ ] T403 [IMP4] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 4

- [ ] T404 [IMP4] Audit all usages of registry.GetTestFile() in /workspace/analyzer.go
- [ ] T405 [IMP4] Audit all usages of registry.GetAllTestFiles() in /workspace/analyzer.go
- [ ] T406 [IMP4] Replace GetTestFile() calls with GetResourceTests() where appropriate in /workspace/analyzer.go
- [ ] T407 [IMP4] Deprecate testFiles map in ResourceRegistry struct in /workspace/registry.go
- [ ] T408 [IMP4] Remove RegisterTestFile() method or repurpose it in /workspace/registry.go
- [ ] T409 [IMP4] Update GetUntestedResources() to only use resourceTests in /workspace/registry.go

### Verification for Phase 4

- [ ] T410 [IMP4] Verify all tests pass (green phase)
- [ ] T411 [IMP4] Run integration tests against terraform-provider-time
- [ ] T412 [IMP4] Refactor for clarity (refactor phase)

**Checkpoint**: Single source of truth for test-to-resource associations

---

## Phase 5: Simplify Parser Function Chain (IMP5)

**Goal**: Collapse 4 redundant parser functions into 1 with configuration struct

**Current Problem** (parser.go):
```go
parseTestFile
parseTestFileWithHelpers
parseTestFileWithHelpersAndLocals
parseTestFileWithSettings
```

### Tests for Phase 5 (TDD)

- [ ] T501 [P] [IMP5] Write tests for ParserConfig struct with various option combinations in /workspace/parser_test.go
- [ ] T502 [P] [IMP5] Ensure backward compatibility tests for existing ParseTestFile() API in /workspace/parser_test.go
- [ ] T503 [IMP5] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 5

- [ ] T504 [IMP5] Define ParserConfig struct in /workspace/parser.go:
```go
type ParserConfig struct {
    CustomHelpers    []string
    LocalHelpers     []LocalHelper
    TestNamePatterns []string
}
```
- [ ] T505 [IMP5] Create single ParseTestFileWithConfig() function in /workspace/parser.go
- [ ] T506 [IMP5] Update parseTestFile() to call ParseTestFileWithConfig with empty config in /workspace/parser.go
- [ ] T507 [IMP5] Deprecate intermediate functions with comments pointing to new API in /workspace/parser.go
- [ ] T508 [IMP5] Update buildRegistry() to use new ParseTestFileWithConfig() in /workspace/parser.go

### Verification for Phase 5

- [ ] T509 [IMP5] Verify all tests pass (green phase)
- [ ] T510 [IMP5] Verify public API backward compatibility
- [ ] T511 [IMP5] Refactor for clarity (refactor phase)

**Checkpoint**: Single configurable parser function

---

## Phase 6: Simplify Settings Boolean Toggles (IMP6)

**Goal**: Make mandatory matching strategies non-configurable

**Current Problem** (settings.go):
```go
EnableFunctionMatching   bool
EnableFileBasedMatching  bool
EnableFuzzyMatching      bool
```

FunctionMatching and FileBasedMatching always run sequentially anyway. Only FuzzyMatching has real configurability needs (it's expensive and can produce false positives).

### Tests for Phase 6 (TDD)

- [ ] T601 [P] [IMP6] Write tests verifying FunctionMatching always runs regardless of setting in /workspace/settings_test.go
- [ ] T602 [P] [IMP6] Write tests verifying FileBasedMatching always runs as fallback in /workspace/settings_test.go
- [ ] T603 [IMP6] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 6

- [ ] T604 [IMP6] Remove EnableFunctionMatching from Settings struct in /workspace/settings.go
- [ ] T605 [IMP6] Remove EnableFileBasedMatching from Settings struct in /workspace/settings.go
- [ ] T606 [IMP6] Update linker.go to always run function and file-based matching in /workspace/linker.go
- [ ] T607 [IMP6] Keep only EnableFuzzyMatching as user-configurable option in /workspace/settings.go
- [ ] T608 [IMP6] Update documentation to reflect mandatory strategies

### Verification for Phase 6

- [ ] T609 [IMP6] Verify all tests pass (green phase)
- [ ] T610 [IMP6] Verify .golangci.yml example works without removed options
- [ ] T611 [IMP6] Refactor for clarity (refactor phase)

**Checkpoint**: Simplified settings with only meaningful toggles

---

## Phase 7: Merge Resource and DataSource Maps (IMP7)

**Goal**: Use single definitions map instead of separate resources/dataSources maps

**Current Problem** (registry.go):
```go
resources    map[string]*ResourceInfo
dataSources  map[string]*ResourceInfo
```

Since ResourceInfo already has `IsDataSource bool`, separate maps complicate lookups. The Linker already has to merge them for matching.

### Tests for Phase 7 (TDD)

- [ ] T701 [P] [IMP7] Write tests for unified GetAllDefinitions() method in /workspace/registry_test.go
- [ ] T702 [P] [IMP7] Write tests for filtered GetResources() and GetDataSources() in /workspace/registry_test.go
- [ ] T703 [IMP7] Verify tests FAIL before implementation (red phase)

### Implementation for Phase 7

- [ ] T704 [IMP7] Add single definitions map[string]*ResourceInfo to ResourceRegistry in /workspace/registry.go
- [ ] T705 [IMP7] Update RegisterResource() to use definitions map in /workspace/registry.go
- [ ] T706 [IMP7] Add GetAllDefinitions() that returns full map in /workspace/registry.go
- [ ] T707 [IMP7] Refactor GetAllResources() to filter definitions by IsDataSource==false in /workspace/registry.go
- [ ] T708 [IMP7] Refactor GetAllDataSources() to filter definitions by IsDataSource==true in /workspace/registry.go
- [ ] T709 [IMP7] Remove resources and dataSources maps from struct in /workspace/registry.go
- [ ] T710 [IMP7] Simplify linker.go:LinkTestsToResources() to use GetAllDefinitions() in /workspace/linker.go

### Verification for Phase 7

- [ ] T711 [IMP7] Verify all tests pass (green phase)
- [ ] T712 [IMP7] Run integration tests against all validation providers
- [ ] T713 [IMP7] Refactor for clarity (refactor phase)

**Checkpoint**: Single map for all resource definitions

---

## Phase 8: Integration & Validation

**Purpose**: Validate all improvements work together correctly

- [ ] T801 [P] Run full test suite: `go test ./... -v`
- [ ] T802 [P] Run benchmarks: `go test ./... -bench=. -benchmem`
- [ ] T803 [P] Run linter against terraform-provider-time
- [ ] T804 [P] Run linter against terraform-provider-tls
- [ ] T805 [P] Run linter against terraform-provider-http
- [ ] T806 [P] Run linter against terraform-provider-aap
- [ ] T807 Document performance improvements in /workspace/validation/BENCHMARKS.md
- [ ] T808 Update README.md with simplified configuration options
- [ ] T809 Run `go fmt`, `go vet`, `golangci-lint` on all files
- [ ] T810 Create git commit with descriptive message

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (IMP1)**: No dependencies - can start immediately (HIGHEST PRIORITY - 5x perf gain)
- **Phase 2 (IMP2)**: No dependencies - can run in parallel with Phase 1
- **Phase 3 (IMP3)**: No dependencies - can run in parallel with Phases 1-2
- **Phase 4 (IMP4)**: Depends on Phase 1 (registry changes)
- **Phase 5 (IMP5)**: No dependencies - can run in parallel with Phases 1-3
- **Phase 6 (IMP6)**: Depends on Phase 3 (linker changes)
- **Phase 7 (IMP7)**: Depends on Phase 4 (registry changes)
- **Phase 8**: Depends on all previous phases

### Parallel Opportunities

```bash
# Can run simultaneously:
Phase 1 (IMP1) + Phase 2 (IMP2) + Phase 3 (IMP3) + Phase 5 (IMP5)

# Then:
Phase 4 (IMP4) after Phase 1
Phase 6 (IMP6) after Phase 3
Phase 7 (IMP7) after Phase 4

# Finally:
Phase 8 after all phases complete
```

### Risk Assessment

| Phase | Risk | Mitigation |
|-------|------|------------|
| IMP1 | High - Core architecture change | Comprehensive testing, Option A simpler |
| IMP2 | Low - Utility extraction | Pure refactor, no behavior change |
| IMP3 | Medium - Behavior consistency | Test linked tests never fail diagnostics |
| IMP4 | Medium - Data model change | Careful audit of all usages |
| IMP5 | Low - API consolidation | Backward-compatible wrappers |
| IMP6 | Low - Settings simplification | Document removed options |
| IMP7 | Medium - Data model change | Careful audit of all usages |

---

## Implementation Strategy

### Recommended Order (Sequential)

1. **Phase 1 (IMP1)** - Critical performance fix, 5x improvement
2. **Phase 5 (IMP5)** - Parser cleanup, reduces cognitive load
3. **Phase 2 (IMP2)** - File name parsing, quick win
4. **Phase 3 (IMP3)** - Function matching, improves consistency
5. **Phase 4 (IMP4)** - Remove legacy testFiles
6. **Phase 7 (IMP7)** - Merge resource maps
7. **Phase 6 (IMP6)** - Simplify settings
8. **Phase 8** - Validation

### Quick Wins First (Parallel Team)

With multiple developers:

1. Developer A: Phase 1 (IMP1) - Performance
2. Developer B: Phase 2 (IMP2) + Phase 3 (IMP3) - Utility consolidation
3. Developer C: Phase 5 (IMP5) - Parser simplification

After Phase 1 completes:
4. Developer A: Phase 4 (IMP4) + Phase 7 (IMP7) - Registry cleanup

After Phase 3 completes:
5. Developer B: Phase 6 (IMP6) - Settings

---

## Notes

- TDD REQUIRED: All tests must FAIL before implementation (red-green-refactor)
- Commit after each phase completion
- Run full test suite after each phase
- Keep backward compatibility for public APIs
- Document breaking changes in CHANGELOG.md
- Each phase should be independently deployable
