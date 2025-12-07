package update_missing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Only has single-step test, missing update test
func TestAccConfig_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_config" "test" { name = "example" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_config.test", "name", "example"),
				),
			},
		},
	})
}
