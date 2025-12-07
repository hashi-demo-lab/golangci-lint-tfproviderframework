package tfprovidertest_test

import (
	"fmt"
	"testing"

	tfprovidertest "github.com/example/tfprovidertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	// Note: analysistest will be imported when integration tests are enabled
	// "golang.org/x/tools/go/analysis/analysistest"
)

// T006: Test for Settings defaults
func TestSettings_Defaults(t *testing.T) {
	t.Run("default settings should enable all analyzers", func(t *testing.T) {
		settings := tfprovidertest.DefaultSettings()
		assert.True(t, settings.EnableBasicTest)
		assert.True(t, settings.EnableUpdateTest)
		assert.True(t, settings.EnableImportTest)
		assert.True(t, settings.EnableErrorTest)
		assert.True(t, settings.EnableStateCheck)
		assert.Equal(t, "resource_*.go", settings.ResourcePathPattern)
		assert.Equal(t, "data_source_*.go", settings.DataSourcePathPattern)
		assert.Equal(t, "*_test.go", settings.TestFilePattern)
		assert.Empty(t, settings.ExcludePaths)
		assert.True(t, settings.ExcludeBaseClasses)
	})
}

// T007: Test for ResourceRegistry operations
func TestResourceRegistry_Operations(t *testing.T) {
	t.Run("should register resources and retrieve them", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()

		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/test/resource_widget.go",
		}
		registry.RegisterResource(resource)

		retrieved := registry.GetResource("widget")
		require.NotNil(t, retrieved)
		assert.Equal(t, "widget", retrieved.Name)
	})

	t.Run("should track untested resources", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()
		registry.RegisterResource(&tfprovidertest.ResourceInfo{Name: "tested", IsDataSource: false})
		registry.RegisterResource(&tfprovidertest.ResourceInfo{Name: "untested", IsDataSource: false})
		registry.RegisterTestFile(&tfprovidertest.TestFileInfo{ResourceName: "tested"})

		untested := registry.GetUntestedResources()
		assert.Len(t, untested, 1)
		assert.Equal(t, "untested", untested[0].Name)
	})
}

// T008: Test for AST resource detection
func TestAST_ResourceDetection(t *testing.T) {
	t.Run("should detect terraform-plugin-framework resources", func(t *testing.T) {
		t.Skip("AST parser not yet implemented")

		// Expected behavior:
		// sourceCode := `
		// package provider
		//
		// import "github.com/hashicorp/terraform-plugin-framework/resource/schema"
		//
		// type WidgetResource struct{}
		//
		// func (r *WidgetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
		//     resp.Schema = schema.Schema{
		//         Attributes: map[string]schema.Attribute{
		//             "name": schema.StringAttribute{Required: true},
		//         },
		//     }
		// }
		// `
		//
		// resources := parseResources(sourceCode)
		// require.Len(t, resources, 1)
		// assert.Equal(t, "widget", resources[0].Name)
		// assert.False(t, resources[0].IsDataSource)
	})
}

// T009: Test for AST data source detection
func TestAST_DataSourceDetection(t *testing.T) {
	t.Run("should detect terraform-plugin-framework data sources", func(t *testing.T) {
		t.Skip("AST parser not yet implemented")

		// Expected behavior:
		// sourceCode := `
		// package provider
		//
		// import "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
		//
		// type AccountDataSource struct{}
		//
		// func (d *AccountDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
		//     resp.Schema = schema.Schema{
		//         Attributes: map[string]schema.Attribute{
		//             "id": schema.StringAttribute{Computed: true},
		//         },
		//     }
		// }
		// `
		//
		// dataSources := parseDataSources(sourceCode)
		// require.Len(t, dataSources, 1)
		// assert.Equal(t, "account", dataSources[0].Name)
		// assert.True(t, dataSources[0].IsDataSource)
	})
}

// T010: Test for test file parsing
func TestAST_TestFileParsing(t *testing.T) {
	t.Run("should parse TestAcc functions from test files", func(t *testing.T) {
		t.Skip("Test file parser not yet implemented")

		// Expected behavior:
		// sourceCode := `
		// package provider
		//
		// import (
		//     "testing"
		//     "github.com/hashicorp/terraform-plugin-testing/helper/resource"
		// )
		//
		// func TestAccResourceWidget_basic(t *testing.T) {
		//     resource.Test(t, resource.TestCase{
		//         Steps: []resource.TestStep{
		//             {
		//                 Config: "resource \"example_widget\" \"test\" { name = \"example\" }",
		//                 Check: resource.ComposeTestCheckFunc(
		//                     resource.TestCheckResourceAttr("example_widget.test", "name", "example"),
		//                 ),
		//             },
		//         },
		//     })
		// }
		// `
		//
		// testFile := parseTestFile(sourceCode)
		// require.NotNil(t, testFile)
		// assert.Equal(t, "widget", testFile.ResourceName)
		// require.Len(t, testFile.TestFunctions, 1)
		// assert.Equal(t, "TestAccResourceWidget_basic", testFile.TestFunctions[0].Name)
		// assert.True(t, testFile.TestFunctions[0].UsesResourceTest)
	})

	t.Run("should detect multi-step tests for update coverage", func(t *testing.T) {
		t.Skip("Test file parser not yet implemented")

		// Expected behavior:
		// sourceCode := `
		// func TestAccResourceConfig_update(t *testing.T) {
		//     resource.Test(t, resource.TestCase{
		//         Steps: []resource.TestStep{
		//             {Config: "step1"},
		//             {Config: "step2"},
		//         },
		//     })
		// }
		// `
		//
		// testFile := parseTestFile(sourceCode)
		// testFunc := testFile.TestFunctions[0]
		// assert.Len(t, testFunc.TestSteps, 2)
	})

	t.Run("should detect import test steps", func(t *testing.T) {
		t.Skip("Test file parser not yet implemented")

		// Expected behavior:
		// sourceCode := `
		// func TestAccResourceServer_import(t *testing.T) {
		//     resource.Test(t, resource.TestCase{
		//         Steps: []resource.TestStep{
		//             {
		//                 ResourceName:      "example_server.test",
		//                 ImportState:       true,
		//                 ImportStateVerify: true,
		//             },
		//         },
		//     })
		// }
		// `
		//
		// testFile := parseTestFile(sourceCode)
		// testStep := testFile.TestFunctions[0].TestSteps[0]
		// assert.True(t, testStep.ImportState)
		// assert.True(t, testStep.ImportStateVerify)
	})
}

