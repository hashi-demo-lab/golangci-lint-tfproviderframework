Executive Summary
The current "Foundational Implementation" is clean, well-structured, and strictly follows TDD, which is excellent. However, the core design decision to rely on File-Based Matching (resource_widget.go ↔ resource_widget_test.go) as the primary association mechanism is critically fragile. It fails for composite files (multiple resources in one file) and non-standard project structures, which are common in mature providers.

1. Critical Design Flaw: Resource-to-Test Association
   Current Design: The buildRegistry function in parser.go (L231-241) only registers a test file if its filename allows it to be mapped directly to a known resource.

Scenario A (Fails): A provider defines ResourceA and ResourceB in internal/provider/resources.go. The parser extracts "resources" as the name from the filename. Since no resource is named "resources", the test file is ignored. ResourceA and ResourceB are reported as untested.

Scenario B (Fails): Tests are grouped in internal/provider/all_tests_test.go. The filename doesn't match any specific resource, so all tests inside are ignored.

Challenge: You cannot assume a 1:1 file mapping. Terraform providers often group simple resources or split complex ones into multiple files (resource_widget.go, resource_widget_helpers.go).

Recommendation: Shift from File-First to Function-First indexing.

Index All Test Functions: When parsing \_test.go files, extract all TestAcc\* functions into a package-level index, regardless of the filename.

Fuzzy Matching Strategy:

Primary: Match TestAcc<ResourceName>... function names to ResourceRegistry entries.

Secondary: If file-based matching works (resource_widget.go + resource_widget_test.go), use it to narrow the search scope, but do not rely on it exclusively.

Update ResourceRegistry: It should map ResourceName → []TestFunctionInfo directly, decoupling the strict dependency on TestFileInfo.

2. Logic Gap: Update Test Verification (User Story 2)
   Current Design (Planned): The runUpdateTestAnalyzer (analyzer.go L127) checks if len(testFunc.TestSteps) >= 2.

Challenge: This heuristic is prone to False Positives.

The "Import is not Update" Problem: A common test pattern is Step 1 (Apply) -> Step 2 (Import). This has 2 steps but tests import, not update.

The "No-Op" Problem: A test might run the same config twice to check idempotency. This is not an update test (attribute modification).

Recommendation: Refine the heuristic for TestStepInfo in parser.go:

Explicitly Detect Update Intent: A step is an "Update Step" only if:

It is not the first step (StepNumber > 0).

It defines Config.

ImportState is false.

(Optional/Advanced) The Config string is different from the previous step's Config.

3. Logic Gap: Updatable Attribute Detection
   Current Design: Intends to check Schema for attributes without RequiresReplace.

Challenge: In terraform-plugin-framework, attributes can be defined via schema.StringAttribute, schema.Int64Attribute, etc. These are struct literals.

Pointer Analysis: PlanModifiers are often slices of interface implementations (e.g., []planmodifier.String{stringplanmodifier.RequiresReplace()}).

AST Limitation: Parsing stringplanmodifier.RequiresReplace() via AST is reasonably easy, but custom modifiers or helper functions returning modifiers will be missed.

Recommendation: Accept that AST analysis is imperfect here.

Whitelist Standard Modifiers: Explicitly look for RequiresReplace in the AST of the PlanModifiers slice.

Conservative Default: If PlanModifiers is populated but cannot be fully resolved (e.g., it uses a variable or helper function), assume the attribute is updatable to avoid false negatives (missing tests are worse than extra tests), OR provide a comment-based suppression mechanism (//lint:ignore tfprovider-update-test).

4. Missing Feature: "Sweeper" & Helper Handling
   Current Design: ExcludeSweeperFiles handles files named \*\_sweeper.go.

Challenge: Many providers define acceptance test helpers in test_utils.go or similar. These might contain resource.Test wrappers.

If a resource test calls myProviderTest(t, case), the current parser (L300 in parser.go) only checks resource.Test or resource.ParallelTest.

The CustomTestHelpers setting exists, but the parser logic for checkUsesResourceTestWithHelpers needs to be robust enough to handle helpers defined in the same package versus imported packages.

Recommendation: Ensure checkUsesResourceTestWithHelpers handles:

Local Helpers: Function calls TestHelper(...) where TestHelper is defined in the same package and calls resource.Test. (Recursive check might be expensive, so just checking the name against CustomTestHelpers config is a pragmatic tradeoff).

5. Data Model Refinement
   Current:

Go
type TestFileInfo struct {
ResourceName string // <--- This forces 1:1 mapping
...
}
Proposed:

Go
type TestFileInfo struct {
FilePath string
// ResourceName string <--- Remove this constraint
TestFunctions []TestFunctionInfo
}

type ResourceRegistry struct {
...
// Map ResourceName to the specific functions that test it,
// potentially spanning multiple files.
ResourceTests map[string][]\*TestFunctionInfo
}
Summary of Actions
To improve the design before proceeding to User Story 2:

Refactor parser.go: Remove the dependency on extractResourceNameFromFilePath for ignoring files. Parse all test files.

Refactor registry.go: Implement a "Linker" pass. After parsing all resources and all test functions, run a linking step that associates functions to resources based on naming conventions (TestAcc<Resource>...), using file proximity as a tie-breaker/hint only.

Update TestStepInfo: Add HasConfig and IsImport flags to accurately distinguish Update steps from Import steps.
