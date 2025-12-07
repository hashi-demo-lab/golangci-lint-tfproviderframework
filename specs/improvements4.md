Based on the files you provided (parser.go and registry.go) and the previous gap analysis, here is a detailed technical implementation plan.
This plan focuses on modifying the AST parsing logic to capture the missing metadata (Drift Checks, Plan Checks, and Sweepers) so the Analyzers can enforce them.
Phase 1: Data Model Updates (registry.go)
We need to update the structs to store the new metadata discovered during parsing.
File: registry.go
Update TestFunctionInfo struct
Add HasCheckDestroy to track if the resource.TestCase includes a destruction check.
Go
type TestFunctionInfo struct {
// ... existing fields ...
HasCheckDestroy bool // NEW: Tracks presence of CheckDestroy in resource.TestCase
// ...
}

Update TestStepInfo struct
Add HasPlanCheck to support Terraform Plugin Framework's ConfigPlanChecks.
Go
type TestStepInfo struct {
// ... existing fields ...
HasPlanCheck bool // NEW: Tracks presence of ConfigPlanChecks
// ...
}

Phase 2: Parser Logic Updates (parser.go)
We need to update the AST inspectors to look for the specific fields and function calls associated with the new rules.
File: parser.go

1.  Parsing CheckDestroy (Drift Checks)
    The current extractTestSteps function only extracts steps. We need to upgrade it to extract the entire TestCase configuration.
    Modify extractTestSteps signature and logic:
    Return hasCheckDestroy bool in addition to steps.
    Go
    // Change return type to return multiple values or a struct
    func extractTestCaseDetails(body \*ast.BlockStmt) ([]TestStepInfo, bool) { // bool is hasCheckDestroy
    var steps []TestStepInfo
    var hasCheckDestroy bool
    // ... existing setup ...

        ast.Inspect(body, func(n ast.Node) bool {
             // ... find resource.Test call ...
             if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
                 if len(call.Args) >= 2 {
                     // Pass pointers to capture data
                     steps, hasCheckDestroy = parseTestCase(call.Args[1])
                 }
                 return false
             }
             return true
        })
        return steps, hasCheckDestroy

    }

Implement parseTestCase (Replacement for extractStepsFromTestCase):
Iterate over the resource.TestCase struct fields to find CheckDestroy.
Go
func parseTestCase(testCaseExpr ast.Expr) ([]TestStepInfo, bool) {
var steps []TestStepInfo
var hasCheckDestroy bool
stepNumber := 0

    compLit, ok := testCaseExpr.(*ast.CompositeLit)
    if !ok { return steps, false }

    for _, elt := range compLit.Elts {
        kv, ok := elt.(*ast.KeyValueExpr)
        if !ok { continue }

        key, ok := kv.Key.(*ast.Ident)
        if !ok { continue }

        switch key.Name {
        case "Steps":
            // ... existing step extraction logic ...
        case "CheckDestroy":
            hasCheckDestroy = true
        }
    }
    return steps, hasCheckDestroy

}

Update ParseTestFileWithConfig:
Map the new boolean to the TestFunctionInfo struct. 2. Parsing ConfigPlanChecks (Plan Checks)
Update the parsing of individual test steps to recognize Framework plan checks.
Modify parseTestStepWithHash:
Go
func parseTestStepWithHash(stepExpr ast.Expr, stepNum int) TestStepInfo {
// ... existing setup ...

    switch key.Name {
    case "Config":
        // ...
    case "Check":
        // ...
    case "ConfigPlanChecks": // NEW: Framework Plan Checks
        step.HasPlanCheck = true
    // ...
    }
    return step

}

3. Detecting Sweepers
   Add a new function to scan files for resource.AddTestSweepers. This is typically found in TestMain or init() functions.
   Add CheckHasSweepers function:
   Go
   // CheckHasSweepers scans a file for resource.AddTestSweepers calls
   func CheckHasSweepers(file *ast.File) bool {
   found := false
   ast.Inspect(file, func(n ast.Node) bool {
   if call, ok := n.(*ast.CallExpr); ok {
   if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
   if ident, ok := sel.X.(*ast.Ident); ok {
   if ident.Name == "resource" && sel.Sel.Name == "AddTestSweepers" {
   found = true
   return false
   }
   }
   }
   }
   return true
   })
   return found
   }

Phase 3: Integration Checklist
Use this checklist to track the implementation progress.
Step 1: registry.go Modifications
[ ] Add HasCheckDestroy bool to TestFunctionInfo.
[ ] Add HasPlanCheck bool to TestStepInfo.
Step 2: parser.go Refactoring
[ ] Rename extractStepsFromTestCase to parseTestCase (or similar).
[ ] Update parseTestCase to detecting CheckDestroy key in resource.TestCase composite literal.
[ ] Update extractTestSteps to call parseTestCase and return the hasCheckDestroy boolean.
[ ] Update ParseTestFileWithConfig to assign HasCheckDestroy to the TestFunctionInfo object.
[ ] Update parseTestStepWithHash to detect ConfigPlanChecks key and set HasPlanCheck.
[ ] Implement CheckHasSweepers function to AST scan for resource.AddTestSweepers.
Step 3: Analyzer Logic (in analyzer.go)
[ ] Drift Check Analyzer: Create a loop that checks !fn.HasCheckDestroy. Report error if false.
[ ] Plan Check Support: Update StateCheckAnalyzer to allow step.HasPlanCheck as a valid substitute for step.HasCheck.
Logic: if step.HasConfig && !step.HasCheck && !step.HasPlanCheck { report }
[ ] Sweeper Analyzer:
Iterate all files in pass.Files.
Call CheckHasSweepers(file).
If no file returns true, issue a package-level warning.
Example: How StateCheckAnalyzer Logic Should Change
Currently, analyzer.go likely does this:

Go

// Current Logic
if step.HasConfig && !step.HasCheck {
pass.Reportf(pos, "test step missing Check")
}

With the new HasPlanCheck field parsed in Phase 2, you will update it to:

Go

// New Logic
if step.HasConfig && !step.HasCheck && !step.HasPlanCheck {
pass.Reportf(pos, "test step missing state Check or Plan Check")
}
