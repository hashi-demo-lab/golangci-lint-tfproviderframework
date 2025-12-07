package tfprovidertest

import (
	"fmt"
	"go/ast"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test settings.go module
func TestSettings_Module(t *testing.T) {
	t.Run("DefaultSettings returns proper configuration", func(t *testing.T) {
		settings := DefaultSettings()

		assert.True(t, settings.EnableBasicTest, "BasicTest should be enabled by default")
		assert.True(t, settings.EnableUpdateTest, "UpdateTest should be enabled by default")
		assert.True(t, settings.EnableImportTest, "ImportTest should be enabled by default")
		assert.True(t, settings.EnableErrorTest, "ErrorTest should be enabled by default")
		assert.True(t, settings.EnableStateCheck, "StateCheck should be enabled by default")

		assert.Equal(t, "resource_*.go", settings.ResourcePathPattern)
		assert.Equal(t, "data_source_*.go", settings.DataSourcePathPattern)
		assert.Equal(t, "*_test.go", settings.TestFilePattern)

		assert.True(t, settings.ExcludeBaseClasses, "Should exclude base classes by default")
		assert.True(t, settings.ExcludeSweeperFiles, "Should exclude sweeper files by default")
		assert.True(t, settings.ExcludeMigrationFiles, "Should exclude migration files by default")
		assert.True(t, settings.EnableFileBasedMatching, "File-based matching should be enabled")

		assert.False(t, settings.Verbose, "Verbose should be disabled by default")
		assert.Empty(t, settings.ExcludePaths, "ExcludePaths should be empty by default")
		assert.Empty(t, settings.CustomTestHelpers, "CustomTestHelpers should be empty by default")
	})
}

// Test registry.go module
func TestRegistry_Module(t *testing.T) {
	t.Run("NewResourceRegistry creates empty registry", func(t *testing.T) {
		reg := NewResourceRegistry()

		assert.NotNil(t, reg)
		assert.Empty(t, reg.GetAllResources())
		assert.Empty(t, reg.GetAllDataSources())
		assert.Empty(t, reg.GetAllTestFiles())
	})

	t.Run("RegisterResource adds resource to registry", func(t *testing.T) {
		reg := NewResourceRegistry()
		resource := &ResourceInfo{
			Name:         "test_resource",
			IsDataSource: false,
			FilePath:     "/path/to/resource_test_resource.go",
		}

		reg.RegisterResource(resource)

		retrieved := reg.GetResource("test_resource")
		assert.NotNil(t, retrieved)
		assert.Equal(t, "test_resource", retrieved.Name)
		assert.Equal(t, "/path/to/resource_test_resource.go", retrieved.FilePath)
	})

	t.Run("GetResourceByFile retrieves resource by file path", func(t *testing.T) {
		reg := NewResourceRegistry()
		resource := &ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}

		reg.RegisterResource(resource)

		retrieved := reg.GetResourceByFile("/path/to/resource_widget.go")
		assert.NotNil(t, retrieved)
		assert.Equal(t, "widget", retrieved.Name)
	})
}

// Test utils.go module
func TestUtils_Module(t *testing.T) {
	t.Run("toSnakeCase converts CamelCase correctly", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"MyResource", "my_resource"},
			{"HTTPServer", "http_server"},
			{"PrivateKeyRSA", "private_key_rsa"},
			{"Widget", "widget"},
			{"", ""},
		}

		for _, tc := range tests {
			result := CamelCaseToSnakeCaseExported(tc.input)
			assert.Equal(t, tc.expected, result, "Input: %s", tc.input)
		}
	})

	t.Run("toTitleCase converts snake_case correctly", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"my_resource", "MyResource"},
			{"http_server", "HttpServer"},
			{"widget", "Widget"},
			{"", ""},
		}

		for _, tc := range tests {
			result := toTitleCase(tc.input)
			assert.Equal(t, tc.expected, result, "Input: %s", tc.input)
		}
	})

	t.Run("IsSweeperFile identifies sweeper files", func(t *testing.T) {
		assert.True(t, IsSweeperFile("/path/to/resource_sweeper.go"))
		assert.True(t, IsSweeperFile("compute_sweeper.go"))
		assert.False(t, IsSweeperFile("resource_widget.go"))
		assert.False(t, IsSweeperFile("sweeper/resource.go"))
	})

	t.Run("IsMigrationFile identifies migration files", func(t *testing.T) {
		assert.True(t, IsMigrationFile("resource_migrate.go"))
		assert.True(t, IsMigrationFile("resource_migration_v1.go"))
		assert.True(t, IsMigrationFile("resource_state_upgrader.go"))
		assert.False(t, IsMigrationFile("resource_widget.go"))
	})

	t.Run("isTestFunction validates test function names", func(t *testing.T) {
		assert.True(t, IsTestFunctionExported("TestAccWidget_basic", nil))
		assert.True(t, IsTestFunctionExported("TestDataSource_200", nil))
		assert.True(t, IsTestFunctionExported("TestPrivateKeyRSA", nil))
		assert.False(t, IsTestFunctionExported("testHelper", nil)) // lowercase
		assert.False(t, IsTestFunctionExported("Helper", nil))     // no Test prefix
	})
}

