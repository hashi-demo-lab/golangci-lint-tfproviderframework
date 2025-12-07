package testlintdata

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

type WidgetResource struct{}

func (r *WidgetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_widget"
}

func (r *WidgetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'widget' has no acceptance test file"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

func (r *WidgetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {}
func (r *WidgetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse)     {}
func (r *WidgetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {}
func (r *WidgetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {}
