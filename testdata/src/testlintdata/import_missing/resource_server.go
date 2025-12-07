// Package import_missing contains a resource with ImportState but no import test.
package import_missing

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// ServerResource is a resource implementing ImportState but without import tests.
type ServerResource struct{}

// Schema returns the schema for the server resource.
func (r *ServerResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'server' implements ImportState but has no import test coverage"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}

// ImportState implements resource import.
func (r *ServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import logic here
}
