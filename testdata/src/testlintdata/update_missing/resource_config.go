package testlintdata

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// T033: Resource with updatable attributes but only single-step test
type ConfigResource struct{}

func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'config' has updatable attributes but no update test coverage"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
				// No PlanModifiers means updatable
			},
			"description": schema.StringAttribute{
				Optional: true,
				// No PlanModifiers means updatable
			},
			"tags": schema.MapAttribute{
				Optional: true,
				ElementType: schema.StringType{},
				// No PlanModifiers means updatable
			},
		},
	}
}