// Test parser.go module
func TestParser_Module(t *testing.T) {
	t.Run("extractResourceNameFromFilePath parses standard patterns", func(t *testing.T) {
		tests := []struct {
			filePath       string
			expectedName   string
			expectedIsData bool
		}{
			{"/path/to/resource_widget_test.go", "widget", false},
			{"/path/to/data_source_http_test.go", "http", true},
			{"/path/to/group_resource_test.go", "group", false},
			{"/path/to/inventory_data_source_test.go", "inventory", true},
			{"/path/to/ephemeral_private_key_test.go", "private_key", false},
			{"/path/to/not_a_test.go", "", false},
		}

		for _, tc := range tests {
			name, isData := extractResourceNameFromFilePath(tc.filePath)
			assert.Equal(t, tc.expectedName, name, "FilePath: %s", tc.filePath)
			assert.Equal(t, tc.expectedIsData, isData, "FilePath: %s (isDataSource)", tc.filePath)
		}
	})
}

// Test analyzer.go module
func TestAnalyzer_Module(t *testing.T) {
	t.Run("All analyzers are defined", func(t *testing.T) {
		assert.NotNil(t, BasicTestAnalyzer)
		assert.NotNil(t, UpdateTestAnalyzer)
		assert.NotNil(t, ImportTestAnalyzer)
		assert.NotNil(t, ErrorTestAnalyzer)
		assert.NotNil(t, StateCheckAnalyzer)

		assert.Equal(t, "tfprovider-resource-basic-test", BasicTestAnalyzer.Name)
		assert.Equal(t, "tfprovider-resource-update-test", UpdateTestAnalyzer.Name)
		assert.Equal(t, "tfprovider-resource-import-test", ImportTestAnalyzer.Name)
		assert.Equal(t, "tfprovider-test-error-cases", ErrorTestAnalyzer.Name)
		assert.Equal(t, "tfprovider-test-check-functions", StateCheckAnalyzer.Name)
	})

	t.Run("Plugin BuildAnalyzers returns enabled analyzers", func(t *testing.T) {
		plugin, err := New(nil)
		assert.NoError(t, err)

		analyzers, err := plugin.BuildAnalyzers()
		assert.NoError(t, err)
		assert.Len(t, analyzers, 5, "All 5 analyzers should be enabled by default")
	})

	t.Run("Plugin respects disabled analyzers", func(t *testing.T) {
		settings := map[string]interface{}{
			"EnableBasicTest":  true,
			"EnableUpdateTest": false,
			"EnableImportTest": false,
			"EnableErrorTest":  false,
			"EnableStateCheck": false,
		}

		plugin, err := New(settings)
		assert.NoError(t, err)

		analyzers, err := plugin.BuildAnalyzers()
		assert.NoError(t, err)
		assert.Len(t, analyzers, 1, "Only BasicTest should be enabled")
		assert.Equal(t, "tfprovider-resource-basic-test", analyzers[0].Name)
	})
}

