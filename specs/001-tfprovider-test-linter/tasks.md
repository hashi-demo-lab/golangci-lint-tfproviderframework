# Tasks: Terraform Provider Test Coverage Linter

**Input**: Design documents from `/workspace/specs/001-tfprovider-test-linter/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: TDD approach required - all tests written FIRST before implementation

**Organization**: Tasks grouped by user story (linting rule) to enable independent implementation and testing

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1-US5)
- Include exact file paths in descriptions

## Path Conventions

This is a single-project golangci-lint plugin with all code at repository root:
- `tfprovidertest.go` - Core analyzer implementation
- `tfprovidertest_test.go` - Analyzer tests
- `testdata/` - Test fixtures

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and Go module structure

- [X] T001 Create Go module with go.mod in /workspace/
- [X] T002 Add dependencies: golang.org/x/tools v0.31.0, plugin-module-register v0.1.1, testify v1.10.0
- [X] T003 [P] Create .golangci.example.yml configuration template in /workspace/
- [X] T004 [P] Create .custom-gcl.yml for local development in /workspace/
- [X] T005 [P] Create README.md with installation and usage docs in /workspace/

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core shared infrastructure that ALL analyzers depend on

**⚠️ CRITICAL**: No user story analyzer work can begin until this phase is complete

### Tests for Foundational Components (TDD) ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T006 [P] Test for Settings defaults in /workspace/tfprovidertest_test.go
- [X] T007 [P] Test for ResourceRegistry operations in /workspace/tfprovidertest_test.go
- [X] T008 [P] Test for AST resource detection in /workspace/tfprovidertest_test.go
- [X] T009 [P] Test for AST data source detection in /workspace/tfprovidertest_test.go
- [X] T010 [P] Test for test file parsing in /workspace/tfprovidertest_test.go

### Foundational Implementation

- [X] T011 [P] Implement Settings struct with defaults in /workspace/tfprovidertest.go
- [X] T012 [P] Implement ResourceRegistry with thread-safe maps in /workspace/tfprovidertest.go
- [X] T013 Implement AST parser for detecting framework resources in /workspace/tfprovidertest.go
- [X] T014 Implement AST parser for detecting data sources in /workspace/tfprovidertest.go
- [X] T015 Implement AST parser for extracting schema attributes in /workspace/tfprovidertest.go
- [X] T016 Implement test file parser for TestAcc functions in /workspace/tfprovidertest.go
- [X] T017 Implement Plugin struct with New(), BuildAnalyzers(), GetLoadMode() in /workspace/tfprovidertest.go
- [X] T018 Add plugin registration via init() and register.Plugin() in /workspace/tfprovidertest.go

**Checkpoint**: Foundation ready - analyzer implementation can now begin in parallel

---

## Phase 3: User Story 1 - Identify Untested Resources (Priority: P1) 🎯 MVP

**Goal**: Detect resources and data sources that lack basic acceptance tests

**Independent Test**: Can be fully tested with a provider having one resource with no test file. Linter reports missing test with actionable feedback.

### Tests for User Story 1 (TDD) ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [X] T019 [P] [US1] Create testdata fixture: resource with no test file in /workspace/testdata/src/testlintdata/basic_missing/
- [X] T020 [P] [US1] Create testdata fixture: resource with test file but no TestAcc function in /workspace/testdata/src/testlintdata/basic_missing/
- [X] T021 [P] [US1] Create testdata fixture: well-tested resource (passing case) in /workspace/testdata/src/testlintdata/basic_passing/
- [X] T022 [P] [US1] Create testdata fixture: data source with no test in /workspace/testdata/src/testlintdata/basic_missing/
- [X] T023 [US1] Write TestBasicTestCoverage using analysistest.Run() in /workspace/tfprovidertest_test.go
- [X] T024 [US1] Verify test FAILS (red phase)

### Implementation for User Story 1

- [X] T025 [US1] Implement BasicTestAnalyzer with Name, Doc, Run fields in /workspace/tfprovidertest.go
- [X] T026 [US1] Implement Run function: traverse AST, build ResourceRegistry in /workspace/tfprovidertest.go
- [X] T027 [US1] Implement Run function: match resources to test files in /workspace/tfprovidertest.go
- [X] T028 [US1] Implement Run function: detect missing test files via GetUntestedResources() in /workspace/tfprovidertest.go
- [X] T029 [US1] Implement Run function: detect missing TestAcc functions in /workspace/tfprovidertest.go
- [X] T030 [US1] Implement pass.Report() calls with actionable messages in /workspace/tfprovidertest.go
- [X] T031 [US1] Verify TestBasicTestCoverage PASSES (green phase)
- [X] T032 [US1] Refactor for clarity and performance (refactor phase)

**Checkpoint**: Basic test detection fully functional - can detect untested resources independently

---

## Phase 4: User Story 2 - Enforce Update Test Coverage (Priority: P2)

**Goal**: Verify resources with updatable attributes have multi-step update tests

**Independent Test**: Can be fully tested by analyzing resource schema for updatable attributes and checking for multi-step tests

### Tests for User Story 2 (TDD) ⚠️

- [X] T033 [P] [US2] Create testdata fixture: resource with updatable attrs, single-step test in /workspace/testdata/src/testlintdata/update_missing/
- [X] T034 [P] [US2] Create testdata fixture: resource with RequiresReplace attrs (passing) in /workspace/testdata/src/testlintdata/update_passing/
- [X] T035 [P] [US2] Create testdata fixture: resource with multi-step update test (passing) in /workspace/testdata/src/testlintdata/update_passing/
- [X] T036 [US2] Write TestUpdateTestCoverage using analysistest.Run() in /workspace/tfprovidertest_test.go
- [X] T037 [US2] Verify test FAILS (red phase)

### Implementation for User Story 2

- [X] T038 [US2] Implement UpdateTestAnalyzer with Name, Doc, Run fields in /workspace/tfprovidertest.go
- [X] T039 [US2] Implement attribute analysis: detect updatable attributes (no RequiresReplace) in /workspace/tfprovidertest.go
- [X] T040 [US2] Implement test step analysis: count steps, compare configs in /workspace/tfprovidertest.go
- [X] T041 [US2] Implement HasUpdatableAttributes() check in /workspace/tfprovidertest.go
- [X] T042 [US2] Implement HasUpdateTest() check via TestFileInfo in /workspace/tfprovidertest.go
- [X] T043 [US2] Implement pass.Report() for missing update tests in /workspace/tfprovidertest.go
- [X] T044 [US2] Verify TestUpdateTestCoverage PASSES (green phase)
- [X] T045 [US2] Refactor for clarity (refactor phase)

**Checkpoint**: Update test detection fully functional - independent of other analyzers

---

## Phase 5: User Story 3 - Validate Import State Testing (Priority: P2)

**Goal**: Ensure resources implementing ImportState have import tests

**Independent Test**: Can be fully tested by detecting ImportState method and checking for ImportState: true test steps

### Tests for User Story 3 (TDD) ⚠️

- [X] T046 [P] [US3] Create testdata fixture: resource with ImportState method, no import test in /workspace/testdata/src/testlintdata/import_missing/
- [X] T047 [P] [US3] Create testdata fixture: resource with ImportState and valid import test (passing) in /workspace/testdata/src/testlintdata/import_passing/
- [X] T048 [P] [US3] Create testdata fixture: resource without ImportState (passing) in /workspace/testdata/src/testlintdata/import_passing/
- [X] T049 [US3] Write TestImportTestCoverage using analysistest.Run() in /workspace/tfprovidertest_test.go
- [X] T050 [US3] Verify test FAILS (red phase)

### Implementation for User Story 3

- [X] T051 [US3] Implement ImportTestAnalyzer with Name, Doc, Run fields in /workspace/tfprovidertest.go
- [X] T052 [US3] Implement AST detection of ImportState method implementation in /workspace/tfprovidertest.go
- [X] T053 [US3] Implement test step analysis: find ImportState: true, ImportStateVerify: true in /workspace/tfprovidertest.go
- [X] T054 [US3] Implement HasImportTest() check via TestStepInfo in /workspace/tfprovidertest.go
- [X] T055 [US3] Implement pass.Report() for missing import tests in /workspace/tfprovidertest.go
- [X] T056 [US3] Verify TestImportTestCoverage PASSES (green phase)
- [X] T057 [US3] Refactor for clarity (refactor phase)

**Checkpoint**: Import test detection fully functional - independent of other analyzers

---

## Phase 6: User Story 4 - Enforce Error Case Testing (Priority: P3)

**Goal**: Verify resources have tests for invalid configurations using ExpectError

**Independent Test**: Can be fully tested by checking for validation rules in schema and ExpectError in test steps

### Tests for User Story 4 (TDD) ⚠️

- [X] T058 [P] [US4] Create testdata fixture: resource with validators, no error test in /workspace/testdata/src/testlintdata/error_missing/
- [X] T059 [P] [US4] Create testdata fixture: resource with validators and ExpectError test (passing) in /workspace/testdata/src/testlintdata/error_passing/
- [X] T060 [P] [US4] Create testdata fixture: resource with no validators (passing) in /workspace/testdata/src/testlintdata/error_passing/
- [X] T061 [US4] Write TestErrorTestCoverage using analysistest.Run() in /workspace/tfprovidertest_test.go
- [X] T062 [US4] Verify test FAILS (red phase)

### Implementation for User Story 4

- [X] T063 [US4] Implement ErrorTestAnalyzer with Name, Doc, Run fields in /workspace/tfprovidertest.go
- [X] T064 [US4] Implement attribute analysis: detect Required, Validators, ConflictsWith in /workspace/tfprovidertest.go
- [X] T065 [US4] Implement HasValidationRules() check in /workspace/tfprovidertest.go
- [X] T066 [US4] Implement test step analysis: find ExpectError fields in /workspace/tfprovidertest.go
- [X] T067 [US4] Implement HasErrorTest() check via TestStepInfo in /workspace/tfprovidertest.go
- [X] T068 [US4] Implement pass.Report() for missing error tests in /workspace/tfprovidertest.go
- [X] T069 [US4] Verify TestErrorTestCoverage PASSES (green phase)
- [X] T070 [US4] Refactor for clarity (refactor phase)

**Checkpoint**: Error test detection fully functional - independent of other analyzers

---

## Phase 7: User Story 5 - Validate Test Quality with State Checks (Priority: P3)

**Goal**: Ensure test steps use proper state validation functions

**Independent Test**: Can be fully tested by analyzing TestStep blocks for Check fields with validation functions

### Tests for User Story 5 (TDD) ⚠️

- [X] T071 [P] [US5] Create testdata fixture: test with steps missing Check fields in /workspace/testdata/src/testlintdata/checks_missing/
- [X] T072 [P] [US5] Create testdata fixture: test with empty ComposeTestCheckFunc (passing) in /workspace/testdata/src/testlintdata/checks_passing/
- [X] T073 [P] [US5] Create testdata fixture: test with proper check functions (passing) in /workspace/testdata/src/testlintdata/checks_passing/
- [X] T074 [US5] Write TestStateCheckValidation using analysistest.Run() in /workspace/tfprovidertest_test.go
- [X] T075 [US5] Verify test FAILS (red phase)

### Implementation for User Story 5

- [X] T076 [US5] Implement StateCheckAnalyzer with Name, Doc, Run fields in /workspace/tfprovidertest.go
- [X] T077 [US5] Implement test step analysis: detect Check field presence in /workspace/tfprovidertest.go
- [X] T078 [US5] Implement check function parsing: extract TestCheckResourceAttr, TestCheckResourceAttrSet in /workspace/tfprovidertest.go
- [X] T079 [US5] Implement HasStateChecks() validation logic in /workspace/tfprovidertest.go
- [X] T080 [US5] Implement pass.Report() for missing check functions in /workspace/tfprovidertest.go
- [X] T081 [US5] Verify TestStateCheckValidation PASSES (green phase)
- [X] T082 [US5] Refactor for clarity (refactor phase)

**Checkpoint**: State check validation fully functional - all 5 analyzers complete

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Integration, validation, and finalization

- [X] T083 [P] Validate all 5 analyzers return from BuildAnalyzers() in /workspace/tfprovidertest.go
- [X] T084 [P] Validate Settings configuration enables/disables analyzers in /workspace/tfprovidertest.go
- [ ] T085 [P] Test plugin integration with .custom-gcl.yml locally
- [ ] T086 Run linter against terraform-provider-time (expect 0 false positives, <10s runtime)
- [ ] T087 Run linter against terraform-provider-tls (expect 0 false positives)
- [ ] T088 Run linter against terraform-provider-http (expect 0 false positives)
- [ ] T089 Run linter against terraform-provider-aap (expect 3+ gaps detected)
- [ ] T090 [P] Document validation results in /workspace/validation/
- [X] T091 [P] Add performance benchmarks using testing.B in /workspace/tfprovidertest_test.go
- [ ] T092 [P] Update README.md with validation results and performance metrics
- [X] T093 Code cleanup: remove debug statements, add godoc comments
- [X] T094 Run go fmt, go vet, golangci-lint on plugin code
- [X] T095 Verify quickstart.md examples work end-to-end

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-7)**: All depend on Foundational phase completion
  - User stories can then proceed in parallel (US1-US5)
  - Or sequentially in priority order (P1 → P2 → P2 → P3 → P3)
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational - Independent of US1
- **User Story 3 (P2)**: Can start after Foundational - Independent of US1, US2
- **User Story 4 (P3)**: Can start after Foundational - Independent of US1-US3
- **User Story 5 (P3)**: Can start after Foundational - Independent of US1-US4

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD red-green-refactor)
- Testdata fixtures before test functions
- Test failure verification before implementation
- Implementation before test pass verification
- Test pass before refactoring
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tests marked [P] can run in parallel
- All Foundational implementations marked [P] can run in parallel (after tests fail)
- Once Foundational phase completes, all 5 user stories can start in parallel
- Within each story, testdata fixtures marked [P] can be created in parallel
- Validation tasks marked [P] in Polish phase can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all testdata fixtures together (TDD - before tests):
Task: "Create testdata fixture: resource with no test file"
Task: "Create testdata fixture: resource with test file but no TestAcc function"
Task: "Create testdata fixture: well-tested resource (passing case)"
Task: "Create testdata fixture: data source with no test"

# Then write test function that uses these fixtures:
Task: "Write TestBasicTestCoverage using analysistest.Run()"

# Verify it fails (red phase), then implement:
Task: "Implement BasicTestAnalyzer with Name, Doc, Run fields"
```

