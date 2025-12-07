package statecheck_missing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Test step without Check function
func TestAccItem_nocheck(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_item" "test" { name = "example" }`,
				// Missing Check!
			},
		},
	})
}