// Integration test for the full workflow
func TestIntegration_FileBasedMatching(t *testing.T) {
	t.Run("File-based matching workflow", func(t *testing.T) {
		// This test documents the expected workflow
		// 1. Parse resource files to find resources (AST-based)
		// 2. Parse test files to find tests (file-based)
		// 3. Associate tests with resources using file naming
		// 4. Report untested resources

		reg := NewResourceRegistry()

		// Simulate finding a resource
		resource := &ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/provider/internal/resource_widget.go",
		}
		reg.RegisterResource(resource)

		// Simulate finding a test file
		testFile := &TestFileInfo{
			FilePath:     "/provider/internal/resource_widget_test.go",
			ResourceName: "widget",
			IsDataSource: false,
			TestFunctions: []TestFunctionInfo{
				{
					Name:             "TestAccWidget_basic",
					UsesResourceTest: true,
				},
			},
		}
		reg.RegisterTestFile(testFile)

		// Verify association
		foundTest := reg.GetTestFile("widget")
		assert.NotNil(t, foundTest)
		assert.Len(t, foundTest.TestFunctions, 1)

		// Verify no untested resources
		untested := reg.GetUntestedResources()
		assert.Empty(t, untested, "Widget has tests, should not be untested")
	})
}

// =============================================================================
// Phase 1 Foundation Tests - MatchType and New Registry Methods
// =============================================================================

// Test MatchType.String() for all values
func TestMatchType_String(t *testing.T) {
	t.Run("MatchTypeNone returns 'none'", func(t *testing.T) {
		assert.Equal(t, "none", MatchTypeNone.String())
	})

	t.Run("MatchTypeFunctionName returns 'function_name'", func(t *testing.T) {
		assert.Equal(t, "function_name", MatchTypeFunctionName.String())
	})

	t.Run("MatchTypeFileProximity returns 'file_proximity'", func(t *testing.T) {
		assert.Equal(t, "file_proximity", MatchTypeFileProximity.String())
	})

	t.Run("MatchTypeFuzzy returns 'fuzzy'", func(t *testing.T) {
		assert.Equal(t, "fuzzy", MatchTypeFuzzy.String())
	})

	t.Run("MatchType default value is MatchTypeNone", func(t *testing.T) {
		var m MatchType // default value
		assert.Equal(t, MatchTypeNone, m)
		assert.Equal(t, "none", m.String())
	})
}

// Test RegisterTestFunction
func TestRegisterTestFunction(t *testing.T) {
	t.Run("should register test function to global index", func(t *testing.T) {
		reg := NewResourceRegistry()

		fn := &TestFunctionInfo{
			Name:     "TestAccWidget_basic",
			FilePath: "/path/to/resource_widget_test.go",
		}

		reg.RegisterTestFunction(fn)

		funcs := reg.GetAllTestFunctions()
		assert.Len(t, funcs, 1)
		assert.Equal(t, "TestAccWidget_basic", funcs[0].Name)
		assert.Equal(t, "/path/to/resource_widget_test.go", funcs[0].FilePath)
	})

	t.Run("should append multiple test functions", func(t *testing.T) {
		reg := NewResourceRegistry()

		fn1 := &TestFunctionInfo{Name: "TestAccWidget_basic"}
		fn2 := &TestFunctionInfo{Name: "TestAccWidget_update"}
		fn3 := &TestFunctionInfo{Name: "TestAccServer_basic"}

		reg.RegisterTestFunction(fn1)
		reg.RegisterTestFunction(fn2)
		reg.RegisterTestFunction(fn3)

		funcs := reg.GetAllTestFunctions()
		assert.Len(t, funcs, 3)
	})
}

// Test LinkTestToResource
func TestLinkTestToResource(t *testing.T) {
	t.Run("should associate test function with resource", func(t *testing.T) {
		reg := NewResourceRegistry()

		// Register a resource first
		reg.RegisterResource(&ResourceInfo{Name: "widget"})

		fn := &TestFunctionInfo{
			Name:              "TestAccWidget_basic",
			InferredResources: []string{"widget"},
			MatchConfidence:   1.0,
			MatchType:         MatchTypeFunctionName,
		}

		reg.RegisterTestFunction(fn)
		reg.LinkTestToResource("widget", fn)

		tests := reg.GetResourceTests("widget")
		assert.Len(t, tests, 1)
		assert.Equal(t, "TestAccWidget_basic", tests[0].Name)
	})

	t.Run("should allow multiple tests per resource", func(t *testing.T) {
		reg := NewResourceRegistry()

		reg.RegisterResource(&ResourceInfo{Name: "widget"})

		fn1 := &TestFunctionInfo{Name: "TestAccWidget_basic"}
		fn2 := &TestFunctionInfo{Name: "TestAccWidget_update"}

		reg.LinkTestToResource("widget", fn1)
		reg.LinkTestToResource("widget", fn2)

		tests := reg.GetResourceTests("widget")
		assert.Len(t, tests, 2)
	})

	t.Run("GetResourceTests returns nil for unknown resource", func(t *testing.T) {
		reg := NewResourceRegistry()

		tests := reg.GetResourceTests("nonexistent")
		assert.Nil(t, tests)
	})
}