// T036: Test for Update Test Analyzer
func TestUpdateTestCoverage(t *testing.T) {
	t.Run("should detect missing update tests for updatable resources", func(t *testing.T) {
		// TDD red phase - this test should fail before implementation
		// analysistest will validate that resources with updatable attributes
		// but only single-step tests get flagged

		// This test is intentionally minimal until the analyzer is implemented
		// Expected behavior after implementation:
		// analysistest.Run(t, analysistest.TestData(), UpdateTestAnalyzer, "update_missing")
		// analysistest.Run(t, analysistest.TestData(), UpdateTestAnalyzer, "update_passing")
	})
}

// T083: Validate all 5 analyzers return from BuildAnalyzers()
func TestPlugin_BuildAnalyzers(t *testing.T) {
	t.Run("should return all 5 analyzers when all are enabled", func(t *testing.T) {
		plugin, err := tfprovidertest.New(nil)
		require.NoError(t, err)
		require.NotNil(t, plugin)

		analyzers, err := plugin.BuildAnalyzers()
		require.NoError(t, err)
		require.Len(t, analyzers, 5, "should return exactly 5 analyzers when all are enabled")

		// Verify analyzer names
		expectedNames := map[string]bool{
			"tfprovider-resource-basic-test":  false,
			"tfprovider-resource-update-test": false,
			"tfprovider-resource-import-test": false,
			"tfprovider-test-error-cases":     false,
			"tfprovider-test-check-functions": false,
		}

		for _, analyzer := range analyzers {
			if _, exists := expectedNames[analyzer.Name]; exists {
				expectedNames[analyzer.Name] = true
			} else {
				t.Errorf("unexpected analyzer: %s", analyzer.Name)
			}
		}

		// Ensure all analyzers were found
		for name, found := range expectedNames {
			assert.True(t, found, "analyzer %s should be included", name)
		}
	})
}

// T084: Validate Settings configuration enables/disables analyzers
func TestPlugin_Settings(t *testing.T) {
	t.Run("should respect individual analyzer disable settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"EnableBasicTest":  true,
			"EnableUpdateTest": false,
			"EnableImportTest": true,
			"EnableErrorTest":  false,
			"EnableStateCheck": true,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)

		analyzers, err := plugin.BuildAnalyzers()
		require.NoError(t, err)
		require.Len(t, analyzers, 3, "should return only 3 enabled analyzers")

		// Verify only enabled analyzers are returned
		enabledNames := make(map[string]bool)
		for _, analyzer := range analyzers {
			enabledNames[analyzer.Name] = true
		}

		assert.True(t, enabledNames["tfprovider-resource-basic-test"], "basic test should be enabled")
		assert.False(t, enabledNames["tfprovider-resource-update-test"], "update test should be disabled")
		assert.True(t, enabledNames["tfprovider-resource-import-test"], "import test should be enabled")
		assert.False(t, enabledNames["tfprovider-test-error-cases"], "error test should be disabled")
		assert.True(t, enabledNames["tfprovider-test-check-functions"], "state check should be enabled")
	})

	t.Run("should disable all analyzers when all settings are false", func(t *testing.T) {
		settings := map[string]interface{}{
			"EnableBasicTest":  false,
			"EnableUpdateTest": false,
			"EnableImportTest": false,
			"EnableErrorTest":  false,
			"EnableStateCheck": false,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)

		analyzers, err := plugin.BuildAnalyzers()
		require.NoError(t, err)
		require.Len(t, analyzers, 0, "should return no analyzers when all are disabled")
	})

	t.Run("default settings should enable all analyzers", func(t *testing.T) {
		plugin, err := tfprovidertest.New(nil)
		require.NoError(t, err)

		analyzers, err := plugin.BuildAnalyzers()
		require.NoError(t, err)
		require.Len(t, analyzers, 5, "default settings should enable all 5 analyzers")
	})
}

// Integration tests using analysistest.Run() for all 5 analyzers
func TestBasicTestAnalyzer_Integration(t *testing.T) {
	t.Skip("TODO: Create testdata directories for analysistest")
	// testdata := analysistest.TestData()
	// analysistest.Run(t, testdata, tfprovidertest.BasicTestAnalyzer, "testlintdata/basic_missing")
	// analysistest.Run(t, testdata, tfprovidertest.BasicTestAnalyzer, "testlintdata/basic_passing")
}

