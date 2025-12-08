This detailed implementation plan transitions the tfprovidertest linter to use Content-Based matching (Inferred Resource Matching) as the primary method for associating tests with resources.
This approach is significantly more robust than file naming conventions because it links a test to a resource based on the actual Terraform configuration being tested.
Phase 1: Update Data Model
First, we need to introduce a new high-priority match type to the registry.
File: registry.go
Task: Add MatchTypeInferred to the MatchType enum and update the String() method.

Go

// registry.go

const (
MatchTypeNone MatchType = iota
// MatchTypeInferred indicates the match was found by parsing the HCL config (Highest Priority)
MatchTypeInferred
MatchTypeFunctionName
MatchTypeFileProximity
MatchTypeFuzzy
)

func (m MatchType) String() string {
switch m {
case MatchTypeInferred:
return "inferred_from_config" //
// ... existing cases ...
}
}

Phase 2: Enhanced AST Parsing
We need to teach the parser to read the Config string inside resource.TestStep and extract the resource type being tested.
File: parser.go
Task 1: Add Regex for HCL Parsing
Add a compiled regular expression to find resource declarations. We only care about the type, not the name.

Go

// parser.go

import (
"regexp"
// ... existing imports
)

// Regex to find 'resource "example_widget" "name" {'
// Captures the resource type (e.g., "example_widget")
var resourceTypeRegex = regexp.MustCompile(`resource\s+"([^"]+)"\s+"[^"]+"\s+\{`)

Task 2: Update extractTestSteps
Modify this function to return the inferred resources found in the config strings.

Go

// parser.go

// Change signature to return inferred resources
func extractTestSteps(body \*ast.BlockStmt) ([]TestStepInfo, []string) {
var steps []TestStepInfo
var inferredResources []string
uniqueInferred := make(map[string]bool)

    // ... existing AST inspection setup ...

    ast.Inspect(body, func(n ast.Node) bool {
        // ... existing check for resource.Test ...

        if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
            if len(call.Args) >= 2 {
                // Pass the map to collect resources
                newSteps := extractStepsFromTestCase(call.Args[1], &stepNumber, uniqueInferred)
                steps = append(steps, newSteps...)
            }
            return false
        }
        return true
    })

    // Convert map to slice
    for res := range uniqueInferred {
        inferredResources = append(inferredResources, res)
    }

    return steps, inferredResources

}

Task 3: Update extractStepsFromTestCase and parseTestStep
Pass the uniqueInferred map down to the step parser so it can analyze the Config fields.

Go

// parser.go

func extractStepsFromTestCase(testCaseExpr ast.Expr, stepNumber \*int, inferred map[string]bool) []TestStepInfo {
// ... existing logic ...

    // When parsing steps:
    step := parseTestStepWithHash(stepExpr, *stepNumber, inferred)
    // ...

}

func parseTestStepWithHash(stepExpr ast.Expr, stepNum int, inferred map[string]bool) TestStepInfo {
// ... existing logic ...

    switch key.Name {
    case "Config":
        step.HasConfig = true
        step.ConfigHash = hashConfigExpr(kv.Value)

        // NEW: Extract resource name from Config string
        if basicLit, ok := kv.Value.(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
            // Remove backticks or quotes
            configContent := strings.Trim(basicLit.Value, "`\"")
            matches := resourceTypeRegex.FindAllStringSubmatch(configContent, -1)
            for _, match := range matches {
                if len(match) > 1 {
                    inferred[match[1]] = true
                }
            }
        }
        // Note: We intentionally skip dynamic configs (fmt.Sprintf) for now
        // as they are harder to parse statically.
    // ...
    }
    return step

}

Task 4: Update ParseTestFileWithConfig
Connect the extracted inferred resources to the TestFunctionInfo struct.

Go

// parser.go

func ParseTestFileWithConfig(...) \*TestFileInfo {
// ...

    // Update call to extractTestSteps
    steps, inferred := extractTestSteps(funcDecl.Body)

    testFunc := TestFunctionInfo{
        // ... existing fields ...
        TestSteps:         steps,
        InferredResources: inferred, // Populate the field
    }
    // ...

}

Phase 3: Logic Update (The Linker)
Now that we have the data, we need to update the Linker to prioritize it.
File: linker.go
Task: Update LinkTestsToResources to check InferredResources first.

Go

// linker.go

func (l \*Linker) LinkTestsToResources() {
// ... existing loop over testFunctions ...

    for _, testFunc := range l.registry.GetAllTestFunctions() {
        linked := false

        // PRIORITY 1: Inferred Content Matching (New)
        // Check if the test explicitly configures a known resource
        for _, inferredName := range testFunc.InferredResources {
            if resource := l.registry.GetResourceOrDataSource(inferredName); resource != nil {
                // Link them!
                testFunc.MatchType = MatchTypeInferred
                testFunc.MatchConfidence = 1.0 // High confidence
                l.registry.LinkTestToResource(inferredName, testFunc)
                linked = true
                // We break here because if a test configures a resource,
                // it is definitively testing that resource.
                break
            }
        }

        if linked {
            continue
        }

        // PRIORITY 2: Function Name Matching (Existing)
        // ... (Keep existing logic for extractResourceNameFromTestFunc) ...

        // PRIORITY 3: File Proximity (Existing)
        // ... (Keep existing logic) ...
    }

}

Verification Checklist
Registry: Verify MatchTypeInferred is available in registry.go.
Parser: Ensure extractTestSteps now returns a slice of strings (resources) alongside steps.
Linker: Ensure the linker iterates InferredResources before attempting to parse the function name.
Test: Create a test file named random_test.go (unrelated name) containing a resource.Test with Config: 'resource "example_widget" ...'. Verify the linter correctly links this test to the example_widget resource.
