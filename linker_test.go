// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"strings"
	"testing"

	"github.com/example/tfprovidertest/internal/analysis"
	"github.com/example/tfprovidertest/internal/matching"
	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
)

func TestLinkerFunctionNameMatching(t *testing.T) {
	reg := registry.NewResourceRegistry()

	// Register resources
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})
	reg.RegisterResource(&registry.ResourceInfo{Name: "instance"})

	// Register test functions
	fn1 := &registry.TestFunctionInfo{Name: "TestAccWidget_basic", FilePath: "/test.go"}
	fn2 := &registry.TestFunctionInfo{Name: "TestAccInstance_update", FilePath: "/test.go"}
	reg.RegisterTestFunction(fn1)
	reg.RegisterTestFunction(fn2)

	// Run linker
	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Verify matches
	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 1 {
		t.Errorf("expected 1 widget test, got %d", len(widgetTests))
	}
	if len(widgetTests) > 0 {
		if widgetTests[0].MatchType != registry.MatchTypeFunctionName {
			t.Errorf("expected MatchTypeFunctionName, got %v", widgetTests[0].MatchType)
		}
		if widgetTests[0].MatchConfidence != 0.95 {
			t.Errorf("expected confidence 0.95, got %f", widgetTests[0].MatchConfidence)
		}
	}

	instanceTests := reg.GetResourceTests("instance")
	if len(instanceTests) != 1 {
		t.Errorf("expected 1 instance test, got %d", len(instanceTests))
	}
}

func TestLinkerFileProximityMatching(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})

	// Test function with non-standard name but matching file
	fn := &registry.TestFunctionInfo{
		Name:     "TestWidgetOperations_all", // Doesn't follow TestAcc pattern
		FilePath: "/path/to/resource_widget_test.go",
	}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 1 {
		t.Errorf("expected 1 widget test, got %d", len(widgetTests))
	}
	if len(widgetTests) > 0 {
		if widgetTests[0].MatchType != registry.MatchTypeFileProximity {
			t.Errorf("expected MatchTypeFileProximity, got %v", widgetTests[0].MatchType)
		}
		if widgetTests[0].MatchConfidence != 0.9 {
			t.Errorf("expected confidence 0.9, got %f", widgetTests[0].MatchConfidence)
		}
	}
}

func TestLinkerFileProximityDataSource(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "http", Kind: registry.KindDataSource})

	// Test function in data source file
	fn := &registry.TestFunctionInfo{
		Name:     "TestHTTPDataSource_basic",
		FilePath: "/path/to/data_source_http_test.go",
	}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	httpTests := reg.GetResourceTests("http")
	if len(httpTests) != 1 {
		t.Errorf("expected 1 http test, got %d", len(httpTests))
	}
}

func TestLinkerFuzzyMatching(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})

	// Test function with slightly different name
	fn := &registry.TestFunctionInfo{
		Name:     "TestAccWidgets_basic", // "widgets" instead of "widget"
		FilePath: "/path/to/test.go",
	}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	settings.EnableFuzzyMatching = true
	settings.FuzzyMatchThreshold = 0.7
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 1 {
		t.Errorf("expected 1 widget test from fuzzy match, got %d", len(widgetTests))
	}
	if len(widgetTests) > 0 && widgetTests[0].MatchType != registry.MatchTypeFuzzy {
		t.Errorf("expected MatchTypeFuzzy, got %v", widgetTests[0].MatchType)
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
		{"", "abc", 3},
		{"abc", "", 3},
		{"flaw", "lawn", 2},
		{"gumbo", "gambol", 2},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := matching.LevenshteinDistance(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("LevenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		a, b        string
		minExpected float64
		maxExpected float64
	}{
		{"widget", "widget", 1.0, 1.0},
		{"widget", "widgets", 0.8, 1.0}, // 1 char difference in 7 char string
		{"instance", "instances", 0.8, 1.0},
		{"bucket", "socket", 0.5, 0.7}, // Different words
		{"", "", 1.0, 1.0},
		{"abc", "xyz", 0.0, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := matching.CalculateSimilarity(tt.a, tt.b)
			if got < tt.minExpected {
				t.Errorf("CalculateSimilarity(%q, %q) = %f, want >= %f", tt.a, tt.b, got, tt.minExpected)
			}
			if got > tt.maxExpected {
				t.Errorf("CalculateSimilarity(%q, %q) = %f, want <= %f", tt.a, tt.b, got, tt.maxExpected)
			}
		})
	}
}

