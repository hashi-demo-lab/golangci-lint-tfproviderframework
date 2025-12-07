// Package update_missing contains a resource with updatable attrs but no update test.
package update_missing

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// ConfigResource is a resource with updatable attributes but only single-step test.
type ConfigResource struct{}

// Schema returns the schema for the config resource.
func (r *ConfigResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'config' has updatable attributes but no update test coverage"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional: true,
			},
			"description": schema.StringAttribute{
				Optional: true,
			},
		},
	}
}