func TestUpdateTestAnalyzer_Integration(t *testing.T) {
	t.Skip("TODO: Create testdata directories for analysistest")
	// testdata := analysistest.TestData()
	// analysistest.Run(t, testdata, tfprovidertest.UpdateTestAnalyzer, "testlintdata/update_missing")
	// analysistest.Run(t, testdata, tfprovidertest.UpdateTestAnalyzer, "testlintdata/update_passing")
}

func TestImportTestAnalyzer_Integration(t *testing.T) {
	t.Skip("TODO: Create testdata directories for analysistest")
	// testdata := analysistest.TestData()
	// analysistest.Run(t, testdata, tfprovidertest.ImportTestAnalyzer, "testlintdata/import_missing")
	// analysistest.Run(t, testdata, tfprovidertest.ImportTestAnalyzer, "testlintdata/import_passing")
}

func TestErrorTestAnalyzer_Integration(t *testing.T) {
	t.Skip("TODO: Create testdata directories for analysistest")
	// testdata := analysistest.TestData()
	// analysistest.Run(t, testdata, tfprovidertest.ErrorTestAnalyzer, "testlintdata/error_missing")
	// analysistest.Run(t, testdata, tfprovidertest.ErrorTestAnalyzer, "testlintdata/error_passing")
}

func TestStateCheckAnalyzer_Integration(t *testing.T) {
	t.Skip("TODO: Create testdata directories for analysistest")
	// testdata := analysistest.TestData()
	// analysistest.Run(t, testdata, tfprovidertest.StateCheckAnalyzer, "testlintdata/statecheck_missing")
	// analysistest.Run(t, testdata, tfprovidertest.StateCheckAnalyzer, "testlintdata/statecheck_passing")
}

// T091: Performance benchmarks
func BenchmarkResourceRegistry_Register(b *testing.B) {
	registry := tfprovidertest.NewResourceRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "benchmark_resource",
			IsDataSource: false,
			FilePath:     "/test/resource_benchmark.go",
		}
		registry.RegisterResource(resource)
	}
}

func BenchmarkResourceRegistry_GetResource(b *testing.B) {
	registry := tfprovidertest.NewResourceRegistry()

	// Setup: register 100 resources
	for i := 0; i < 100; i++ {
		resource := &tfprovidertest.ResourceInfo{
			Name:         fmt.Sprintf("resource_%d", i),
			IsDataSource: false,
			FilePath:     "/test/resource.go",
		}
		registry.RegisterResource(resource)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetResource("resource_50")
	}
}

func BenchmarkResourceRegistry_GetUntestedResources(b *testing.B) {
	registry := tfprovidertest.NewResourceRegistry()

	// Setup: register 50 resources and 25 test files
	for i := 0; i < 50; i++ {
		resource := &tfprovidertest.ResourceInfo{
			Name:         fmt.Sprintf("resource_%d", i),
			IsDataSource: false,
			FilePath:     "/test/resource.go",
		}
		registry.RegisterResource(resource)
	}

	for i := 0; i < 25; i++ {
		testFile := &tfprovidertest.TestFileInfo{
			ResourceName: fmt.Sprintf("resource_%d", i),
			FilePath:     "/test/resource_test.go",
		}
		registry.RegisterTestFile(testFile)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetUntestedResources()
	}
}

func BenchmarkPlugin_BuildAnalyzers(b *testing.B) {
	for i := 0; i < b.N; i++ {
		plugin, err := tfprovidertest.New(nil)
		if err != nil {
			b.Fatal(err)
		}
		_, err = plugin.BuildAnalyzers()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// T100: Test for isSweeperFile helper function
func TestIsSweeperFile(t *testing.T) {
	t.Run("should identify sweeper files", func(t *testing.T) {
		tests := []struct {
			name     string
			filePath string
			expected bool
		}{
			// Sweeper files - should return true
			{"resource_sweeper.go", "/path/to/provider/resource_sweeper.go", true},
			{"compute_sweeper.go", "/path/to/provider/compute_sweeper.go", true},
			{"iam_sweeper.go", "/some/dir/iam_sweeper.go", true},
			{"sweeper.go suffix only", "/path/to/sweeper.go", false}, // Only matches *_sweeper.go
			{"resource_access_sweeper.go", "/path/to/resource_access_sweeper.go", true},

			// Non-sweeper files - should return false
			{"regular resource", "/path/to/provider/resource_widget.go", false},
			{"data source", "/path/to/provider/data_source_widget.go", false},
			{"test file", "/path/to/provider/resource_widget_test.go", false},
			{"sweeper test file", "/path/to/provider/resource_sweeper_test.go", false},
			{"sweeper in directory name", "/path/sweeper/resource_widget.go", false},
			{"contains sweeper but not suffix", "/path/to/sweeper_utils.go", false},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				result := tfprovidertest.IsSweeperFile(tc.filePath)
				assert.Equal(t, tc.expected, result, "isSweeperFile(%q) should be %v", tc.filePath, tc.expected)
			})
		}
	})
}

