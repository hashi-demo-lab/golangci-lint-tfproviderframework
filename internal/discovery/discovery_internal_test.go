package discovery

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/example/tfprovidertest/internal/registry"
)

// These white-box tests exercise the discovery strategies directly inside the
// `discovery` package. They were added in U3 to (a) lock in behavior the
// powerhmc validation (U1) confirmed correct, and (b) pin the defects that U6
// will fix. Fixtures mirror terraform-provider-powerhmc's shape: pure
// plugin-framework resources with lowercase unexported receiver types and a
// Metadata() TypeName that diverges from the Go type name.

// parseResourcesFromSrc is a small helper mirroring the inline-source pattern
// used by the root-package parser tests.
func parseResourcesFromSrc(t *testing.T, filename, src string) []*registry.ResourceInfo {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}
	return ParseResources(file, fset, filename)
}

func findResourceByName(resources []*registry.ResourceInfo, name string) *registry.ResourceInfo {
	for _, r := range resources {
		if r.Name == name {
			return r
		}
	}
	return nil
}

// TestParseResources_MetadataTypeNameOverride locks the confirmed-correct
// behavior from U1: MetadataMethodStrategy is authoritative for the canonical
// name and overrides the name guessed from the receiver type. The powerhmc
// `systemConfigResource` type sets TypeName "_sys_config", so it must be
// discovered as "sys_config", NOT the type-derived "system_config".
func TestParseResources_MetadataTypeNameOverride(t *testing.T) {
	src := `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type systemConfigResource struct{}

func NewSystemConfigResource() resource.Resource { return &systemConfigResource{} }

func (r *systemConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sys_config"
}

func (r *systemConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {}
`
	resources := parseResourcesFromSrc(t, "system_config_resource.go", src)

	if findResourceByName(resources, "sys_config") == nil {
		var names []string
		for _, r := range resources {
			names = append(names, r.Name)
		}
		t.Fatalf("expected resource canonical name %q from Metadata override; got %v", "sys_config", names)
	}
	if findResourceByName(resources, "system_config") != nil {
		t.Errorf("type-derived name %q should have been overridden by Metadata TypeName %q", "system_config", "sys_config")
	}
}

// TestParseResources_ImportStateOnUnexportedReceiver pins finding F1 from U1.
//
// powerhmc's resource types use lowercase unexported receivers (e.g.
// `systemConfigResource`) and rename the canonical resource via Metadata
// (e.g. "sys_config"). hasImportStateMethod reconstructs the expected receiver
// as toTitleCase(name)+"Resource" = "SysConfigResource", which never matches
// the real type `systemConfigResource`. The result is HasImportState=false
// even though an ImportState method exists, producing a false "missing
// ImportState" diagnostic in the analyzer/plugin path.
//
// SKIPPED until U6 resolves the receiver from the actual declaring type.
func TestParseResources_ImportStateOnUnexportedReceiver(t *testing.T) {
	t.Skip("U6/F1: hasImportStateMethod reconstructs the wrong receiver name for unexported, Metadata-renamed resources — see docs/plans/2026-06-05-001-powerhmc-validation-findings.md")

	src := `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type systemConfigResource struct{}

func NewSystemConfigResource() resource.Resource { return &systemConfigResource{} }

func (r *systemConfigResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sys_config"
}

func (r *systemConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {}

func (r *systemConfigResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {}
`
	resources := parseResourcesFromSrc(t, "system_config_resource.go", src)
	got := findResourceByName(resources, "sys_config")
	if got == nil {
		t.Fatalf("resource sys_config not discovered")
	}
	if !got.HasImportState {
		t.Errorf("HasImportState = false; want true (ImportState method exists on unexported receiver systemConfigResource)")
	}
}

// TestParseResources_SDKv2DataSourceFilenameClassification locks the current
// SDKv2 classification behavior: a *schema.Resource returned from a factory is
// classified as a data source only when the file is named data_source_*.go,
// otherwise as a resource. This pins the filename heuristic so U6's hardening
// (preferring stronger signals, preventing import-substring cross-fire) is a
// deliberate, test-visible change rather than silent drift.
func TestParseResources_SDKv2DataSourceFilenameClassification(t *testing.T) {
	dsSrc := `
package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func dataSourceWidget() *schema.Resource { return &schema.Resource{} }
`
	rSrc := `
package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceWidget() *schema.Resource { return &schema.Resource{} }
`
	ds := parseResourcesFromSrc(t, "data_source_widget.go", dsSrc)
	if len(ds) == 0 {
		t.Fatalf("expected a data source to be discovered from data_source_widget.go")
	}
	if ds[0].Kind != registry.KindDataSource {
		t.Errorf("data_source_widget.go: Kind = %v; want KindDataSource", ds[0].Kind)
	}

	r := parseResourcesFromSrc(t, "resource_widget.go", rSrc)
	if len(r) == 0 {
		t.Fatalf("expected a resource to be discovered from resource_widget.go")
	}
	if r[0].Kind != registry.KindResource {
		t.Errorf("resource_widget.go: Kind = %v; want KindResource", r[0].Kind)
	}
}
