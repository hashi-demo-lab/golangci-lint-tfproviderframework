package testlintdata

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// T035: Resource with multi-step update test (passing)
type ServerResource struct{}

func (r *ServerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Required: true,
			},
			"cpu_count": schema.Int64Attribute{
				Optional: true,
			},
			"memory_gb": schema.Int64Attribute{
				Optional: true,
			},
		},
	}
}
