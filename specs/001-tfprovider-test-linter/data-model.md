# Phase 1: Data Model

**Feature**: Terraform Provider Test Coverage Linter
**Date**: 2025-12-07
**Status**: Complete

## Overview

This document defines the core entities, their attributes, relationships, and validation rules for the Terraform Provider Test Coverage Linter.

## Entity Definitions

### 1. Analyzer

Represents a single linting rule implemented as a go/analysis.Analyzer instance.

**Attributes**:
- `Name` (string, required): Unique identifier for the analyzer (e.g., "tfprovider-resource-basic-test")
- `Doc` (string, required): Human-readable description of what the analyzer checks
- `Run` (function, required): Analysis function that receives *analysis.Pass and returns (interface{}, error)
- `Requires` ([]*analysis.Analyzer, optional): Dependent analyzers that must run first
- `Enabled` (bool, default: true): Whether this analyzer is enabled via configuration

**Validation Rules**:
- Name must match pattern: `^tfprovider-[a-z-]+$`
- Doc must be non-empty and end with period
- Run function must not be nil
- Enabled state driven by Settings configuration

**Relationships**:
- Has many `Diagnostic` (one-to-many): Produces diagnostics when issues found
- Configured by `Settings` (one-to-one): User configuration controls behavior

**State Transitions**:
- Created → Registered (via init() function)
- Registered → Running (when golangci-lint executes)
- Running → Complete (after analysis.Pass processed)

---

### 2. Settings

Represents user-configurable options loaded from .golangci.yml.

**Attributes**:
- `EnableBasicTest` (bool, default: true): Enable basic acceptance test coverage check
- `EnableUpdateTest` (bool, default: true): Enable update test coverage check
- `EnableImportTest` (bool, default: true): Enable import test coverage check
- `EnableErrorTest` (bool, default: true): Enable error case test coverage check
- `EnableStateCheck` (bool, default: true): Enable state check function validation
- `ResourcePathPattern` (string, default: "resource_*.go"): Glob pattern for resource files
- `DataSourcePathPattern` (string, default: "data_source_*.go"): Glob pattern for data source files
- `TestFilePattern` (string, default: "*_test.go"): Glob pattern for test files
- `ExcludePaths` ([]string, default: []): Paths to exclude from analysis (e.g., vendor/)

**Validation Rules**:
- At least one Enable* flag must be true
- Path patterns must be valid glob expressions
- ExcludePaths must use absolute or repo-relative paths

**Relationships**:
- Configures multiple `Analyzer` instances (one-to-many)
- Created by `Plugin.New()` via register.DecodeSettings

---

### 3. ResourceInfo

Represents a detected Terraform resource or data source in provider code.

**Attributes**:
- `Name` (string, required): Resource type name (e.g., "widget", "account")
- `IsDataSource` (bool, required): true for data sources, false for resources
- `FilePath` (string, required): Absolute path to the schema definition file
- `SchemaPos` (token.Pos, required): AST position of Schema() method
- `Attributes` ([]AttributeInfo, required): List of schema attributes
- `HasImportState` (bool, default: false): Whether resource implements ImportState method
- `ImportStatePos` (token.Pos, optional): AST position of ImportState() method

**Validation Rules**:
- Name must be non-empty and match [a-z_]+ pattern
- FilePath must exist and be a .go file
- SchemaPos must be valid position within file
- If HasImportState is true, ImportStatePos must be set

**Relationships**:
- Has many `AttributeInfo` (one-to-many): Schema attributes
- Matched to `TestFileInfo` (one-to-one): Corresponding test file
- Stored in `ResourceRegistry` (many-to-one)

---

### 4. AttributeInfo

Represents a single attribute in a resource or data source schema.

**Attributes**:
- `Name` (string, required): Attribute name (e.g., "description", "tags")
- `Type` (string, required): Attribute type (e.g., "String", "Int64", "List")
- `Required` (bool, default: false): Whether attribute is required
- `Optional` (bool, default: false): Whether attribute is optional
- `Computed` (bool, default: false): Whether attribute is computed
- `IsUpdatable` (bool, default: true): Whether attribute can be updated in-place
- `HasValidators` (bool, default: false): Whether attribute has validators
- `ValidatorTypes` ([]string, optional): List of validator type names

**Validation Rules**:
- Name must be non-empty
- Exactly one of Required, Optional, or Computed must be true
- If HasValidators is true, ValidatorTypes must be non-empty
- IsUpdatable is false if PlanModifiers include RequiresReplace

**Relationships**:
- Belongs to `ResourceInfo` (many-to-one)
- Referenced in `TestStep` checks (many-to-many)

**Derived Properties**:
- `NeedsUpdateTest`: True if Optional=true AND IsUpdatable=true
- `NeedsValidationTest`: True if HasValidators=true OR Required=true

---

### 5. TestFileInfo

Represents a Go test file containing acceptance tests.

