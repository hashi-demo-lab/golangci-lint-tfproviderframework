// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestFindLocalTestHelpers(t *testing.T) {
	tests := []struct {
		name            string
		src             string
		expectedHelpers []string
	}{
		{
			name: "discovers helper that calls resource.Test",
			src: `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// AccTestHelper is a custom test wrapper
func AccTestHelper(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}

// TestAccWidget_basic is an actual test
func TestAccWidget_basic(t *testing.T) {
	AccTestHelper(t, resource.TestCase{})
}
`,
			expectedHelpers: []string{"AccTestHelper"},
		},
		{
			name: "discovers helper that calls resource.ParallelTest",
			src: `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// ParallelHelper wraps ParallelTest
func ParallelHelper(t *testing.T, tc resource.TestCase) {
	resource.ParallelTest(t, tc)
}
`,
			expectedHelpers: []string{"ParallelHelper"},
		},
		{
			name: "ignores unexported functions",
			src: `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// helper is unexported
func helper(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}
`,
			expectedHelpers: []string{},
		},
		{
			name: "ignores functions that do not call resource.Test",
			src: `
package provider_test

import (
	"testing"
)

// SomeHelper does not call resource.Test
func SomeHelper(t *testing.T) {
	t.Log("hello")
}
`,
			expectedHelpers: []string{},
		},
		{
			name: "ignores functions without *testing.T parameter",
			src: `
package provider_test

import (
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// NoTestingT does not accept *testing.T
func NoTestingT(tc resource.TestCase) {
	// This function can't actually call resource.Test without *testing.T
	_ = tc
}
`,
			expectedHelpers: []string{},
		},
		{
			name: "ignores Test prefixed functions",
			src: `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccWidget is a test, not a helper
func TestAccWidget(t *testing.T) {
	resource.Test(t, resource.TestCase{})
}
`,
			expectedHelpers: []string{},
		},
		{
			name: "discovers multiple helpers",
			src: `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Helper1 is first helper
func Helper1(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}

// Helper2 is second helper
func Helper2(t *testing.T, tc resource.TestCase) {
	resource.ParallelTest(t, tc)
}
`,
			expectedHelpers: []string{"Helper1", "Helper2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test_test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse source: %v", err)
			}

			helpers := FindLocalTestHelpers([]*ast.File{file}, fset)

			if len(helpers) != len(tt.expectedHelpers) {
				t.Errorf("expected %d helpers, got %d", len(tt.expectedHelpers), len(helpers))
				for _, h := range helpers {
					t.Logf("  found: %s", h.Name)
				}
				return
			}

			for i, expected := range tt.expectedHelpers {
				if helpers[i].Name != expected {
					t.Errorf("helper[%d]: expected %q, got %q", i, expected, helpers[i].Name)
				}
			}
		})
	}
}

