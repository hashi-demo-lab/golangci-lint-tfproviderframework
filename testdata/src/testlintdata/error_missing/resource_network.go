package testlintdata

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// T058: Resource with validators but no error test
type NetworkResource struct{}

func (r *NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) { // want "resource 'network' has validation rules but no error case tests"
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"cidr": schema.StringAttribute{
				Required: true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+/[0-9]+$`), "must be valid CIDR"),
				},
			},
			"name": schema.StringAttribute{
				Required: true,
			},
		},
	}
}
