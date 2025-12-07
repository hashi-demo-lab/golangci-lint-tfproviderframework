package tfprovidertest

import (
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
		assert.False(t, IsTestFunctionExported("Helper", nil)) // no Test prefix
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