func TestAcceptsTestingT(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		expected bool
	}{
		{
			name: "function with *testing.T parameter",
			src: `
package test
import "testing"
func MyFunc(t *testing.T) {}
`,
			expected: true,
		},
		{
			name: "function with *testing.T as second parameter",
			src: `
package test
import "testing"
func MyFunc(name string, t *testing.T) {}
`,
			expected: true,
		},
		{
			name: "function with named *testing.T parameter",
			src: `
package test
import "testing"
func MyFunc(testing *testing.T) {}
`,
			expected: true,
		},
		{
			name: "function without *testing.T parameter",
			src: `
package test
func MyFunc(name string) {}
`,
			expected: false,
		},
		{
			name: "function with no parameters",
			src: `
package test
func MyFunc() {}
`,
			expected: false,
		},
		{
			name: "function with testing.T (not pointer)",
			src: `
package test
import "testing"
func MyFunc(t testing.T) {}
`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse source: %v", err)
			}

			// Find the function declaration
			var funcDecl *ast.FuncDecl
			ast.Inspect(file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok {
					funcDecl = fd
					return false
				}
				return true
			})

			if funcDecl == nil {
				t.Fatal("could not find function declaration")
			}

			result := AcceptsTestingT(funcDecl)
			if result != tt.expected {
				t.Errorf("AcceptsTestingT() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMatchesExcludePattern(t *testing.T) {
	tests := []struct {
		name           string
		filePath       string
		patterns       []string
		expectExcluded bool
		expectPattern  string
	}{
		{
			name:           "matches sweeper file pattern",
			filePath:       "/path/to/resource_widget_sweeper.go",
			patterns:       []string{"*_sweeper.go", "*_test_helpers.go"},
			expectExcluded: true,
			expectPattern:  "*_sweeper.go",
		},
		{
			name:           "matches test helpers pattern",
			filePath:       "/path/to/acc_test_helpers.go",
			patterns:       []string{"*_sweeper.go", "*_test_helpers.go"},
			expectExcluded: true,
			expectPattern:  "*_test_helpers.go",
		},
		{
			name:           "does not match regular resource file",
			filePath:       "/path/to/resource_widget.go",
			patterns:       []string{"*_sweeper.go", "*_test_helpers.go"},
			expectExcluded: false,
			expectPattern:  "",
		},
		{
			name:           "does not match regular test file",
			filePath:       "/path/to/resource_widget_test.go",
			patterns:       []string{"*_sweeper.go", "*_test_helpers.go"},
			expectExcluded: false,
			expectPattern:  "",
		},
		{
			name:           "empty patterns matches nothing",
			filePath:       "/path/to/any_file.go",
			patterns:       []string{},
			expectExcluded: false,
			expectPattern:  "",
		},
		{
			name:           "matches pattern with prefix",
			filePath:       "/path/to/test_helpers_common.go",
			patterns:       []string{"test_helpers_*.go"},
			expectExcluded: true,
			expectPattern:  "test_helpers_*.go",
		},
		{
			name:           "matches pattern with single character wildcard",
			filePath:       "/path/to/resource_a_sweeper.go",
			patterns:       []string{"resource_?_sweeper.go"},
			expectExcluded: true,
			expectPattern:  "resource_?_sweeper.go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchesExcludePattern(tt.filePath, tt.patterns)

			if result.Excluded != tt.expectExcluded {
				t.Errorf("MatchesExcludePattern(%q).Excluded = %v, want %v",
					tt.filePath, result.Excluded, tt.expectExcluded)
			}

			if tt.expectExcluded && result.MatchedPattern != tt.expectPattern {
				t.Errorf("MatchesExcludePattern(%q).MatchedPattern = %q, want %q",
					tt.filePath, result.MatchedPattern, tt.expectPattern)
			}

			if result.FilePath != tt.filePath {
				t.Errorf("MatchesExcludePattern(%q).FilePath = %q, want %q",
					tt.filePath, result.FilePath, tt.filePath)
			}
		})
	}
}

