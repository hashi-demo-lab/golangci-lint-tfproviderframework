package discovery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
)

func keysOf(m map[string]*registry.ResourceInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// U4: parity tests for the shared registry-construction routine. The plugin
// path (BuildRegistry) and the CLI path (buildRegistryFromFiles) previously
// diverged — the plugin never ran ParseProviderRegistryMaps (Google-style
// central resource maps) or ClassifyAllTests (orphan filtering). Both now
// delegate to BuildRegistryFromFiles, so these assertions hold for both.

func parseFiles(t *testing.T, srcs map[string]string) ([]*ast.File, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for name, src := range srcs {
		f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	return files, fset
}

// TestBuildRegistryFromFiles_IncludesProviderRegistryMaps proves the shared
// routine discovers Google-style central-map resources — the capability the
// plugin path was missing.
func TestBuildRegistryFromFiles_IncludesProviderRegistryMaps(t *testing.T) {
	srcs := map[string]string{
		"provider.go": `
package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

var generatedResources = map[string]*schema.Resource{
	"google_compute_instance": {},
	"google_storage_bucket":   {},
}
`,
	}
	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())

	defs := reg.GetAllDefinitions()
	if _, ok := defs["resource:google_compute_instance"]; !ok {
		t.Errorf("central-map resource google_compute_instance not discovered; got keys %v", keysOf(defs))
	}
	if _, ok := defs["resource:google_storage_bucket"]; !ok {
		t.Errorf("central-map resource google_storage_bucket not discovered; got keys %v", keysOf(defs))
	}
}

// TestBuildRegistryFromFiles_ClassifiesTests proves ClassifyAllTests runs: a
// provider-level test with no resources is classified as a provider test and
// excluded from the orphan list (without classification it would default to a
// resource test and be falsely counted as an orphan).
func TestBuildRegistryFromFiles_ClassifiesTests(t *testing.T) {
	srcs := map[string]string{
		"provider_test.go": `
package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccProvider_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{{Config: "# no resources"}},
	})
}
`,
	}
	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())

	for _, fn := range reg.GetUnmatchedTestFunctions() {
		if fn.Name == "TestAccProvider_basic" {
			t.Errorf("TestAccProvider_basic was counted as an orphan; ClassifyAllTests should classify it as a provider test and exclude it")
		}
	}
}