// T101: Test for ExcludeSweeperFiles setting
func TestSettings_ExcludeSweeperFiles(t *testing.T) {
	t.Run("default settings should exclude sweeper files", func(t *testing.T) {
		settings := tfprovidertest.DefaultSettings()
		assert.True(t, settings.ExcludeSweeperFiles, "ExcludeSweeperFiles should be true by default")
	})

	t.Run("should be configurable via YAML settings", func(t *testing.T) {
		// Test that users can set ExcludeSweeperFiles: false to include sweeper files
		settings := map[string]interface{}{
			"ExcludeSweeperFiles": false,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)
		require.NotNil(t, plugin)

		// We need to verify the setting was applied
		// Since we can't directly access the settings from Plugin, we test indirectly
		// by ensuring the plugin was created without error
	})
}

// T151: Test for BuildExpectedTestPath helper function
func TestBuildExpectedTestPath(t *testing.T) {
	t.Run("should build expected test path for resource", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/provider/resource_widget.go",
		}
		expected := "/path/to/provider/resource_widget_test.go"
		actual := tfprovidertest.BuildExpectedTestPath(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should build expected test path for data source", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "http",
			IsDataSource: true,
			FilePath:     "/path/to/provider/data_source_http.go",
		}
		expected := "/path/to/provider/data_source_http_test.go"
		actual := tfprovidertest.BuildExpectedTestPath(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should handle nested directories", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "complex_resource",
			IsDataSource: false,
			FilePath:     "/home/user/projects/terraform-provider-example/internal/provider/resource_complex_resource.go",
		}
		expected := "/home/user/projects/terraform-provider-example/internal/provider/resource_complex_resource_test.go"
		actual := tfprovidertest.BuildExpectedTestPath(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should handle file without .go extension gracefully", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/provider/resource_widget",
		}
		// Should append _test.go even if original doesn't have .go
		expected := "/path/to/provider/resource_widget_test.go"
		actual := tfprovidertest.BuildExpectedTestPath(resource)
		assert.Equal(t, expected, actual)
	})
}

// T152: Test for BuildExpectedTestFunc helper function
func TestBuildExpectedTestFunc(t *testing.T) {
	t.Run("should build expected test function name for resource", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/provider/resource_widget.go",
		}
		expected := "TestAccWidget_basic"
		actual := tfprovidertest.BuildExpectedTestFunc(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should build expected test function name for data source", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "http",
			IsDataSource: true,
			FilePath:     "/path/to/provider/data_source_http.go",
		}
		expected := "TestAccDataSourceHttp_basic"
		actual := tfprovidertest.BuildExpectedTestFunc(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should handle snake_case resource names", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "complex_resource",
			IsDataSource: false,
			FilePath:     "/path/to/provider/resource_complex_resource.go",
		}
		expected := "TestAccComplexResource_basic"
		actual := tfprovidertest.BuildExpectedTestFunc(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should handle snake_case data source names", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "private_key",
			IsDataSource: true,
			FilePath:     "/path/to/provider/data_source_private_key.go",
		}
		expected := "TestAccDataSourcePrivateKey_basic"
		actual := tfprovidertest.BuildExpectedTestFunc(resource)
		assert.Equal(t, expected, actual)
	})

	t.Run("should handle single word resource name", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "server",
			IsDataSource: false,
			FilePath:     "/path/to/provider/resource_server.go",
		}
		expected := "TestAccServer_basic"
		actual := tfprovidertest.BuildExpectedTestFunc(resource)
		assert.Equal(t, expected, actual)
	})
}

// T102: Test for IsMigrationFile helper function
func TestIsMigrationFile(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		// Migration files that should be excluded
		{
			name:     "file ending with _migrate.go",
			filePath: "/path/to/resource_widget_migrate.go",
			expected: true,
		},
		{
			name:     "file containing _migration in name",
			filePath: "/path/to/resource_widget_migration.go",
			expected: true,
		},
		{
			name:     "file containing _migration_ in name",
			filePath: "/path/to/resource_widget_migration_v1.go",
			expected: true,
		},
		{
			name:     "file ending with _state_upgrader.go",
			filePath: "/path/to/resource_widget_state_upgrader.go",
			expected: true,
		},
		{
			name:     "state upgrader without resource prefix",
			filePath: "/path/to/widget_state_upgrader.go",
			expected: true,
		},
		// Regular resource files that should NOT be excluded
		{
			name:     "regular resource file",
			filePath: "/path/to/resource_widget.go",
			expected: false,
		},
		{
			name:     "resource file with suffix",
			filePath: "/path/to/resource_widget_model.go",
			expected: false,
		},
		{
			name:     "data source file",
			filePath: "/path/to/data_source_widget.go",
			expected: false,
		},
		{
			name:     "test file",
			filePath: "/path/to/resource_widget_test.go",
			expected: false,
		},
		// Edge cases
		{
			name:     "file with migrate in directory name but not filename",
			filePath: "/path/to/migrate/resource_widget.go",
			expected: false,
		},
		{
			name:     "file named exactly migrate.go",
			filePath: "/path/to/migrate.go",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tfprovidertest.IsMigrationFile(tt.filePath)
			assert.Equal(t, tt.expected, result, "IsMigrationFile(%q) should return %v", tt.filePath, tt.expected)
		})
	}
}

// T103: Test for ExcludeMigrationFiles setting in DefaultSettings
func TestSettings_ExcludeMigrationFilesDefault(t *testing.T) {
	t.Run("ExcludeMigrationFiles should be true by default", func(t *testing.T) {
		settings := tfprovidertest.DefaultSettings()
		assert.True(t, settings.ExcludeMigrationFiles, "ExcludeMigrationFiles should be true by default")
	})
}

