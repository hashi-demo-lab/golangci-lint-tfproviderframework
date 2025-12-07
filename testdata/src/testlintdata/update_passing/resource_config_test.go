package update_passing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Has multi-step test that validates update behavior
func TestAccConfig_update(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_config" "test" { name = "initial" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_config.test", "name", "initial"),
				),
			},
			{
				Config: `resource "example_config" "test" { name = "updated" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_config.test", "name", "updated"),
				),
			},
		},
	})
}
