// Package statecheck_missing contains a resource with test steps missing Check.
package statecheck_missing

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// ItemResource is a resource with test steps that don't include Check functions.
type ItemResource struct{}

// Schema returns the schema for the item resource.
func (r *ItemResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "test step for resource 'item' has no state validation checks"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}