func TestLinkerNoMatch(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})

	// Test function that doesn't match any resource
	fn := &registry.TestFunctionInfo{
		Name:     "TestAccOrphanResource_basic",
		FilePath: "/path/to/orphan_test.go",
	}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Should have no matches for widget since function doesn't match
	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 0 {
		t.Errorf("expected 0 widget tests, got %d", len(widgetTests))
	}

	// Function should have no inferred resources
	unmatched := reg.GetUnmatchedTestFunctions()
	if len(unmatched) != 1 {
		t.Errorf("expected 1 unmatched test, got %d", len(unmatched))
	}
}

func TestLinkerMultipleResources(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})
	reg.RegisterResource(&registry.ResourceInfo{Name: "gadget"})
	reg.RegisterResource(&registry.ResourceInfo{Name: "device"})

	// Register test functions
	fn1 := &registry.TestFunctionInfo{Name: "TestAccWidget_basic", FilePath: "/test.go"}
	fn2 := &registry.TestFunctionInfo{Name: "TestAccWidget_update", FilePath: "/test.go"}
	fn3 := &registry.TestFunctionInfo{Name: "TestAccGadget_basic", FilePath: "/test.go"}
	reg.RegisterTestFunction(fn1)
	reg.RegisterTestFunction(fn2)
	reg.RegisterTestFunction(fn3)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 2 {
		t.Errorf("expected 2 widget tests, got %d", len(widgetTests))
	}

	gadgetTests := reg.GetResourceTests("gadget")
	if len(gadgetTests) != 1 {
		t.Errorf("expected 1 gadget test, got %d", len(gadgetTests))
	}

	deviceTests := reg.GetResourceTests("device")
	if len(deviceTests) != 0 {
		t.Errorf("expected 0 device tests (no tests for it), got %d", len(deviceTests))
	}
}

func TestLinkerDataSourceMatching(t *testing.T) {
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "ami", Kind: registry.KindDataSource})

	fn := &registry.TestFunctionInfo{Name: "TestAccDataSourceAmi_basic", FilePath: "/test.go"}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	amiTests := reg.GetResourceTests("ami")
	if len(amiTests) != 1 {
		t.Errorf("expected 1 ami test, got %d", len(amiTests))
	}
}

func TestLinkerInferredMatching(t *testing.T) {
	reg := registry.NewResourceRegistry()

	// Register a resource named "example_widget"
	reg.RegisterResource(&registry.ResourceInfo{Name: "example_widget"})

	// Create a test function with InferredResources (simulating what extractTestSteps would populate
	// when parsing a Config string containing `resource "example_widget" "test" {`)
	fn := &registry.TestFunctionInfo{
		Name:              "TestSomeArbitraryName_basic",
		FilePath:          "/path/to/arbitrary_test.go",
		InferredResources: []string{"example_widget"},
	}
	reg.RegisterTestFunction(fn)

	// Run linker
	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Verify the test is linked to "example_widget" with MatchTypeInferred
	widgetTests := reg.GetResourceTests("example_widget")
	if len(widgetTests) != 1 {
		t.Fatalf("expected 1 example_widget test, got %d", len(widgetTests))
	}

	if widgetTests[0].MatchType != registry.MatchTypeInferred {
		t.Errorf("expected MatchTypeInferred, got %v", widgetTests[0].MatchType)
	}
	if widgetTests[0].MatchConfidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", widgetTests[0].MatchConfidence)
	}
}

