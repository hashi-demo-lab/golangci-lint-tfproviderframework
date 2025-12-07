// Package statecheck_passing contains a resource with properly checked test steps.
package statecheck_passing

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// ItemResource is a resource with test steps that include Check functions.
type ItemResource struct{}

// Schema returns the schema for the item resource.
func (r *ItemResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}