**Attributes**:
- `FilePath` (string, required): Absolute path to test file
- `ResourceName` (string, required): Resource name this test file covers (e.g., "widget")
- `IsDataSource` (bool, required): Whether this tests a data source
- `TestFunctions` ([]TestFunctionInfo, required): List of test functions

**Validation Rules**:
- FilePath must exist and be a _test.go file
- ResourceName must match corresponding ResourceInfo.Name
- TestFunctions must contain at least one entry for basic coverage

**Relationships**:
- Matches to `ResourceInfo` (one-to-one): The resource being tested
- Has many `TestFunctionInfo` (one-to-many): Test functions in this file

---

### 6. TestFunctionInfo

Represents a single test function (e.g., TestAccResourceWidget_basic).

**Attributes**:
- `Name` (string, required): Function name (must start with TestAcc)
- `FunctionPos` (token.Pos, required): AST position of function declaration
- `UsesResourceTest` (bool, required): Whether function calls resource.Test()
- `TestSteps` ([]TestStepInfo, required): List of test steps in this function
- `HasErrorCase` (bool, default: false): Whether any step uses ExpectError
- `HasImportStep` (bool, default: false): Whether any step uses ImportState

**Validation Rules**:
- Name must match pattern: `^TestAcc(Resource|DataSource)[A-Za-z0-9_]+$`
- If UsesResourceTest is false, test is not counted as acceptance test
- TestSteps must be non-empty if UsesResourceTest is true

**Relationships**:
- Belongs to `TestFileInfo` (many-to-one)
- Has many `TestStepInfo` (one-to-many)

---

### 7. TestStepInfo

Represents a single resource.TestStep{} in an acceptance test.

**Attributes**:
- `StepNumber` (int, required): Sequential step number (0-based)
- `Config` (string, required): Terraform configuration for this step
- `HasCheck` (bool, required): Whether step includes Check field
- `CheckFunctions` ([]string, optional): List of check function names (e.g., "resource.TestCheckResourceAttr")
- `ImportState` (bool, default: false): Whether this is an import test step
- `ImportStateVerify` (bool, default: false): Whether import state verification is enabled
- `ExpectError` (bool, default: false): Whether this step expects an error

**Validation Rules**:
- StepNumber must be >= 0
- Config must be non-empty
- If HasCheck is true, CheckFunctions must be non-empty
- If ImportState is true, Config may be empty (import-only step)
- If ExpectError is true, check functions are optional

**Relationships**:
- Belongs to `TestFunctionInfo` (many-to-one)

**Derived Properties**:
- `IsUpdateStep`: True if StepNumber > 0 AND Config differs from previous step
- `IsValidImportStep`: True if ImportState=true AND ImportStateVerify=true

---

### 8. ResourceRegistry

Singleton registry mapping resource names to ResourceInfo and TestFileInfo.

**Attributes**:
- `Resources` (map[string]*ResourceInfo, required): Map of resource name → ResourceInfo
- `DataSources` (map[string]*ResourceInfo, required): Map of data source name → ResourceInfo
- `TestFiles` (map[string]*TestFileInfo, required): Map of resource name → TestFileInfo

**Operations**:
- `RegisterResource(info *ResourceInfo)`: Add resource to registry
- `RegisterTestFile(info *TestFileInfo)`: Add test file to registry
- `GetResourceWithTest(name string) (*ResourceInfo, *TestFileInfo, bool)`: Retrieve matched pair
- `GetUntestedResources() []*ResourceInfo`: Find resources without test files

**Validation Rules**:
- Resource names must be unique within Resources map
- Data source names must be unique within DataSources map
- TestFiles keys must reference existing Resources or DataSources

**Relationships**:
- Contains many `ResourceInfo` (one-to-many)
- Contains many `TestFileInfo` (one-to-many)
- Shared across all `Analyzer` instances (singleton)

---

### 9. Diagnostic

Represents a linting issue reported by an analyzer.

**Attributes**:
- `Pos` (token.Pos, required): Source code position where issue occurs
- `Message` (string, required): Human-readable error message
- `Category` (string, required): Analyzer name that produced diagnostic
- `SuggestedFix` (string, optional): Guidance on how to resolve the issue

**Validation Rules**:
- Pos must be valid position in analyzed file
- Message must be non-empty and actionable
- Category must match one of the 5 analyzer names
- SuggestedFix should provide concrete remediation steps

**Relationships**:
- Produced by `Analyzer` (many-to-one)
- References `ResourceInfo` or `TestFileInfo` context (many-to-one)

**Message Templates**:
- Basic test missing: "resource '{name}' has no acceptance test file"
- Update test missing: "resource '{name}' has updatable attributes but no update test coverage"
- Import test missing: "resource '{name}' implements ImportState but has no import test coverage"
- Error test missing: "resource '{name}' has validation rules but no error case tests"
- State check missing: "test step for resource '{name}' has no state validation checks"

---

### 10. Plugin

Represents the golangci-lint plugin instance.

**Attributes**:
- `settings` (Settings, required): User configuration loaded from .golangci.yml