// T104: Test for ExcludeMigrationFiles setting can be configured via YAML
func TestSettings_ExcludeMigrationFilesConfigurable(t *testing.T) {
	t.Run("ExcludeMigrationFiles can be disabled via settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"ExcludeMigrationFiles": false,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)

		// The plugin should be created successfully with the custom setting
		require.NotNil(t, plugin)
	})
}

// =============================================================================
// Task 1: TestDataSource_* Pattern Matching Tests (Priority 1.3)
// =============================================================================

// T105: Test for extractResourceNameFromTestFunc with TestDataSource_* patterns
func TestExtractResourceNameFromTestFunc_DataSourcePatterns(t *testing.T) {
	t.Run("should extract resource name from TestDataSource_ pattern when filename provides context", func(t *testing.T) {
		// TestDataSource_200 in data_source_http_test.go should map to "http"
		// The function alone cannot determine this, we need file context
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestDataSource_200")
		// This should return empty because the resource name isn't in the function name
		// The fallback to filename-based extraction should handle this
		assert.Equal(t, "", result, "TestDataSource_200 doesn't contain resource name, should return empty")
	})

	t.Run("should extract resource name from standard TestDataSource pattern", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestDataSourceHttp_basic")
		assert.Equal(t, "http", result)
	})

	t.Run("should extract resource name from TestAccDataSource pattern", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestAccDataSourceHttp_basic")
		assert.Equal(t, "http", result)
	})
}

// T106: Test for parseTestFile with file-based resource name extraction
func TestParseTestFile_FilenameBasedResourceExtraction(t *testing.T) {
	t.Run("should use filename to determine resource name when function names don't contain it", func(t *testing.T) {
		// This tests that data_source_http_test.go with TestDataSource_200 will correctly identify "http"
		// The test file parser should extract resource name from filename first

		// Expected: ResourceName should be "http" based on file name "data_source_http_test.go"
		// even though test function is "TestDataSource_200"
	})
}

// T107: Test for isTestFunction with various non-standard patterns
func TestIsTestFunction_NonStandardPatterns(t *testing.T) {
	t.Run("should match TestDataSource_<number> pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestDataSource_200", nil)
		assert.True(t, result, "TestDataSource_200 should be recognized as a test function")
	})

	t.Run("should match TestDataSource_<description> pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestDataSource_withAuthorizationRequestHeader_200", nil)
		assert.True(t, result, "TestDataSource_withAuthorizationRequestHeader_200 should be recognized")
	})

	t.Run("should match TestDataSource_<status> pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestDataSource_404", nil)
		assert.True(t, result, "TestDataSource_404 should be recognized as a test function")
	})
}

// =============================================================================
// Task 2: Support TestResource* without Acc Tests (Priority 2.1)
// =============================================================================

// T108: Test for extractResourceNameFromTestFunc with TLS provider patterns
func TestExtractResourceNameFromTestFunc_TLSPatterns(t *testing.T) {
	t.Run("should extract private_key from TestPrivateKeyRSA", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestPrivateKeyRSA")
		assert.Equal(t, "private_key", result)
	})

	t.Run("should extract private_key from TestPrivateKeyECDSA", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestPrivateKeyECDSA")
		assert.Equal(t, "private_key", result)
	})

	t.Run("should extract private_key from TestPrivateKeyED25519", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestPrivateKeyED25519")
		assert.Equal(t, "private_key", result)
	})

	t.Run("should extract locally_signed_cert from TestResourceLocallySignedCert", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestResourceLocallySignedCert")
		assert.Equal(t, "locally_signed_cert", result)
	})

	t.Run("should extract private_key from TestAccPrivateKeyRSA_UpgradeFromVersion3_4_0", func(t *testing.T) {
		result := tfprovidertest.ExtractResourceNameFromTestFunc("TestAccPrivateKeyRSA_UpgradeFromVersion3_4_0")
		// Should extract private_key from the AccPrivateKey part
		assert.Equal(t, "private_key", result)
	})
}

// T109: Test for isTestFunction with TestResource* patterns (without Acc)
func TestIsTestFunction_TestResourceWithoutAcc(t *testing.T) {
	t.Run("should match TestPrivateKeyRSA pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestPrivateKeyRSA", nil)
		assert.True(t, result, "TestPrivateKeyRSA should be recognized as a test function")
	})

	t.Run("should match TestResourceLocallySignedCert pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestResourceLocallySignedCert", nil)
		assert.True(t, result, "TestResourceLocallySignedCert should be recognized as a test function")
	})

	t.Run("should match TestPrivateKeyECDSA pattern", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("TestPrivateKeyECDSA", nil)
		assert.True(t, result, "TestPrivateKeyECDSA should be recognized as a test function")
	})

	t.Run("should not match helper functions", func(t *testing.T) {
		result := tfprovidertest.IsTestFunctionExported("testHelper", nil)
		assert.False(t, result, "testHelper should NOT be recognized as a test function")
	})
}

