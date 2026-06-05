package discovery

import (
	"testing"

	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
)

// TestDetectUnregisteredResources covers finding F3: a framework data source
// that is defined in source (constructor + Schema + Metadata) but never listed
// in the provider's DataSources() aggregator is flagged Unregistered, while a
// registered sibling is not. Mirrors terraform-provider-powerhmc, where
// NewLparDataSource exists but DataSources() lists only NewSystemConfigDataSource
// and NewViosDataSource.
//
// The constructor->canonical-name resolution must go through the returned
// type's Metadata (NewSysCfgDataSource -> sysCfgDataSource -> "sys_config"), not
// a name guess, so a Metadata-renamed registered resource is NOT falsely flagged.
func TestDetectUnregisteredResources(t *testing.T) {
	srcs := map[string]string{
		"provider.go": `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

type p struct{}

func (p *p) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSysCfgDataSource,
	}
}
`,
		"sys_config_data_source.go": `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func NewSysCfgDataSource() datasource.DataSource { return &sysCfgDataSource{} }

type sysCfgDataSource struct{}

func (d *sysCfgDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sys_config"
}

func (d *sysCfgDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {}
`,
		"lpar_data_source.go": `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func NewLparDataSource() datasource.DataSource { return &lparDataSource{} }

type lparDataSource struct{}

func (d *lparDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lpar"
}

func (d *lparDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {}
`,
	}

	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())
	DetectUnregisteredResources(files, fset, reg)

	get := func(name string) *registry.ResourceInfo {
		return reg.GetResourceOrDataSource("data source:" + name)
	}

	lpar := get("lpar")
	if lpar == nil {
		t.Fatalf("lpar data source not discovered")
	}
	if !lpar.Unregistered {
		t.Errorf("lpar should be flagged Unregistered (NewLparDataSource not in DataSources())")
	}

	sys := get("sys_config")
	if sys == nil {
		t.Fatalf("sys_config data source not discovered")
	}
	if sys.Unregistered {
		t.Errorf("sys_config should NOT be flagged Unregistered (registered via NewSysCfgDataSource despite Metadata rename)")
	}
}

// TestDetectUnregisteredResources_SpreadAggregatorIsNoop ensures that when an
// aggregator spreads a builder slice we cannot read (e.g.
// terraform-provider-hcp's `append(base, packer.ResourceSchemaBuilders...)`),
// nothing is flagged — the registered set is unknown, so flagging would risk
// false positives.
func TestDetectUnregisteredResources_SpreadAggregatorIsNoop(t *testing.T) {
	srcs := map[string]string{
		"provider.go": `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"

	"example.com/other/builders"
)

type p struct{}

func (p *p) DataSources(ctx context.Context) []func() datasource.DataSource {
	return append([]func() datasource.DataSource{}, builders.All...)
}
`,
		"lpar_data_source.go": `
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

func NewLparDataSource() datasource.DataSource { return &lparDataSource{} }

type lparDataSource struct{}

func (d *lparDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lpar"
}

func (d *lparDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {}
`,
	}
	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())
	DetectUnregisteredResources(files, fset, reg)

	for _, info := range reg.GetAllDefinitions() {
		if info.Unregistered {
			t.Errorf("%s flagged despite an unresolvable spread aggregator (should be conservative)", info.Name)
		}
	}
}

// TestDetectUnregisteredResources_NoAggregatorIsNoop ensures providers that do
// not use the aggregator pattern (e.g. SDKv2 / registry-factory) are never
// flagged — the check only applies when Resources()/DataSources()/Actions()
// methods are present.
func TestDetectUnregisteredResources_NoAggregatorIsNoop(t *testing.T) {
	srcs := map[string]string{
		"widget.go": `
package provider

import "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

func resourceWidget() *schema.Resource { return &schema.Resource{} }
`,
	}
	files, fset := parseFiles(t, srcs)
	reg := BuildRegistryFromFiles(files, fset, config.DefaultSettings())
	DetectUnregisteredResources(files, fset, reg)

	for _, info := range reg.GetAllDefinitions() {
		if info.Unregistered {
			t.Errorf("%s flagged Unregistered with no aggregator present", info.Name)
		}
	}
}