**Operations**:
- `New(settings any) (register.LinterPlugin, error)`: Factory function for plugin creation
- `BuildAnalyzers() ([]*analysis.Analyzer, error)`: Returns list of enabled analyzers
- `GetLoadMode() string`: Returns LoadModeSyntax for performance

**Validation Rules**:
- settings must be successfully decoded from YAML
- BuildAnalyzers must return at least one analyzer

**Relationships**:
- Owns `Settings` (one-to-one)
- Creates multiple `Analyzer` instances (one-to-many)

---

## Entity Relationship Diagram

```
Plugin (1) ←→ (1) Settings
   ↓
   creates
   ↓
Analyzer (5) ──produces→ Diagnostic (*)
   ↓
   uses
   ↓
ResourceRegistry (1)
   ├─ Resources: (*)
   │     └→ ResourceInfo ──has→ AttributeInfo (*)
   │             ↓
   │          matched to
   │             ↓
   └─ TestFiles: (*)
         └→ TestFileInfo ──has→ TestFunctionInfo (*)
                                    ├→ TestStepInfo (*)
```

**Key**:
- (1) = one instance
- (*) = many instances
- ──→ = one-to-many relationship
- ←→ = one-to-one relationship

---

## Data Flow

1. **Plugin Initialization**:
   - golangci-lint loads plugin via init() registration
   - Plugin.New() decodes Settings from .golangci.yml
   - BuildAnalyzers() creates 5 Analyzer instances

2. **Analysis Phase**:
   - Each Analyzer.Run() receives *analysis.Pass
   - First pass: Build ResourceRegistry by traversing AST
   - Detect ResourceInfo from schema.Resource implementations
   - Detect TestFileInfo from test files with resource.Test() calls

3. **Validation Phase**:
   - Match ResourceInfo to TestFileInfo via ResourceRegistry
   - For each analyzer rule:
     - Check if corresponding test pattern exists
     - Generate Diagnostic if missing/incomplete
   - Report diagnostics via pass.Report()

4. **Output**:
   - golangci-lint collects diagnostics from all analyzers
   - Format as JSON, text, or IDE-compatible format
   - Exit with error code if diagnostics found (fail CI)

---

## Validation Examples

### Example 1: Resource with No Test File

**ResourceInfo**:
```go
{
  Name: "widget",
  IsDataSource: false,
  FilePath: "/internal/provider/resource_widget.go",
  SchemaPos: token.Pos(1234),
  Attributes: [...],
  HasImportState: false
}
```

**TestFileInfo**: `nil` (not found)

**Diagnostic**:
```go
{
  Pos: token.Pos(1234), // SchemaPos from ResourceInfo
  Message: "resource 'widget' has no acceptance test file",
  Category: "tfprovider-resource-basic-test",
  SuggestedFix: "Create resource_widget_test.go with TestAccResourceWidget_basic function"
}
```

---

### Example 2: Resource with Update Needed but No Multi-Step Test

**ResourceInfo**:
```go
{
  Name: "config",
  IsDataSource: false,
  FilePath: "/internal/provider/resource_config.go",
  Attributes: [
    {Name: "name", IsUpdatable: false}, // immutable
    {Name: "description", IsUpdatable: true}, // updatable
    {Name: "tags", IsUpdatable: true} // updatable
  ]
}
```

**TestFileInfo**:
```go
{
  FilePath: "/internal/provider/resource_config_test.go",
  ResourceName: "config",
  TestFunctions: [
    {
      Name: "TestAccResourceConfig_basic",
      TestSteps: [
        {StepNumber: 0, Config: "...", HasCheck: true} // only 1 step
      ]
    }
  ]
}
```

**Diagnostic**:
```go
{
  Pos: token.Pos(1234), // SchemaPos from ResourceInfo
  Message: "resource 'config' has updatable attributes but no update test coverage",
  Category: "tfprovider-resource-update-test",
  SuggestedFix: "Add TestAccResourceConfig_update with multi-step test modifying 'description' or 'tags'"
}
```

---

## Implementation Notes

### AST Traversal Strategy

All entities are populated via a single AST traversal per file:

```go
ast.Inspect(file, func(node ast.Node) bool {
  switch n := node.(type) {
  case *ast.FuncDecl:
    // Detect Schema() methods → ResourceInfo
    // Detect ImportState() methods → HasImportState
    // Detect TestAcc* functions → TestFunctionInfo
  case *ast.CompositeLit:
    // Detect schema.Schema{} → Attributes
    // Detect resource.TestStep{} → TestStepInfo
  }
  return true
})
```

### Performance Considerations

- ResourceRegistry is built once and shared across all analyzers
- AST caching prevents redundant parsing
- Parallel analyzer execution enabled by go/analysis framework
- Memory-efficient: ResourceInfo references, not full AST copies

### Future Extensions

- Add `DeprecatedInfo` entity to detect deprecated attributes without tests
- Add `MigrationInfo` to track schema version changes
- Support for custom test pattern matchers via configuration