func TestLinkerInferredMatchingPriority(t *testing.T) {
	// Test that function name matching takes priority over inferred matching
	// This ensures that when a test's function name clearly indicates the resource,
	// we use that even if the config references other resources as dependencies
	reg := registry.NewResourceRegistry()

	// Register two resources
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})
	reg.RegisterResource(&registry.ResourceInfo{Name: "gadget"})

	// Create a test function whose name suggests "widget" but has "gadget" inferred from config
	// Priority: function name > file proximity > inferred
	fn := &registry.TestFunctionInfo{
		Name:              "TestAccWidget_basic",                  // Function name suggests "widget"
		FilePath:          "/path/to/resource_widget_test.go",     // File suggests "widget"
		InferredResources: []string{"gadget"},                     // Config references "gadget" as dependency
	}
	reg.RegisterTestFunction(fn)

	// Run linker
	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Verify the test is linked to "widget" (function name), not "gadget" (inferred)
	// Function name matching has highest priority
	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 1 {
		t.Errorf("expected 1 widget test (function name priority), got %d", len(widgetTests))
	}

	gadgetTests := reg.GetResourceTests("gadget")
	if len(gadgetTests) != 0 {
		t.Errorf("expected 0 gadget tests (function name should take priority over inferred), got %d", len(gadgetTests))
	}

	// Verify match type is function_name since that has highest priority
	if len(widgetTests) > 0 && widgetTests[0].MatchType != registry.MatchTypeFunctionName {
		t.Errorf("expected MatchTypeFunctionName, got %v", widgetTests[0].MatchType)
	}
}

func TestLinkerPriorityMatching(t *testing.T) {
	// Test that function name matching takes priority over file proximity
	reg := registry.NewResourceRegistry()
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget"})
	reg.RegisterResource(&registry.ResourceInfo{Name: "gadget"})

	// Test function with name matching "widget" but in file matching "gadget"
	fn := &registry.TestFunctionInfo{
		Name:     "TestAccWidget_basic",
		FilePath: "/path/to/resource_gadget_test.go",
	}
	reg.RegisterTestFunction(fn)

	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Should match widget (function name) not gadget (file proximity)
	widgetTests := reg.GetResourceTests("widget")
	if len(widgetTests) != 1 {
		t.Errorf("expected 1 widget test (function name priority), got %d", len(widgetTests))
	}

	gadgetTests := reg.GetResourceTests("gadget")
	if len(gadgetTests) != 0 {
		t.Errorf("expected 0 gadget tests (function name should take priority), got %d", len(gadgetTests))
	}
}

func TestMatchByFileProximity(t *testing.T) {
	reg := registry.NewResourceRegistry()
	linker := matching.NewLinker(reg, config.DefaultSettings())

	// Test now returns compound keys (kind:name) to properly distinguish
	// resources from data sources when they have the same name
	tests := []struct {
		filePath      string
		resourceNames map[string]bool
		expected      string
	}{
		{
			filePath:      "/path/to/resource_widget_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "resource:widget",
		},
		{
			filePath:      "/path/to/data_source_http_test.go",
			resourceNames: map[string]bool{"http": true},
			expected:      "data source:http",
		},
		{
			filePath:      "/path/to/widget_resource_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "resource:widget",
		},
		{
			filePath:      "/path/to/widget_data_source_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "data source:widget",
		},
		{
			// Fallback: plain name without suffix returns simple name
			filePath:      "/path/to/widget_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "widget",
		},
		{
			filePath:      "/path/to/unrelated_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "",
		},
		{
			filePath:      "/path/to/not_a_test.go",
			resourceNames: map[string]bool{"widget": true},
			expected:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.filePath, func(t *testing.T) {
			got := linker.MatchByFileProximity(tt.filePath, tt.resourceNames)
			if got != tt.expected {
				t.Errorf("MatchByFileProximity(%q) = %q, want %q", tt.filePath, got, tt.expected)
			}
		})
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct {
		nums     []int
		expected int
	}{
		{[]int{1, 2, 3}, 1},
		{[]int{3, 2, 1}, 1},
		{[]int{2, 1, 3}, 1},
		{[]int{5}, 5},
		{[]int{-1, 0, 1}, -1},
		{[]int{100, 50, 75}, 50},
	}

	for _, tt := range tests {
		got := matching.MinInt(tt.nums...)
		if got != tt.expected {
			t.Errorf("MinInt(%v) = %d, want %d", tt.nums, got, tt.expected)
		}
	}
}

func TestMinIntEmpty(t *testing.T) {
	got := matching.MinInt()
	if got != 0 {
		t.Errorf("MinInt() = %d, want 0", got)
	}
}

func TestMatchTypeString(t *testing.T) {
	tests := []struct {
		matchType registry.MatchType
		expected  string
	}{
		{registry.MatchTypeNone, "none"},
		{registry.MatchTypeInferred, "inferred_from_config"},
		{registry.MatchTypeFunctionName, "function_name"},
		{registry.MatchTypeFileProximity, "file_proximity"},
		{registry.MatchTypeFuzzy, "fuzzy"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.matchType.String()
			if got != tt.expected {
				t.Errorf("MatchType(%d).String() = %q, want %q", tt.matchType, got, tt.expected)
			}
		})
	}
}

