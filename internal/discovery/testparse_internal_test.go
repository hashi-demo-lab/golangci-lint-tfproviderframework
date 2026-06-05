package discovery

import (
	"go/parser"
	"go/token"
	"testing"
)

// White-box tests for test-file parsing, added in U3. These pin the
// CheckDestroy detection behavior the powerhmc validation (U1) probed,
// including finding F2 (a literal `CheckDestroy: nil` counted as present).

func parseTestFuncs(t *testing.T, filename, src string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}
	info := ParseTestFileWithConfig(file, fset, filename, DefaultParserConfig())
	if info == nil {
		t.Fatalf("ParseTestFileWithConfig returned nil for %s", filename)
	}
	out := map[string]bool{}
	for i := range info.TestFunctions {
		out[info.TestFunctions[i].Name] = info.TestFunctions[i].HasCheckDestroy
	}
	return out
}

// TestParseTestFile_RealCheckDestroyDetected locks the confirmed-correct case:
// a TestCase with a non-nil CheckDestroy function is reported as having
// CheckDestroy. (powerhmc's vios test uses CheckDestroy: testAccCheckViosDestroy.)
func TestParseTestFile_RealCheckDestroyDetected(t *testing.T) {
	src := `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccCheckViosDestroy(s *terraform.State) error { return nil }

func TestAccViosResource_BasicLifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		CheckDestroy: testAccCheckViosDestroy,
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"powerhmc_vios\" \"v\" {}`" + `},
		},
	})
}
`
	got := parseTestFuncs(t, "vios_resource_test.go", src)
	if !got["TestAccViosResource_BasicLifecycle"] {
		t.Errorf("HasCheckDestroy = false; want true for a non-nil CheckDestroy function")
	}
}

// TestParseTestFile_InferredResourcesAreSorted pins finding #13: a test that
// configures multiple resources must expose its inferred resources in a stable
// (sorted) order. The dedup maps the parser uses would otherwise yield
// nondeterministic order, and the linker's first-match-wins would flip such a
// test between candidate resources run-to-run (e.g. a google IAM test that
// configures both the base resource and its *_iam_policy).
func TestParseTestFile_InferredResourcesAreSorted(t *testing.T) {
	src := `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccZebraThenApple(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"x_zebra\" \"z\" {}\nresource \"x_apple\" \"a\" {}`" + `},
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
	if info == nil || len(info.TestFunctions) == 0 {
		t.Fatalf("no test functions parsed")
	}

	got := info.TestFunctions[0].InferredResources
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("InferredResources not sorted: %v", got)
			break
		}
	}
	// Sanity: both resources were captured.
	if len(got) < 2 {
		t.Errorf("expected >=2 inferred resources, got %v", got)
	}
}

// TestParseTestFile_NilCheckDestroyNotCounted pins finding F2 from U1.
//
// powerhmc's system_config tests all set `CheckDestroy: nil`, yet the report
// shows CheckDestroy present. A literal nil destroy check is inert and should
// not count as coverage (same class as a t.Skip()'d test). Fixed in U6 via
// isNilIdent.
func TestParseTestFile_NilCheckDestroyNotCounted(t *testing.T) {
	src := `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSystemConfigResource_ValidateUpdate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		CheckDestroy: nil,
		Steps: []resource.TestStep{
			{Config: ` + "`resource \"powerhmc_sys_config\" \"s\" {}`" + `},
		},
	})
}
`
	got := parseTestFuncs(t, "system_config_resource_test.go", src)
	if got["TestAccSystemConfigResource_ValidateUpdate"] {
		t.Errorf("HasCheckDestroy = true for `CheckDestroy: nil`; want false (nil destroy check is inert)")
	}
}
