package statecheck_passing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Test step with proper Check function
func TestAccItem_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_item" "test" { name = "example" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_item.test", "name", "example"),
				),
			},
		},
	})
}