func TestCheckUsesResourceTestWithLocalHelpers(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		customHelpers []string
		localHelpers  []LocalHelper
		expected      bool
	}{
		{
			name: "detects resource.Test call",
			src: `
package test
func TestFunc() {
	resource.Test(t, tc)
}
`,
			customHelpers: nil,
			localHelpers:  nil,
			expected:      true,
		},
		{
			name: "detects resource.ParallelTest call",
			src: `
package test
func TestFunc() {
	resource.ParallelTest(t, tc)
}
`,
			customHelpers: nil,
			localHelpers:  nil,
			expected:      true,
		},
		{
			name: "detects custom helper call",
			src: `
package test
func TestFunc() {
	testhelper.AccTest(t, tc)
}
`,
			customHelpers: []string{"testhelper.AccTest"},
			localHelpers:  nil,
			expected:      true,
		},
		{
			name: "detects local helper call",
			src: `
package test
func TestFunc() {
	AccTestHelper(t, tc)
}
`,
			customHelpers: nil,
			localHelpers:  []LocalHelper{{Name: "AccTestHelper"}},
			expected:      true,
		},
		{
			name: "returns false when no helper is used",
			src: `
package test
func TestFunc() {
	t.Log("hello")
}
`,
			customHelpers: nil,
			localHelpers:  nil,
			expected:      false,
		},
		{
			name: "returns false for unknown function call",
			src: `
package test
func TestFunc() {
	UnknownHelper(t, tc)
}
`,
			customHelpers: nil,
			localHelpers:  nil,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse source: %v", err)
			}

			// Find the function body
			var body *ast.BlockStmt
			ast.Inspect(file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
					body = fd.Body
					return false
				}
				return true
			})

			if body == nil {
				t.Fatal("could not find function body")
			}

			result := CheckUsesResourceTestWithLocalHelpers(body, tt.customHelpers, tt.localHelpers)
			if result != tt.expected {
				t.Errorf("CheckUsesResourceTestWithLocalHelpers() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDetectHelperUsed(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		localHelpers []LocalHelper
		expected     string
	}{
		{
			name: "detects resource.Test",
			src: `
package test
func TestFunc() {
	resource.Test(t, tc)
}
`,
			localHelpers: nil,
			expected:     "resource.Test",
		},
		{
			name: "detects resource.ParallelTest",
			src: `
package test
func TestFunc() {
	resource.ParallelTest(t, tc)
}
`,
			localHelpers: nil,
			expected:     "resource.ParallelTest",
		},
		{
			name: "detects local helper",
			src: `
package test
func TestFunc() {
	AccTestHelper(t, tc)
}
`,
			localHelpers: []LocalHelper{{Name: "AccTestHelper"}},
			expected:     "AccTestHelper",
		},
		{
			name: "returns empty for unknown calls",
			src: `
package test
func TestFunc() {
	UnknownFunc()
}
`,
			localHelpers: nil,
			expected:     "",
		},
		{
			name: "returns empty for no calls",
			src: `
package test
func TestFunc() {
	x := 1
	_ = x
}
`,
			localHelpers: nil,
			expected:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("failed to parse source: %v", err)
			}

			// Find the function body
			var body *ast.BlockStmt
			ast.Inspect(file, func(n ast.Node) bool {
				if fd, ok := n.(*ast.FuncDecl); ok && fd.Body != nil {
					body = fd.Body
					return false
				}
				return true
			})

			if body == nil {
				t.Fatal("could not find function body")
			}

			result := DetectHelperUsed(body, tt.localHelpers)
			if result != tt.expected {
				t.Errorf("DetectHelperUsed() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAcceptsTestingT_NilCases(t *testing.T) {
	// Test nil function declaration
	if AcceptsTestingT(nil) {
		t.Error("AcceptsTestingT(nil) should return false")
	}

	// Test function declaration with nil Type
	funcDecl := &ast.FuncDecl{Name: ast.NewIdent("test")}
	if AcceptsTestingT(funcDecl) {
		t.Error("AcceptsTestingT with nil Type should return false")
	}

	// Test function declaration with nil Params
	funcDecl = &ast.FuncDecl{
		Name: ast.NewIdent("test"),
		Type: &ast.FuncType{},
	}
	if AcceptsTestingT(funcDecl) {
		t.Error("AcceptsTestingT with nil Params should return false")
	}
}

func TestLocalHelper_Fields(t *testing.T) {
	// Test that LocalHelper can be created with all fields
	src := `
package test
import "testing"
import "github.com/hashicorp/terraform-plugin-testing/helper/resource"
func Helper(t *testing.T) { resource.Test(t, resource.TestCase{}) }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "helper_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	helpers := FindLocalTestHelpers([]*ast.File{file}, fset)
	if len(helpers) != 1 {
		t.Fatalf("expected 1 helper, got %d", len(helpers))
	}

	helper := helpers[0]
	if helper.Name != "Helper" {
		t.Errorf("Name = %q, want %q", helper.Name, "Helper")
	}
	if helper.FilePath != "helper_test.go" {
		t.Errorf("FilePath = %q, want %q", helper.FilePath, "helper_test.go")
	}
	if helper.FuncDecl == nil {
		t.Error("FuncDecl should not be nil")
	}
}

func TestExclusionResult_Fields(t *testing.T) {
	// Test excluded file
	result := MatchesExcludePattern("/path/to/sweeper.go", []string{"*sweeper.go"})
	if !result.Excluded {
		t.Error("should be excluded")
	}
	if result.FilePath != "/path/to/sweeper.go" {
		t.Errorf("FilePath = %q, want %q", result.FilePath, "/path/to/sweeper.go")
	}
	if result.Reason == "" {
		t.Error("Reason should not be empty for excluded file")
	}
	if result.MatchedPattern != "*sweeper.go" {
		t.Errorf("MatchedPattern = %q, want %q", result.MatchedPattern, "*sweeper.go")
	}

	// Test non-excluded file
	result = MatchesExcludePattern("/path/to/resource.go", []string{"*sweeper.go"})
	if result.Excluded {
		t.Error("should not be excluded")
	}
	if result.MatchedPattern != "" {
		t.Errorf("MatchedPattern = %q, want empty", result.MatchedPattern)
	}
}

func TestExclusionDiagnostics_Fields(t *testing.T) {
	// Just verify the struct can be used
	diag := ExclusionDiagnostics{
		ExcludedFiles: []ExclusionResult{
			{FilePath: "a.go", Excluded: true, Reason: "test", MatchedPattern: "*"},
			{FilePath: "b.go", Excluded: true, Reason: "test", MatchedPattern: "*"},
		},
		TotalExcluded: 2,
	}

	if len(diag.ExcludedFiles) != 2 {
		t.Errorf("ExcludedFiles length = %d, want 2", len(diag.ExcludedFiles))
	}
	if diag.TotalExcluded != 2 {
		t.Errorf("TotalExcluded = %d, want 2", diag.TotalExcluded)
	}
}

func TestParserConfig_DefaultConfig(t *testing.T) {
	config := DefaultParserConfig()

	if config.CustomHelpers != nil {
		t.Errorf("DefaultParserConfig().CustomHelpers = %v, want nil", config.CustomHelpers)
	}
	if config.LocalHelpers != nil {
		t.Errorf("DefaultParserConfig().LocalHelpers = %v, want nil", config.LocalHelpers)
	}
	if config.TestNamePatterns != nil {
		t.Errorf("DefaultParserConfig().TestNamePatterns = %v, want nil", config.TestNamePatterns)
	}
}

func TestParseTestFileWithConfig_EmptyConfig(t *testing.T) {
	src := `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWidget_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccWidgetConfig_basic(),
			},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resource_widget_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	config := DefaultParserConfig()
	testFileInfo := ParseTestFileWithConfig(file, fset, "resource_widget_test.go", config)

	if testFileInfo == nil {
		t.Fatal("ParseTestFileWithConfig returned nil")
	}

	if len(testFileInfo.TestFunctions) != 1 {
		t.Errorf("expected 1 test function, got %d", len(testFileInfo.TestFunctions))
	}

	if testFileInfo.TestFunctions[0].Name != "TestAccWidget_basic" {
		t.Errorf("test function name = %q, want %q", testFileInfo.TestFunctions[0].Name, "TestAccWidget_basic")
	}
}

func TestParseTestFileWithConfig_CustomHelpers(t *testing.T) {
	src := `
package provider_test

import (
	"testing"
	"github.com/example/testhelper"
)

func TestAccWidget_basic(t *testing.T) {
	testhelper.AccTest(t, testhelper.TestCase{
		Steps: []testhelper.TestStep{
			{
				Config: testAccWidgetConfig_basic(),
			},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resource_widget_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	config := ParserConfig{
		CustomHelpers: []string{"testhelper.AccTest"},
	}
	testFileInfo := ParseTestFileWithConfig(file, fset, "resource_widget_test.go", config)

	if testFileInfo == nil {
		t.Fatal("ParseTestFileWithConfig returned nil")
	}

	if len(testFileInfo.TestFunctions) != 1 {
		t.Errorf("expected 1 test function, got %d", len(testFileInfo.TestFunctions))
	}

	if testFileInfo.TestFunctions[0].Name != "TestAccWidget_basic" {
		t.Errorf("test function name = %q, want %q", testFileInfo.TestFunctions[0].Name, "TestAccWidget_basic")
	}
}

func TestParseTestFileWithConfig_LocalHelpers(t *testing.T) {
	// First, parse a file with a local helper
	helperSrc := `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func AccTestHelper(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}
`
	fset := token.NewFileSet()
	helperFile, err := parser.ParseFile(fset, "helpers_test.go", helperSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse helper source: %v", err)
	}

	localHelpers := FindLocalTestHelpers([]*ast.File{helperFile}, fset)
	if len(localHelpers) != 1 {
		t.Fatalf("expected 1 local helper, got %d", len(localHelpers))
	}

	// Now parse a test file that uses the local helper
	testSrc := `
package provider_test

import "testing"

func TestAccWidget_basic(t *testing.T) {
	AccTestHelper(t, resource.TestCase{})
}
`
	testFile, err := parser.ParseFile(fset, "resource_widget_test.go", testSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse test source: %v", err)
	}

	config := ParserConfig{
		LocalHelpers: localHelpers,
	}
	testFileInfo := ParseTestFileWithConfig(testFile, fset, "resource_widget_test.go", config)

	if testFileInfo == nil {
		t.Fatal("ParseTestFileWithConfig returned nil")
	}

	if len(testFileInfo.TestFunctions) != 1 {
		t.Errorf("expected 1 test function, got %d", len(testFileInfo.TestFunctions))
	}

	if testFileInfo.TestFunctions[0].Name != "TestAccWidget_basic" {
		t.Errorf("test function name = %q, want %q", testFileInfo.TestFunctions[0].Name, "TestAccWidget_basic")
	}

	if testFileInfo.TestFunctions[0].HelperUsed != "AccTestHelper" {
		t.Errorf("helper used = %q, want %q", testFileInfo.TestFunctions[0].HelperUsed, "AccTestHelper")
	}
}

func TestParseTestFileWithConfig_TestNamePatterns(t *testing.T) {
	src := `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestIntegration_Widget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccWidgetConfig_basic(),
			},
		},
	})
}