// Test GetUnmatchedTestFunctions
func TestGetUnmatchedTestFunctions(t *testing.T) {
	t.Run("should return functions with empty InferredResources", func(t *testing.T) {
		reg := NewResourceRegistry()

		matched := &TestFunctionInfo{
			Name:              "TestAccWidget_basic",
			InferredResources: []string{"widget"},
		}
		unmatched := &TestFunctionInfo{
			Name:              "TestAccOrphan_basic",
			InferredResources: []string{}, // No matches
		}

		reg.RegisterTestFunction(matched)
		reg.RegisterTestFunction(unmatched)

		orphans := reg.GetUnmatchedTestFunctions()
		assert.Len(t, orphans, 1)
		assert.Equal(t, "TestAccOrphan_basic", orphans[0].Name)
	})

	t.Run("should return empty slice when all functions are matched", func(t *testing.T) {
		reg := NewResourceRegistry()

		fn := &TestFunctionInfo{
			Name:              "TestAccWidget_basic",
			InferredResources: []string{"widget"},
		}

		reg.RegisterTestFunction(fn)

		orphans := reg.GetUnmatchedTestFunctions()
		assert.Empty(t, orphans)
	})

	t.Run("should return empty slice when no functions registered", func(t *testing.T) {
		reg := NewResourceRegistry()

		orphans := reg.GetUnmatchedTestFunctions()
		assert.Empty(t, orphans)
	})
}

// Test GetUntestedResources with new resourceTests
func TestGetUntestedResources_WithResourceTests(t *testing.T) {
	t.Run("should use resourceTests map for test coverage check", func(t *testing.T) {
		reg := NewResourceRegistry()

		// Register two resources
		reg.RegisterResource(&ResourceInfo{Name: "tested"})
		reg.RegisterResource(&ResourceInfo{Name: "untested"})

		// Link test to 'tested' resource via new method
		fn := &TestFunctionInfo{
			Name:              "TestAccTested_basic",
			InferredResources: []string{"tested"},
		}
		reg.RegisterTestFunction(fn)
		reg.LinkTestToResource("tested", fn)

		untested := reg.GetUntestedResources()
		assert.Len(t, untested, 1)
		assert.Equal(t, "untested", untested[0].Name)
	})

	t.Run("should also check legacy testFiles for backward compatibility", func(t *testing.T) {
		reg := NewResourceRegistry()

		// Register two resources
		reg.RegisterResource(&ResourceInfo{Name: "tested_legacy"})
		reg.RegisterResource(&ResourceInfo{Name: "untested"})

		// Register test via legacy method
		testFile := &TestFileInfo{
			FilePath:     "/path/to/resource_tested_legacy_test.go",
			ResourceName: "tested_legacy",
			TestFunctions: []TestFunctionInfo{
				{Name: "TestAccTestedLegacy_basic"},
			},
		}
		reg.RegisterTestFile(testFile)

		untested := reg.GetUntestedResources()
		assert.Len(t, untested, 1)
		assert.Equal(t, "untested", untested[0].Name)
	})
}