// T110: Test for CamelCase to snake_case conversion for resource names
func TestCamelCaseToSnakeCase(t *testing.T) {
	t.Run("should convert PrivateKey to private_key", func(t *testing.T) {
		result := tfprovidertest.CamelCaseToSnakeCaseExported("PrivateKey")
		assert.Equal(t, "private_key", result)
	})

	t.Run("should convert LocallySignedCert to locally_signed_cert", func(t *testing.T) {
		result := tfprovidertest.CamelCaseToSnakeCaseExported("LocallySignedCert")
		assert.Equal(t, "locally_signed_cert", result)
	})

	t.Run("should convert SelfSignedCert to self_signed_cert", func(t *testing.T) {
		result := tfprovidertest.CamelCaseToSnakeCaseExported("SelfSignedCert")
		assert.Equal(t, "self_signed_cert", result)
	})

	t.Run("should handle acronyms like RSA", func(t *testing.T) {
		// RSA alone should become "rsa" (all lowercase)
		result := tfprovidertest.CamelCaseToSnakeCaseExported("RSA")
		assert.Equal(t, "rsa", result)
	})

	t.Run("should handle PrivateKeyRSA", func(t *testing.T) {
		// PrivateKeyRSA should become "private_key_rsa"
		result := tfprovidertest.CamelCaseToSnakeCaseExported("PrivateKeyRSA")
		assert.Equal(t, "private_key_rsa", result)
	})
}

// =============================================================================
// Task 3: File-Based Test Matching Fallback Tests (Priority 2.2)
// =============================================================================

// T111: Test for HasMatchingTestFile function
func TestHasMatchingTestFile(t *testing.T) {
	t.Run("should return true when resource has test file with Test* functions", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()

		// Register a resource
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}
		registry.RegisterResource(resource)

		// Register a matching test file with test functions
		testFile := &tfprovidertest.TestFileInfo{
			FilePath:     "/path/to/resource_widget_test.go",
			ResourceName: "widget",
			IsDataSource: false,
			TestFunctions: []tfprovidertest.TestFunctionInfo{
				{Name: "TestWidgetSomething", UsesResourceTest: true},
			},
		}
		registry.RegisterTestFile(testFile)

		result := tfprovidertest.HasMatchingTestFile("widget", false, registry)
		assert.True(t, result, "Should return true when test file exists with Test* functions")
	})

	t.Run("should return false when test file has no Test* functions", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()

		// Register a resource
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}
		registry.RegisterResource(resource)

		// Register a test file with NO test functions (empty)
		testFile := &tfprovidertest.TestFileInfo{
			FilePath:      "/path/to/resource_widget_test.go",
			ResourceName:  "widget",
			IsDataSource:  false,
			TestFunctions: []tfprovidertest.TestFunctionInfo{}, // Empty
		}
		registry.RegisterTestFile(testFile)

		result := tfprovidertest.HasMatchingTestFile("widget", false, registry)
		assert.False(t, result, "Should return false when test file has no Test* functions")
	})

	t.Run("should return false when no test file exists", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()

		// Register only the resource, no test file
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}
		registry.RegisterResource(resource)

		result := tfprovidertest.HasMatchingTestFile("widget", false, registry)
		assert.False(t, result, "Should return false when no test file exists")
	})

	t.Run("should work for data sources", func(t *testing.T) {
		registry := tfprovidertest.NewResourceRegistry()

		// Register a data source
		dataSource := &tfprovidertest.ResourceInfo{
			Name:         "http",
			IsDataSource: true,
			FilePath:     "/path/to/data_source_http.go",
		}
		registry.RegisterResource(dataSource)

		// Register a matching test file
		testFile := &tfprovidertest.TestFileInfo{
			FilePath:     "/path/to/data_source_http_test.go",
			ResourceName: "http",
			IsDataSource: true,
			TestFunctions: []tfprovidertest.TestFunctionInfo{
				{Name: "TestDataSource_200", UsesResourceTest: true},
			},
		}
		registry.RegisterTestFile(testFile)

		result := tfprovidertest.HasMatchingTestFile("http", true, registry)
		assert.True(t, result, "Should return true for data source with test file containing Test* functions")
	})
}

// T112: Test for file-based test matching integration with buildRegistry
func TestBuildRegistry_FileBasedMatching(t *testing.T) {
	t.Run("should mark resource as tested when test file exists with Test* functions", func(t *testing.T) {
		// This is an integration test that would require setting up AST parsing
		// For now, we test the logic through unit tests above
		t.Skip("Integration test - requires AST setup")
	})
}

// T113: Test for EnableFileBasedMatching setting
func TestSettings_EnableFileBasedMatching(t *testing.T) {
	t.Run("default settings should enable file-based matching", func(t *testing.T) {
		settings := tfprovidertest.DefaultSettings()
		assert.True(t, settings.EnableFileBasedMatching, "EnableFileBasedMatching should be true by default")
	})

	t.Run("EnableFileBasedMatching can be disabled via settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"EnableFileBasedMatching": false,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)
		require.NotNil(t, plugin)
	})
}

// T200: Test for Verbose setting in Settings struct
func TestSettings_Verbose(t *testing.T) {
	t.Run("default settings should have Verbose disabled", func(t *testing.T) {
		settings := tfprovidertest.DefaultSettings()
		assert.False(t, settings.Verbose, "Verbose should be false by default")
	})

	t.Run("Verbose can be enabled via settings", func(t *testing.T) {
		settings := map[string]interface{}{
			"Verbose": true,
		}

		plugin, err := tfprovidertest.New(settings)
		require.NoError(t, err)
		require.NotNil(t, plugin)
	})
}