func TestMatchResourceByName(t *testing.T) {
	tests := []struct {
		name          string
		funcName      string
		resourceNames map[string]bool
		expectedMatch string
		expectedFound bool
	}{
		{
			name:          "basic TestAcc pattern",
			funcName:      "TestAccWidget_basic",
			resourceNames: map[string]bool{"widget": true},
			expectedMatch: "widget",
			expectedFound: true,
		},
		{
			name:          "TestAccResource pattern",
			funcName:      "TestAccResourceWidget_update",
			resourceNames: map[string]bool{"widget": true},
			expectedMatch: "widget",
			expectedFound: true,
		},
		{
			name:          "TestAccDataSource pattern",
			funcName:      "TestAccDataSourceHTTP_basic",
			resourceNames: map[string]bool{"http": true},
			expectedMatch: "http",
			expectedFound: true,
		},
		{
			name:          "strips provider prefix",
			funcName:      "TestAccAWSInstance_basic",
			resourceNames: map[string]bool{"instance": true},
			expectedMatch: "instance",
			expectedFound: true,
		},
		{
			name:          "matches with generated suffix",
			funcName:      "TestAccBucket_generated",
			resourceNames: map[string]bool{"bucket": true},
			expectedMatch: "bucket",
			expectedFound: true,
		},
		{
			name:          "matches with complete suffix",
			funcName:      "TestAccServer_complete",
			resourceNames: map[string]bool{"server": true},
			expectedMatch: "server",
			expectedFound: true,
		},
		{
			name:          "matches camelCase to snake_case",
			funcName:      "TestAccAwsS3Bucket_basic",
			resourceNames: map[string]bool{"aws_s3_bucket": true},
			expectedMatch: "aws_s3_bucket",
			expectedFound: true,
		},
		{
			name:          "matches without provider prefix",
			funcName:      "TestAccAwsS3Bucket_basic",
			resourceNames: map[string]bool{"s3_bucket": true},
			expectedMatch: "s3_bucket",
			expectedFound: true,
		},
		{
			name:          "no match returns empty",
			funcName:      "TestAccWidget_basic",
			resourceNames: map[string]bool{"gadget": true},
			expectedMatch: "",
			expectedFound: false,
		},
		{
			name:          "TestResource pattern",
			funcName:      "TestResourceWidget_basic",
			resourceNames: map[string]bool{"widget": true},
			expectedMatch: "widget",
			expectedFound: true,
		},
		{
			name:          "TestDataSource pattern",
			funcName:      "TestDataSourceHTTP_complete",
			resourceNames: map[string]bool{"http": true},
			expectedMatch: "http",
			expectedFound: true,
		},
		{
			name:          "handles disappears suffix",
			funcName:      "TestAccInstance_disappears",
			resourceNames: map[string]bool{"instance": true},
			expectedMatch: "instance",
			expectedFound: true,
		},
		{
			name:          "handles import suffix",
			funcName:      "TestAccDatabase_import",
			resourceNames: map[string]bool{"database": true},
			expectedMatch: "database",
			expectedFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, found := matching.MatchResourceByName(tt.funcName, tt.resourceNames)
			if match != tt.expectedMatch {
				t.Errorf("MatchResourceByName(%q) match = %q, want %q",
					tt.funcName, match, tt.expectedMatch)
			}
			if found != tt.expectedFound {
				t.Errorf("MatchResourceByName(%q) found = %v, want %v",
					tt.funcName, found, tt.expectedFound)
			}
		})
	}
}