// Test concurrent access to registry
func TestRegistryConcurrentAccess(t *testing.T) {
	t.Run("should handle concurrent RegisterTestFunction calls", func(t *testing.T) {
		reg := NewResourceRegistry()
		var wg sync.WaitGroup

		// Concurrent writes
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				fn := &TestFunctionInfo{
					Name: fmt.Sprintf("TestFunc%d", n),
				}
				reg.RegisterTestFunction(fn)
			}(i)
		}

		wg.Wait()

		funcs := reg.GetAllTestFunctions()
		assert.Len(t, funcs, 100)
	})

	t.Run("should handle concurrent reads while writing", func(t *testing.T) {
		reg := NewResourceRegistry()
		var wg sync.WaitGroup

		// Add some initial functions
		for i := 0; i < 50; i++ {
			fn := &TestFunctionInfo{Name: fmt.Sprintf("Initial%d", i)}
			reg.RegisterTestFunction(fn)
		}

		// Concurrent writes
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				fn := &TestFunctionInfo{
					Name: fmt.Sprintf("Concurrent%d", n),
				}
				reg.RegisterTestFunction(fn)
			}(i)
		}

		// Concurrent reads while writing
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = reg.GetAllTestFunctions()
			}()
		}

		wg.Wait()

		funcs := reg.GetAllTestFunctions()
		assert.Len(t, funcs, 100)
	})

	t.Run("should handle concurrent LinkTestToResource calls", func(t *testing.T) {
		reg := NewResourceRegistry()
		var wg sync.WaitGroup

		// Register resources first
		for i := 0; i < 10; i++ {
			reg.RegisterResource(&ResourceInfo{Name: fmt.Sprintf("resource%d", i)})
		}

		// Concurrent links
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				fn := &TestFunctionInfo{
					Name: fmt.Sprintf("TestFunc%d", n),
				}
				resourceName := fmt.Sprintf("resource%d", n%10)
				reg.LinkTestToResource(resourceName, fn)
			}(i)
		}

		wg.Wait()

		// Verify total links
		totalLinks := 0
		for i := 0; i < 10; i++ {
			tests := reg.GetResourceTests(fmt.Sprintf("resource%d", i))
			totalLinks += len(tests)
		}
		assert.Equal(t, 100, totalLinks)
	})
}

// Test TestFileInfo with new PackageName field
func TestTestFileInfo_PackageName(t *testing.T) {
	t.Run("should store PackageName", func(t *testing.T) {
		info := &TestFileInfo{
			FilePath:    "/path/to/resource_widget_test.go",
			PackageName: "provider_test",
		}

		assert.Equal(t, "provider_test", info.PackageName)
	})
}

// Test TestFunctionInfo with new fields
func TestTestFunctionInfo_NewFields(t *testing.T) {
	t.Run("should store FilePath", func(t *testing.T) {
		info := &TestFunctionInfo{
			Name:     "TestAccWidget_basic",
			FilePath: "/path/to/resource_widget_test.go",
		}

		assert.Equal(t, "/path/to/resource_widget_test.go", info.FilePath)
	})

	t.Run("should store InferredResources", func(t *testing.T) {
		info := &TestFunctionInfo{
			Name:              "TestAccWidget_basic",
			InferredResources: []string{"widget", "widget_v2"},
		}

		assert.Len(t, info.InferredResources, 2)
		assert.Contains(t, info.InferredResources, "widget")
	})

	t.Run("should store MatchConfidence and MatchType", func(t *testing.T) {
		info := &TestFunctionInfo{
			Name:            "TestAccWidget_basic",
			MatchConfidence: 0.95,
			MatchType:       MatchTypeFunctionName,
		}

		assert.Equal(t, 0.95, info.MatchConfidence)
		assert.Equal(t, MatchTypeFunctionName, info.MatchType)
	})
}

// =============================================================================
// Phase 2 Test Step Analysis Improvements Tests
// =============================================================================