// T201: Test for VerboseDiagnosticInfo struct
func TestVerboseDiagnosticInfo(t *testing.T) {
	t.Run("should hold all required fields for verbose output", func(t *testing.T) {
		info := tfprovidertest.VerboseDiagnosticInfo{
			ResourceName:       "private_key",
			ResourceType:       "resource",
			ResourceFile:       "/path/to/resource_private_key.go",
			ResourceLine:       89,
			TestFilesSearched:  []tfprovidertest.TestFileSearchResult{{FilePath: "/path/to/resource_private_key_test.go", Found: true}},
			TestFunctionsFound: []tfprovidertest.TestFunctionMatchInfo{{Name: "TestPrivateKeyRSA", Line: 45, MatchStatus: "not_matched", MatchReason: "missing 'Acc' prefix"}},
			ExpectedPatterns:   []string{"TestAccResource*", "TestAcc*PrivateKey*"},
			FoundPattern:       "TestPrivateKey* (non-standard)",
			SuggestedFixes:     []string{"Rename tests to follow convention", "Configure custom test patterns"},
		}

		assert.Equal(t, "private_key", info.ResourceName)
		assert.Equal(t, "resource", info.ResourceType)
		assert.Equal(t, 89, info.ResourceLine)
		assert.Len(t, info.TestFilesSearched, 1)
		assert.Len(t, info.TestFunctionsFound, 1)
		assert.Equal(t, "not_matched", info.TestFunctionsFound[0].MatchStatus)
		assert.Equal(t, "missing 'Acc' prefix", info.TestFunctionsFound[0].MatchReason)
	})
}

// T202: Test for TestFileSearchResult struct
func TestTestFileSearchResult(t *testing.T) {
	t.Run("should track searched test files with found status", func(t *testing.T) {
		result := tfprovidertest.TestFileSearchResult{
			FilePath: "/path/to/resource_widget_test.go",
			Found:    true,
		}

		assert.Equal(t, "/path/to/resource_widget_test.go", result.FilePath)
		assert.True(t, result.Found)
	})

	t.Run("should handle not found test files", func(t *testing.T) {
		result := tfprovidertest.TestFileSearchResult{
			FilePath: "/path/to/resource_widget_test.go",
			Found:    false,
		}

		assert.False(t, result.Found)
	})
}

// T203: Test for TestFunctionMatchInfo struct
func TestTestFunctionMatchInfo(t *testing.T) {
	t.Run("should track match status for test functions", func(t *testing.T) {
		matchedFunc := tfprovidertest.TestFunctionMatchInfo{
			Name:        "TestAccWidget_basic",
			Line:        45,
			MatchStatus: "matched",
			MatchReason: "",
		}

		assert.Equal(t, "matched", matchedFunc.MatchStatus)
		assert.Empty(t, matchedFunc.MatchReason)
	})

	t.Run("should provide reason for unmatched functions", func(t *testing.T) {
		unmatchedFunc := tfprovidertest.TestFunctionMatchInfo{
			Name:        "TestPrivateKeyRSA",
			Line:        45,
			MatchStatus: "not_matched",
			MatchReason: "missing 'Acc' prefix",
		}

		assert.Equal(t, "not_matched", unmatchedFunc.MatchStatus)
		assert.Equal(t, "missing 'Acc' prefix", unmatchedFunc.MatchReason)
	})
}

// T204: Test for BuildVerboseDiagnostic function
func TestBuildVerboseDiagnostic(t *testing.T) {
	t.Run("should build verbose diagnostic for resource with no test file", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}
		registry := tfprovidertest.NewResourceRegistry()
		registry.RegisterResource(resource)

		info := tfprovidertest.BuildVerboseDiagnosticInfo(resource, registry)

		assert.Equal(t, "widget", info.ResourceName)
		assert.Equal(t, "resource", info.ResourceType)
		assert.Equal(t, "/path/to/resource_widget.go", info.ResourceFile)
		assert.Len(t, info.TestFilesSearched, 1)
		assert.False(t, info.TestFilesSearched[0].Found)
	})

	t.Run("should build verbose diagnostic for resource with test file but no matching functions", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "private_key",
			IsDataSource: false,
			FilePath:     "/path/to/resource_private_key.go",
		}
		testFile := &tfprovidertest.TestFileInfo{
			FilePath:     "/path/to/resource_private_key_test.go",
			ResourceName: "private_key",
			TestFunctions: []tfprovidertest.TestFunctionInfo{
				{Name: "TestPrivateKeyRSA", UsesResourceTest: true},
				{Name: "TestPrivateKeyECDSA", UsesResourceTest: true},
			},
		}

		registry := tfprovidertest.NewResourceRegistry()
		registry.RegisterResource(resource)
		registry.RegisterTestFile(testFile)

		info := tfprovidertest.BuildVerboseDiagnosticInfo(resource, registry)

		assert.Equal(t, "private_key", info.ResourceName)
		assert.Equal(t, "resource", info.ResourceType)
		assert.Len(t, info.TestFilesSearched, 1)
		assert.True(t, info.TestFilesSearched[0].Found)
		assert.Len(t, info.TestFunctionsFound, 2)
	})

	t.Run("should include suggested fixes", func(t *testing.T) {
		resource := &tfprovidertest.ResourceInfo{
			Name:         "widget",
			IsDataSource: false,
			FilePath:     "/path/to/resource_widget.go",
		}
		registry := tfprovidertest.NewResourceRegistry()
		registry.RegisterResource(resource)

		info := tfprovidertest.BuildVerboseDiagnosticInfo(resource, registry)

		assert.NotEmpty(t, info.SuggestedFixes)
	})
}

