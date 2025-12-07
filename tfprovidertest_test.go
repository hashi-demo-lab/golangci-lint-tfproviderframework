package tfprovidertest_test

import (
	"testing"

	tfprovidertest "github.com/example/tfprovidertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T006: Test for Settings defaults
func TestSettings_Defaults(t *testing.T) {
	// This test will fail initially - TDD red phase
	t.Run("default settings should enable all analyzers", func(t *testing.T) {
		// We'll implement Settings struct later
		// For now, this test documents the expected behavior
		t.Skip("Settings struct not yet implemented")

		// Expected behavior:
		// settings := Settings{}
		// assert.True(t, settings.EnableBasicTest)
		// assert.True(t, settings.EnableUpdateTest)
		// assert.True(t, settings.EnableImportTest)
		// assert.True(t, settings.EnableErrorTest)
		// assert.True(t, settings.EnableStateCheck)
		// assert.Equal(t, "resource_*.go", settings.ResourcePathPattern)
		// assert.Equal(t, "data_source_*.go", settings.DataSourcePathPattern)
		// assert.Equal(t, "*_test.go", settings.TestFilePattern)
		// assert.Empty(t, settings.ExcludePaths)
	})
}

// T007: Test for ResourceRegistry operations
func TestResourceRegistry_Operations(t *testing.T) {
	t.Run("should register resources and retrieve them", func(t *testing.T) {
		t.Skip("ResourceRegistry not yet implemented")

		// Expected behavior:
		// registry := NewResourceRegistry()
		//
		// resource := &ResourceInfo{
		//     Name: "widget",
		//     IsDataSource: false,
		//     FilePath: "/test/resource_widget.go",
		// }
		// registry.RegisterResource(resource)
		//
		// retrieved := registry.GetResource("widget")
		// require.NotNil(t, retrieved)
		// assert.Equal(t, "widget", retrieved.Name)
	})

	t.Run("should track untested resources", func(t *testing.T) {
		t.Skip("ResourceRegistry not yet implemented")

		// Expected behavior:
		// registry := NewResourceRegistry()
		// registry.RegisterResource(&ResourceInfo{Name: "tested", IsDataSource: false})
		// registry.RegisterResource(&ResourceInfo{Name: "untested", IsDataSource: false})
		// registry.RegisterTestFile(&TestFileInfo{ResourceName: "tested"})
		//
		// untested := registry.GetUntestedResources()
		// assert.Len(t, untested, 1)
		// assert.Equal(t, "untested", untested[0].Name)
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
		t.Skip("UpdateTestAnalyzer not yet implemented")

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
			Name:         "resource_" + string(rune(i)),
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
			Name:         "resource_" + string(rune(i)),
			IsDataSource: false,
			FilePath:     "/test/resource.go",
		}
		registry.RegisterResource(resource)
	}

	for i := 0; i < 25; i++ {
		testFile := &tfprovidertest.TestFileInfo{
			ResourceName: "resource_" + string(rune(i)),
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