---

## Parallel Example: Foundational Phase

```bash
# Launch all tests together (TDD - write tests first):
Task: "Test for Settings defaults"
Task: "Test for ResourceRegistry operations"
Task: "Test for AST resource detection"
Task: "Test for AST data source detection"
Task: "Test for test file parsing"

# Verify all tests fail, then implement in parallel:
Task: "Implement Settings struct with defaults"
Task: "Implement ResourceRegistry with thread-safe maps"
```

---

## Parallel Example: All User Stories After Foundation

```bash
# Once Phase 2 (Foundational) is complete, all 5 stories can start:
Team Member 1: Phase 3 (US1 - Basic Test Detection)
Team Member 2: Phase 4 (US2 - Update Test Detection)
Team Member 3: Phase 5 (US3 - Import Test Detection)
Team Member 4: Phase 6 (US4 - Error Test Detection)
Team Member 5: Phase 7 (US5 - State Check Detection)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (Basic test detection)
4. **STOP and VALIDATE**: Test on terraform-provider-time
5. Deploy/demo basic linter

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test on real provider → Deploy (MVP!)
3. Add User Story 2 → Test independently → Deploy
4. Add User Story 3 → Test independently → Deploy
5. Add User Story 4 → Test independently → Deploy
6. Add User Story 5 → Test independently → Deploy
7. Each analyzer adds value without breaking previous analyzers

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together (TDD: tests first!)
2. Once Foundational is done:
   - Developer A: User Story 1 (Basic tests)
   - Developer B: User Story 2 (Update tests)
   - Developer C: User Story 3 (Import tests)
   - Developer D: User Story 4 (Error tests)
   - Developer E: User Story 5 (State checks)
3. Stories complete and integrate independently via BuildAnalyzers()

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific analyzer for traceability
- Each analyzer (user story) is independently completable and testable
- TDD REQUIRED: All tests must FAIL before implementation (red-green-refactor)
- Testdata fixtures use // want comments for analysistest expectations
- Commit after each TDD cycle (red → green → refactor)
- Stop at any checkpoint to validate analyzer independently
- Avoid: implementing before tests fail, skipping refactor phase, cross-analyzer dependencies