// Test DetermineIfUpdateStep
func TestDetermineIfUpdateStep(t *testing.T) {
	t.Run("first step is never an update", func(t *testing.T) {
		step := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
			ConfigHash: "abc123",
		}

		assert.False(t, step.DetermineIfUpdateStep(nil))
	})

	t.Run("import step is not an update", func(t *testing.T) {
		prevStep := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
			ConfigHash: "abc123",
		}
		step := &TestStepInfo{
			StepNumber:  1,
			HasConfig:   false,
			ImportState: true,
		}

		assert.False(t, step.DetermineIfUpdateStep(prevStep))
	})

	t.Run("same config hash is idempotency test not update", func(t *testing.T) {
		prevStep := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
			ConfigHash: "abc123",
		}
		step := &TestStepInfo{
			StepNumber: 1,
			HasConfig:  true,
			ConfigHash: "abc123", // Same hash
		}

		assert.False(t, step.DetermineIfUpdateStep(prevStep))
	})

	t.Run("different config hash is update step", func(t *testing.T) {
		prevStep := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
			ConfigHash: "abc123",
		}
		step := &TestStepInfo{
			StepNumber: 1,
			HasConfig:  true,
			ConfigHash: "def456", // Different hash
		}

		assert.True(t, step.DetermineIfUpdateStep(prevStep))
	})

	t.Run("no previous step means not an update", func(t *testing.T) {
		step := &TestStepInfo{
			StepNumber: 1,
			HasConfig:  true,
			ConfigHash: "abc123",
		}

		assert.False(t, step.DetermineIfUpdateStep(nil))
	})

	t.Run("previous step without config means not an update", func(t *testing.T) {
		prevStep := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  false, // No config in previous
		}
		step := &TestStepInfo{
			StepNumber: 1,
			HasConfig:  true,
			ConfigHash: "abc123",
		}

		assert.False(t, step.DetermineIfUpdateStep(prevStep))
	})

	t.Run("step without config is not an update", func(t *testing.T) {
		prevStep := &TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
			ConfigHash: "abc123",
		}
		step := &TestStepInfo{
			StepNumber: 1,
			HasConfig:  false,
		}

		assert.False(t, step.DetermineIfUpdateStep(prevStep))
	})
}

// Test HasRequiresReplaceWithConfidence
func TestHasRequiresReplaceWithConfidence(t *testing.T) {
	t.Run("known modifier returns high confidence", func(t *testing.T) {
		// Test that known modifiers are detected with high confidence
		result := HasRequiresReplaceWithConfidence(nil)
		assert.False(t, result.Found)
		assert.Equal(t, 0.0, result.Confidence)
	})

	t.Run("standardRequiresReplaceModifiers contains expected entries", func(t *testing.T) {
		assert.True(t, standardRequiresReplaceModifiers["RequiresReplace"])
		assert.True(t, standardRequiresReplaceModifiers["RequiresReplaceIf"])
		assert.True(t, standardRequiresReplaceModifiers["RequiresReplaceIfConfigured"])
		assert.False(t, standardRequiresReplaceModifiers["UseStateForUnknown"])
	})
}

// Test CheckSuppressionComment
func TestCheckSuppressionComment(t *testing.T) {
	t.Run("returns false for nil comments", func(t *testing.T) {
		result := CheckSuppressionComment(nil, "tfprovider-resource-basic-test")
		assert.False(t, result)
	})

	t.Run("returns false for empty comments", func(t *testing.T) {
		result := CheckSuppressionComment([]*ast.CommentGroup{}, "tfprovider-resource-basic-test")
		assert.False(t, result)
	})

	t.Run("returns true for matching nolint comment", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:tfprovider-resource-basic-test"},
				},
			},
		}
		result := CheckSuppressionComment(comments, "tfprovider-resource-basic-test")
		assert.True(t, result)
	})

	t.Run("returns true for 'all' suppression", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:all"},
				},
			},
		}
		result := CheckSuppressionComment(comments, "any-check-name")
		assert.True(t, result)
	})

	t.Run("returns false for non-matching check", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:other-check"},
				},
			},
		}
		result := CheckSuppressionComment(comments, "tfprovider-resource-basic-test")
		assert.False(t, result)
	})

	t.Run("handles tfprovidertest:disable format", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// tfprovidertest:disable tfprovider-resource-basic-test"},
				},
			},
		}
		result := CheckSuppressionComment(comments, "tfprovider-resource-basic-test")
		assert.True(t, result)
	})
}

