package discovery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/example/tfprovidertest/pkg/config"
)

// U7 characterization tests for the two still-open prior-review items:
//
//   FIX-010 — test files using custom wrappers around resource.Test() were
//             returned as nil and ignored.
//   FIX-011 — name extraction returning empty for valid-but-unconventional
//             test names dropped the test instead of falling back.
//
// These encode the scenarios so the behavior is pinned; whichever the current
// code already satisfies is locked, and any genuine gap is fixed in this unit.

// FIX-010: a LOCAL helper that wraps resource.Test should cause tests using it
// to be recognized as acceptance tests.
func TestFix010_LocalHelperWrapperRecognized(t *testing.T) {
	src := `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// local wrapper around resource.Test
func myAccTest(t *testing.T, tc resource.TestCase) {
	resource.Test(t, tc)
}

func TestAccWidgetResource_basic(t *testing.T) {
	myAccTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"x_widget\" \"w\" {}`" + `},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "widget_resource_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Local helpers must be discovered and supplied via ParserConfig.
	localHelpers := findLocalTestHelpers([]*ast.File{file}, fset)
	cfg := DefaultParserConfig()
	cfg.LocalHelpers = localHelpers

	info := ParseTestFileWithConfig(file, fset, "widget_resource_test.go", cfg)
	if info == nil {
		t.Fatalf("ParseTestFileWithConfig returned nil")
	}
	var found bool
	for _, fn := range info.TestFunctions {
		if fn.Name == "TestAccWidgetResource_basic" {
			found = true
			if !fn.UsesResourceTest {
				t.Errorf("FIX-010: test using local wrapper myAccTest not recognized as a resource test")
			}
		}
	}
	if !found {
		t.Errorf("FIX-010: TestAccWidgetResource_basic was dropped entirely")
	}
}

// FIX-011: a validly-named test whose Config makes its target unambiguous must
// be matched (not dropped) even if function-name extraction is weak.
func TestFix011_UnconventionalNameFallsBackToInferredContent(t *testing.T) {
	src := `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Unconventional name (no resource/data-source token to extract from).
func TestThingamajig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"x_widget\" \"w\" {}`" + `},
		},
	})
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "misc_test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := ParseTestFileWithConfig(file, fset, "misc_test.go", DefaultParserConfig())
	if info == nil {
		t.Fatalf("ParseTestFileWithConfig returned nil")
	}
	var fn *testFnView
	for i := range info.TestFunctions {
		if info.TestFunctions[i].Name == "TestThingamajig" {
			fn = &testFnView{used: info.TestFunctions[i].UsesResourceTest, inferred: info.TestFunctions[i].InferredResources}
		}
	}
	if fn == nil {
		t.Fatalf("FIX-011: TestThingamajig was dropped entirely")
	}
	if !fn.used {
		t.Errorf("FIX-011: TestThingamajig not recognized as a resource test")
	}
	// The Config references x_widget; inferred-content must capture it so the
	// linker can match despite the unconventional function name.
	if len(fn.inferred) == 0 {
		t.Errorf("FIX-011: no inferred resources captured from Config; linker has nothing to fall back to")
	}
}

type testFnView struct {
	used     bool
	inferred []string
}

// TestFix011_EndToEnd_UnconventionalTestIsMatchedNotOrphaned proves the
// user-facing outcome: an unconventionally-named test whose Config targets a
// known resource is matched via inferred content and is NOT reported as an
// orphan (the false-negative FIX-011 warned about).
func TestFix011_EndToEnd_UnconventionalTestIsMatchedNotOrphaned(t *testing.T) {
	srcs := map[string]string{
		"widget_resource.go": `
package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceWidget() *schema.Resource { return &schema.Resource{} }
`,
		"misc_test.go": `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestThingamajig(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"widget\" \"w\" {}`" + `},
		},
	})
}
`,
	}
	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())

	for _, fn := range reg.GetUnmatchedTestFunctions() {
		if fn.Name == "TestThingamajig" {
			t.Errorf("FIX-011: TestThingamajig reported as orphan despite Config targeting the widget resource")
		}
	}
}