func TestAcc_Widget(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccWidgetConfig_basic(),
			},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resource_widget_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	// Test with custom pattern that only matches TestIntegration*
	config := ParserConfig{
		TestNamePatterns: []string{"TestIntegration*"},
	}
	testFileInfo := ParseTestFileWithConfig(file, fset, "resource_widget_test.go", config)

	if testFileInfo == nil {
		t.Fatal("ParseTestFileWithConfig returned nil")
	}

	if len(testFileInfo.TestFunctions) != 1 {
		t.Errorf("expected 1 test function, got %d", len(testFileInfo.TestFunctions))
	}

	if testFileInfo.TestFunctions[0].Name != "TestIntegration_Widget" {
		t.Errorf("test function name = %q, want %q", testFileInfo.TestFunctions[0].Name, "TestIntegration_Widget")
	}
}

func TestParseTestFileWithConfig_AllOptions(t *testing.T) {
	// First, create a local helper
	helperSrc := `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func MyTestHelper(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}
`
	fset := token.NewFileSet()
	helperFile, err := parser.ParseFile(fset, "helpers_test.go", helperSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse helper source: %v", err)
	}

	localHelpers := FindLocalTestHelpers([]*ast.File{helperFile}, fset)

	// Test file with custom test name pattern
	testSrc := `
package provider_test

import (
	"testing"
	"github.com/example/testhelper"
)

func TestCustom_Widget_basic(t *testing.T) {
	MyTestHelper(t, resource.TestCase{})
}

func TestCustom_Widget_update(t *testing.T) {
	testhelper.ParallelTest(t, testhelper.TestCase{})
}

func TestIgnored_Widget(t *testing.T) {
	MyTestHelper(t, resource.TestCase{})
}
`
	testFile, err := parser.ParseFile(fset, "resource_widget_test.go", testSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse test source: %v", err)
	}

	// Config with all options set
	config := ParserConfig{
		CustomHelpers:    []string{"testhelper.ParallelTest"},
		LocalHelpers:     localHelpers,
		TestNamePatterns: []string{"TestCustom*"},
	}
	testFileInfo := ParseTestFileWithConfig(testFile, fset, "resource_widget_test.go", config)

	if testFileInfo == nil {
		t.Fatal("ParseTestFileWithConfig returned nil")
	}

	// Should only find TestCustom* tests that use either custom or local helpers
	if len(testFileInfo.TestFunctions) != 2 {
		t.Errorf("expected 2 test functions, got %d", len(testFileInfo.TestFunctions))
		for _, tf := range testFileInfo.TestFunctions {
			t.Logf("  found: %s", tf.Name)
		}
	}

	// Verify both TestCustom* tests were found
	foundBasic := false
	foundUpdate := false
	for _, tf := range testFileInfo.TestFunctions {
		if tf.Name == "TestCustom_Widget_basic" {
			foundBasic = true
			if tf.HelperUsed != "MyTestHelper" {
				t.Errorf("TestCustom_Widget_basic helper = %q, want %q", tf.HelperUsed, "MyTestHelper")
			}
		}
		if tf.Name == "TestCustom_Widget_update" {
			foundUpdate = true
		}
	}

	if !foundBasic {
		t.Error("TestCustom_Widget_basic not found")
	}
	if !foundUpdate {
		t.Error("TestCustom_Widget_update not found")
	}
}