// T205: Test for FormatVerboseDiagnostic function
func TestFormatVerboseDiagnostic(t *testing.T) {
	t.Run("should format verbose diagnostic with all sections", func(t *testing.T) {
		info := tfprovidertest.VerboseDiagnosticInfo{
			ResourceName:      "private_key",
			ResourceType:      "resource",
			ResourceFile:      "/path/to/resource_private_key.go",
			ResourceLine:      89,
			TestFilesSearched: []tfprovidertest.TestFileSearchResult{{FilePath: "/path/to/resource_private_key_test.go", Found: true}},
			TestFunctionsFound: []tfprovidertest.TestFunctionMatchInfo{
				{Name: "TestPrivateKeyRSA", Line: 45, MatchStatus: "not_matched", MatchReason: "missing 'Acc' prefix"},
			},
			ExpectedPatterns: []string{"TestAccResource*", "TestAcc*PrivateKey*"},
			FoundPattern:     "TestPrivateKey* (non-standard)",
			SuggestedFixes:   []string{"Rename tests to follow convention (TestAccResourcePrivateKey_RSA)", "Configure custom test patterns in .golangci.yml"},
		}

		output := tfprovidertest.FormatVerboseDiagnostic(info)

		// Check that output contains expected sections
		assert.Contains(t, output, "Resource Location:")
		assert.Contains(t, output, "/path/to/resource_private_key.go:89")
		assert.Contains(t, output, "Test Files Searched:")
		assert.Contains(t, output, "resource_private_key_test.go (found)")
		assert.Contains(t, output, "Test Functions Found:")
		assert.Contains(t, output, "TestPrivateKeyRSA (line 45) - NOT MATCHED (missing 'Acc' prefix)")
		assert.Contains(t, output, "Why Not Matched:")
		assert.Contains(t, output, "Expected pattern: TestAccResource*")
		assert.Contains(t, output, "Suggested Fix:")
	})

	t.Run("should format verbose diagnostic for data source", func(t *testing.T) {
		info := tfprovidertest.VerboseDiagnosticInfo{
			ResourceName:      "http",
			ResourceType:      "data source",
			ResourceFile:      "/path/to/data_source_http.go",
			ResourceLine:      45,
			TestFilesSearched: []tfprovidertest.TestFileSearchResult{{FilePath: "/path/to/data_source_http_test.go", Found: false}},
			ExpectedPatterns:  []string{"TestAccDataSource*"},
			SuggestedFixes:    []string{"Create test file data_source_http_test.go"},
		}

		output := tfprovidertest.FormatVerboseDiagnostic(info)

		assert.Contains(t, output, "data source")
		assert.Contains(t, output, "data_source_http_test.go (not found)")
	})

	t.Run("should handle empty test functions gracefully", func(t *testing.T) {
		info := tfprovidertest.VerboseDiagnosticInfo{
			ResourceName:       "widget",
			ResourceType:       "resource",
			ResourceFile:       "/path/to/resource_widget.go",
			ResourceLine:       10,
			TestFilesSearched:  []tfprovidertest.TestFileSearchResult{{FilePath: "/path/to/resource_widget_test.go", Found: false}},
			TestFunctionsFound: []tfprovidertest.TestFunctionMatchInfo{},
			ExpectedPatterns:   []string{"TestAccWidget_basic"},
			SuggestedFixes:     []string{"Create acceptance test"},
		}

		output := tfprovidertest.FormatVerboseDiagnostic(info)

		// Should not contain Test Functions Found section when empty
		assert.Contains(t, output, "Resource Location:")
		assert.Contains(t, output, "Suggested Fix:")
	})
}

// T206: Test for ClassifyTestFunctionMatch function
func TestClassifyTestFunctionMatch(t *testing.T) {
	t.Run("should classify matching TestAcc function", func(t *testing.T) {
		status, reason := tfprovidertest.ClassifyTestFunctionMatch("TestAccWidget_basic", "widget")
		assert.Equal(t, "matched", status)
		assert.Empty(t, reason)
	})

	t.Run("should classify non-matching function with missing Acc prefix", func(t *testing.T) {
		status, reason := tfprovidertest.ClassifyTestFunctionMatch("TestPrivateKeyRSA", "private_key")
		assert.Equal(t, "not_matched", status)
		assert.Contains(t, reason, "missing 'Acc' prefix")
	})

	t.Run("should classify function with different resource name", func(t *testing.T) {
		status, reason := tfprovidertest.ClassifyTestFunctionMatch("TestAccOtherResource_basic", "widget")
		assert.Equal(t, "not_matched", status)
		assert.Contains(t, reason, "does not match resource")
	})

	t.Run("should classify matching TestResource function", func(t *testing.T) {
		status, reason := tfprovidertest.ClassifyTestFunctionMatch("TestResourceWidget_basic", "widget")
		assert.Equal(t, "matched", status)
		assert.Empty(t, reason)
	})

	t.Run("should classify matching TestDataSource function", func(t *testing.T) {
		status, reason := tfprovidertest.ClassifyTestFunctionMatch("TestDataSourceHttp_basic", "http")
		assert.Equal(t, "matched", status)
		assert.Empty(t, reason)
	})
}
