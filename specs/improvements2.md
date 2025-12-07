Phase 1: Architecture Refactoring (Function-First Indexing)
This phase decouples test discovery from file naming, allowing the linter to find tests regardless of how the project is structured.

1. Refactor parser.go
   [ ] Modify parseTestFile to process all \_test.go files

Current Behavior: Returns nil immediately if the filename doesn't match resource*\*.go or data_source*\*.go.

New Behavior: Proceed with parsing even if extractResourceNameFromFilePath returns an empty string.

[ ] Update AST Inspection logic

Current: Checks for strings.HasPrefix(name, "Test").

New: Check against Settings.TestNamePatterns (defaulting to TestAcc*, TestResource*, etc.) to support non-standard naming.

[ ] Return Partial Information

If valid test functions are found but no resource name can be derived from the file path, return a TestFileInfo object with ResourceName: "" (empty string) instead of nil.

2. Create linker.go (New Component)
   [ ] Define Linker struct

Should hold a reference to the ResourceRegistry and Settings.

[ ] Implement matchResourceByName(funcName string) \*ResourceInfo

Logic: Strip prefixes (TestAcc, TestResource) and suffixes (\_basic, \_generated).

Normalization: Convert the remaining string (e.g., AwsS3Bucket) to snake_case (aws_s3_bucket) and check if it exists in the registry.

[ ] Implement LinkTestsToResources() method

Iterate through all test functions in the registry.

Strategy 1 (Primary): Call matchResourceByName. If a match is found, link the function to the resource.

Strategy 2 (Secondary): If no name match, check if the function's file has a ResourceName (from the legacy file-based parsing). If so, link it.

Fallback: Mark the function as "Unlinked" (useful for debugging "orphaned tests").

3. Update registry.go
   [ ] Update ResourceRegistry Data Structure

Deprecate: testFiles map[string]\*TestFileInfo (enforces 1:1 mapping).

Add: resourceTests map[string][]\*TestFunctionInfo (allows 1:N mapping).

[ ] Add LinkTestToResource(resourceName string, testFunc \*TestFunctionInfo)

Appends the test function to the slice in resourceTests.

[ ] Update GetUntestedResources

Check len(resourceTests[name]) == 0 instead of checking testFiles.

Phase 2: Logic Refinement (Update Test Verification)
This phase eliminates false positives where "Apply -> Import" tests are incorrectly counted as "Update" tests.

1. Update TestStepInfo in parser.go
   [ ] Add new fields to struct:

HasConfig bool: True if the Config field is present and non-empty.

ImportState bool: True if ImportState is set to true.

[ ] Update parseTestStepWithHash

Populate HasConfig and ImportState during AST parsing.

[ ] Implement IsRealUpdateStep() method

Logic:

Go

func (t \*TestStepInfo) IsRealUpdateStep() bool {
return t.StepNumber > 0 && t.HasConfig && !t.ImportState
} 2. Update analyzer.go (UpdateTestAnalyzer)
[ ] Refactor runUpdateTestAnalyzer

Retrieve Tests: Use registry.GetResourceTests(resource.Name) to get all linked functions.

Iterate Steps: Loop through every step of every linked test function.

Check: if step.IsRealUpdateStep() { found = true; break }.

Report: Only report a diagnostic if found remains false after checking all tests.

Phase 3: Attribute Detection & False Negative Prevention
This phase ensures we don't skip resources that should be tested just because AST analysis is hard.

1. Update analyzer.go Attribute Logic
   [ ] Implement isAttributeUpdatable(attr AttributeInfo)

Logic:

If !attr.Optional, return false (Computed-only attributes generally don't need update tests).

If attr.HasRequiresReplace (detected via AST whitelist), return false.

Default: Return true. (Assume it IS updatable if we aren't sure).

[ ] Update AST Parser for Attributes

Explicitly look for stringplanmodifier.RequiresReplace() and planmodifier.RequiresReplace() in the PlanModifiers slice.

Set HasRequiresReplace = true on the attribute if found.

Phase 4: Helper & Configuration Support
This phase allows the linter to work with custom test frameworks and non-standard project structures.

1. Update Settings Struct
   [ ] Add CustomTestHelpers field

Type: []string

Description: List of functions that wrap resource.Test (e.g., ["acctest.ParallelTest", "utils.TestWrapper"]).

[ ] Add TestNamePatterns field

Type: []string

Default: ["TestAcc*"]

2. Update parser.go Helper Detection
   [ ] Update checkUsesResourceTestWithHelpers

Input: Accept Settings object.

Logic: Inside ast.Inspect:

Check for standard resource.Test.

Check if the call matches any string in Settings.CustomTestHelpers.

If either is true, mark function as a valid test.