// TestLinkerClassifyConsistency verifies that if the Linker matches a test function
// to a resource, ClassifyTestFunctionMatch also returns "matched".
// This ensures the two systems use the same matching logic.
func TestLinkerClassifyConsistency(t *testing.T) {
	tests := []struct {
		name         string
		funcName     string
		resourceName string
	}{
		{"basic TestAcc pattern", "TestAccWidget_basic", "widget"},
		{"TestAccResource pattern", "TestAccResourceWidget_update", "widget"},
		{"TestAccDataSource pattern", "TestAccDataSourceHTTP_basic", "http"},
		{"strips provider prefix", "TestAccAWSInstance_basic", "instance"},
		{"with generated suffix", "TestAccBucket_generated", "bucket"},
		{"with complete suffix", "TestAccServer_complete", "server"},
		{"camelCase to snake_case", "TestAccAwsS3Bucket_basic", "aws_s3_bucket"},
		{"without provider prefix", "TestAccAwsS3Bucket_basic", "s3_bucket"},
		{"TestResource pattern", "TestResourceWidget_basic", "widget"},
		{"TestDataSource pattern", "TestDataSourceHTTP_complete", "http"},
		{"disappears suffix", "TestAccInstance_disappears", "instance"},
		{"import suffix", "TestAccDatabase_import", "database"},
		{"migrate suffix", "TestAccServer_migrate", "server"},
		{"full suffix", "TestAccWidget_full", "widget"},
		{"minimal suffix", "TestAccGadget_minimal", "gadget"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test with Linker's MatchResourceByName
			resourceNames := map[string]bool{tt.resourceName: true}
			matched, found := matching.MatchResourceByName(tt.funcName, resourceNames)

			if !found || matched != tt.resourceName {
				t.Fatalf("MatchResourceByName(%q, %q) failed: matched=%q, found=%v",
					tt.funcName, tt.resourceName, matched, found)
			}

			// Test with ClassifyTestFunctionMatch - should also return "matched"
			status, reason := analysis.ClassifyTestFunctionMatch(tt.funcName, tt.resourceName)
			if status != "matched" {
				t.Errorf("CONSISTENCY ERROR: Linker matched %q to %q, but ClassifyTestFunctionMatch returned status=%q, reason=%q",
					tt.funcName, tt.resourceName, status, reason)
				t.Errorf("This breaks the consistency requirement: linked tests should NEVER show 'pattern mismatch' in diagnostics")
			}
		})
	}
}

// TestClassifyNonMatchingFunctions verifies that ClassifyTestFunctionMatch
// correctly identifies non-matching functions with appropriate reasons.
func TestClassifyNonMatchingFunctions(t *testing.T) {
	tests := []struct {
		name            string
		funcName        string
		resourceName    string
		expectedStatus  string
		reasonSubstring string // Part of the reason to verify
	}{
		{
			name:            "wrong resource name",
			funcName:        "TestAccWidget_basic",
			resourceName:    "gadget",
			expectedStatus:  "not_matched",
			reasonSubstring: "does not match resource",
		},
		{
			name:            "completely different name",
			funcName:        "TestAccFooBar_basic",
			resourceName:    "widget",
			expectedStatus:  "not_matched",
			reasonSubstring: "does not match resource",
		},
		{
			name:            "not a test function",
			funcName:        "HelperFunction",
			resourceName:    "widget",
			expectedStatus:  "not_matched",
			reasonSubstring: "does not follow test naming convention",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, reason := analysis.ClassifyTestFunctionMatch(tt.funcName, tt.resourceName)
			if status != tt.expectedStatus {
				t.Errorf("ClassifyTestFunctionMatch(%q, %q) status = %q, want %q",
					tt.funcName, tt.resourceName, status, tt.expectedStatus)
			}
			if tt.reasonSubstring != "" && !strings.Contains(reason, tt.reasonSubstring) {
				t.Errorf("ClassifyTestFunctionMatch(%q, %q) reason = %q, want to contain %q",
					tt.funcName, tt.resourceName, reason, tt.reasonSubstring)
			}
		})
	}
}