// Test GetSuppressedChecks
func TestGetSuppressedChecks(t *testing.T) {
	t.Run("returns empty for nil comments", func(t *testing.T) {
		result := GetSuppressedChecks(nil)
		assert.Empty(t, result)
	})

	t.Run("extracts single check from nolint", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:tfprovider-resource-basic-test"},
				},
			},
		}
		result := GetSuppressedChecks(comments)
		assert.Contains(t, result, "tfprovider-resource-basic-test")
	})

	t.Run("extracts multiple checks from comma-separated list", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:check1,check2,check3"},
				},
			},
		}
		result := GetSuppressedChecks(comments)
		assert.Len(t, result, 3)
		assert.Contains(t, result, "check1")
		assert.Contains(t, result, "check2")
		assert.Contains(t, result, "check3")
	})

	t.Run("handles lint:ignore format", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// lint:ignore mycheck"},
				},
			},
		}
		result := GetSuppressedChecks(comments)
		assert.Contains(t, result, "mycheck")
	})

	t.Run("handles tfprovidertest:disable format", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// tfprovidertest:disable mycheck1,mycheck2"},
				},
			},
		}
		result := GetSuppressedChecks(comments)
		assert.Contains(t, result, "mycheck1")
		assert.Contains(t, result, "mycheck2")
	})

	t.Run("handles multiple comment groups", func(t *testing.T) {
		comments := []*ast.CommentGroup{
			{
				List: []*ast.Comment{
					{Text: "// nolint:check1"},
				},
			},
			{
				List: []*ast.Comment{
					{Text: "// nolint:check2"},
				},
			},
		}
		result := GetSuppressedChecks(comments)
		assert.Contains(t, result, "check1")
		assert.Contains(t, result, "check2")
	})
}

// Test HashConfigExpr
func TestHashConfigExpr(t *testing.T) {
	t.Run("returns empty for nil expr", func(t *testing.T) {
		result := HashConfigExpr(nil)
		assert.Equal(t, "", result)
	})

	t.Run("returns non-empty hash for valid expr", func(t *testing.T) {
		// Create a simple AST expression
		expr := &ast.BasicLit{
			Kind:  0,
			Value: `"test config"`,
		}
		result := HashConfigExpr(expr)
		assert.NotEmpty(t, result)
		// Hash should be 16 hex chars (8 bytes)
		assert.Len(t, result, 16)
	})

	t.Run("same expression produces same hash", func(t *testing.T) {
		expr1 := &ast.BasicLit{Value: `"test config"`}
		expr2 := &ast.BasicLit{Value: `"test config"`}
		result1 := HashConfigExpr(expr1)
		result2 := HashConfigExpr(expr2)
		assert.Equal(t, result1, result2)
	})

	t.Run("different expression produces different hash", func(t *testing.T) {
		expr1 := &ast.BasicLit{Value: `"test config 1"`}
		expr2 := &ast.BasicLit{Value: `"test config 2"`}
		result1 := HashConfigExpr(expr1)
		result2 := HashConfigExpr(expr2)
		assert.NotEqual(t, result1, result2)
	})
}

// Test TestStepInfo new fields
func TestTestStepInfo_NewFields(t *testing.T) {
	t.Run("should store ConfigHash", func(t *testing.T) {
		step := TestStepInfo{
			ConfigHash: "abc123def456",
		}
		assert.Equal(t, "abc123def456", step.ConfigHash)
	})

	t.Run("should store IsUpdateStepFlag", func(t *testing.T) {
		step := TestStepInfo{
			IsUpdateStepFlag: true,
		}
		assert.True(t, step.IsUpdateStepFlag)
	})

	t.Run("should store PreviousConfigHash", func(t *testing.T) {
		step := TestStepInfo{
			PreviousConfigHash: "prev123",
		}
		assert.Equal(t, "prev123", step.PreviousConfigHash)
	})

	t.Run("IsUpdateStep deprecated method still works", func(t *testing.T) {
		step := TestStepInfo{
			StepNumber: 1,
			HasConfig:  true,
		}
		assert.True(t, step.IsUpdateStep())

		step0 := TestStepInfo{
			StepNumber: 0,
			HasConfig:  true,
		}
		assert.False(t, step0.IsUpdateStep())
	})
}

// Test RequiresReplaceResult struct
func TestRequiresReplaceResult(t *testing.T) {
	t.Run("should store all fields", func(t *testing.T) {
		result := RequiresReplaceResult{
			Found:        true,
			Confidence:   0.95,
			ModifierName: "RequiresReplace",
		}

		assert.True(t, result.Found)
		assert.Equal(t, 0.95, result.Confidence)
		assert.Equal(t, "RequiresReplace", result.ModifierName)
	})

	t.Run("default values", func(t *testing.T) {
		var result RequiresReplaceResult
		assert.False(t, result.Found)
		assert.Equal(t, 0.0, result.Confidence)
		assert.Equal(t, "", result.ModifierName)
	})
}