func TestParseTestFileWithConfig_BackwardCompatibility(t *testing.T) {
	src := `
package provider_test

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccWidget_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: testAccWidgetConfig_basic(),
			},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resource_widget_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	// Test that old functions still work
	oldResult := ParseTestFile(file, fset, "resource_widget_test.go")
	newResult := ParseTestFileWithConfig(file, fset, "resource_widget_test.go", DefaultParserConfig())

	if oldResult == nil || newResult == nil {
		t.Fatal("one of the parse functions returned nil")
	}

	if len(oldResult.TestFunctions) != len(newResult.TestFunctions) {
		t.Errorf("old and new results differ: old=%d, new=%d", len(oldResult.TestFunctions), len(newResult.TestFunctions))
	}

	if oldResult.TestFunctions[0].Name != newResult.TestFunctions[0].Name {
		t.Errorf("function names differ: old=%q, new=%q", oldResult.TestFunctions[0].Name, newResult.TestFunctions[0].Name)
	}
}

func TestParseTestFileWithConfig_WithHelpersBackwardCompatibility(t *testing.T) {
	src := `
package provider_test

import (
	"testing"
	"github.com/example/testhelper"
)

func TestAccWidget_basic(t *testing.T) {
	testhelper.AccTest(t, testhelper.TestCase{})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "resource_widget_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}

	customHelpers := []string{"testhelper.AccTest"}

	// Test that old function still works
	oldResult := ParseTestFileWithHelpers(file, fset, "resource_widget_test.go", customHelpers)
	newResult := ParseTestFileWithConfig(file, fset, "resource_widget_test.go", ParserConfig{
		CustomHelpers: customHelpers,
	})

	if oldResult == nil || newResult == nil {
		t.Fatal("one of the parse functions returned nil")
	}

	if len(oldResult.TestFunctions) != len(newResult.TestFunctions) {
		t.Errorf("old and new results differ: old=%d, new=%d", len(oldResult.TestFunctions), len(newResult.TestFunctions))
	}

	if oldResult.TestFunctions[0].Name != newResult.TestFunctions[0].Name {
		t.Errorf("function names differ: old=%q, new=%q", oldResult.TestFunctions[0].Name, newResult.TestFunctions[0].Name)
	}
}

func TestResourceTypeRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single resource block",
			input:    `resource "example_widget" "test" {`,
			expected: []string{"example_widget"},
		},
		{
			name:     "multiple resource blocks",
			input:    `resource "example_widget" "test" { } resource "example_gadget" "other" {`,
			expected: []string{"example_widget", "example_gadget"},
		},
		{
			name: "resource block with newlines",
			input: `
resource "aws_instance" "example" {
  ami = "ami-12345"
}
`,
			expected: []string{"aws_instance"},
		},
		{
			name:     "resource with underscores",
			input:    `resource "aws_s3_bucket" "my_bucket" {`,
			expected: []string{"aws_s3_bucket"},
		},
		{
			name:     "resource with extra whitespace",
			input:    `resource   "example_widget"   "test"   {`,
			expected: []string{"example_widget"},
		},
		{
			name:     "no resource block - data source only",
			input:    `data "example_widget" "test" {`,
			expected: []string{},
		},
		{
			name:     "no resource block - empty string",
			input:    ``,
			expected: []string{},
		},
		{
			name:     "no resource block - plain text",
			input:    `some random text without resource blocks`,
			expected: []string{},
		},
		{
			name:     "invalid pattern - missing quotes",
			input:    `resource example_widget "test" {`,
			expected: []string{},
		},
		{
			name:     "invalid pattern - missing opening brace",
			input:    `resource "example_widget" "test"`,
			expected: []string{},
		},
		{
			name: "multiple resources in HCL config",
			input: `
provider "example" {}

resource "example_widget" "first" {
  name = "widget1"
}

resource "example_gadget" "second" {
  name = "gadget1"
}

resource "example_thing" "third" {
  depends_on = [example_widget.first]
}
`,
			expected: []string{"example_widget", "example_gadget", "example_thing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := resourceTypeRegex.FindAllStringSubmatch(tt.input, -1)
			var extracted []string
			for _, match := range matches {
				if len(match) > 1 {
					extracted = append(extracted, match[1])
				}
			}

			if len(extracted) != len(tt.expected) {
				t.Errorf("expected %d matches, got %d", len(tt.expected), len(extracted))
				t.Errorf("extracted: %v, expected: %v", extracted, tt.expected)
				return
			}

			for i, exp := range tt.expected {
				if extracted[i] != exp {
					t.Errorf("match[%d]: expected %q, got %q", i, exp, extracted[i])
				}
			}
		})
	}
}
