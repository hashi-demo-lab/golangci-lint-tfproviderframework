// Package basic_missing contains a resource without an acceptance test.
package basic_missing

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

// WidgetResource is a test resource without acceptance tests.
type WidgetResource struct{}

// Schema returns the schema for the widget resource.
func (r *WidgetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'widget' has no acceptance test"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}