// TestLinkerDataSourcePreference tests that functions with "DataSource" in the name
// are preferentially matched to data sources over resources when both exist with the same name.
// This fixes the issue where TestAccInventoryDataSource was matched to inventory resource
// instead of inventory data source.
func TestLinkerDataSourcePreference(t *testing.T) {
	reg := registry.NewResourceRegistry()

	// Register BOTH a resource and a data source with the same base name
	reg.RegisterResource(&registry.ResourceInfo{Name: "inventory", Kind: registry.KindResource})
	reg.RegisterResource(&registry.ResourceInfo{Name: "inventory", Kind: registry.KindDataSource})

	// Register a test function that has "DataSource" in the name
	fn := &registry.TestFunctionInfo{
		Name:     "TestAccInventoryDataSource",
		FilePath: "/inventory_data_source_test.go",
	}
	reg.RegisterTestFunction(fn)

	// Run linker
	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// The test should be linked to the DATA SOURCE, not the resource
	// Use compound key to check specifically the data source
	dataSourceTests := reg.GetResourceTests("data source:inventory")
	resourceTests := reg.GetResourceTests("resource:inventory")

	if len(dataSourceTests) != 1 {
		t.Errorf("expected 1 data source test, got %d", len(dataSourceTests))
	}

	if len(resourceTests) != 0 {
		t.Errorf("expected 0 resource tests (should be matched to data source), got %d", len(resourceTests))
	}

	// Verify match type
	if len(dataSourceTests) > 0 && dataSourceTests[0].MatchType != registry.MatchTypeFunctionName {
		t.Errorf("expected MatchTypeFunctionName for data source preference, got %v", dataSourceTests[0].MatchType)
	}
}

// TestLinkerResourceSuffixStripping tests that "Resource" and "DataSource" suffixes
// are stripped from function names to produce the correct resource name match.
// This fixes the issue where TestAccGroupResource produced "group_resource" instead of "group".
func TestLinkerResourceSuffixStripping(t *testing.T) {
	reg := registry.NewResourceRegistry()

	// Register resources
	reg.RegisterResource(&registry.ResourceInfo{Name: "group", Kind: registry.KindResource})
	reg.RegisterResource(&registry.ResourceInfo{Name: "host", Kind: registry.KindResource})

	// Register test functions with "Resource" suffix in name
	fn1 := &registry.TestFunctionInfo{Name: "TestAccGroupResource", FilePath: "/group_resource_test.go"}
	fn2 := &registry.TestFunctionInfo{Name: "TestAccHostResource", FilePath: "/host_resource_test.go"}
	reg.RegisterTestFunction(fn1)
	reg.RegisterTestFunction(fn2)

	// Run linker
	settings := config.DefaultSettings()
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	// Verify "TestAccGroupResource" matches "group" (not "group_resource")
	groupTests := reg.GetResourceTests("group")
	if len(groupTests) != 1 {
		t.Errorf("expected 1 group test, got %d (TestAccGroupResource should match 'group')", len(groupTests))
	}

	// Verify "TestAccHostResource" matches "host"
	hostTests := reg.GetResourceTests("host")
	if len(hostTests) != 1 {
		t.Errorf("expected 1 host test, got %d (TestAccHostResource should match 'host')", len(hostTests))
	}
}
