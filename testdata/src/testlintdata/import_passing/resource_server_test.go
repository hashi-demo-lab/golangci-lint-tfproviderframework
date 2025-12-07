package import_passing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Test with import step
func TestAccServer_import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_server" "test" { name = "example" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_server.test", "name", "example"),
				),
			},
			{
				ResourceName:      "example_server.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
