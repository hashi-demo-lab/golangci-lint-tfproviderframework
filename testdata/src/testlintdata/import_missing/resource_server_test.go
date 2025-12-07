package import_missing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Basic test without import step
func TestAccServer_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_server" "test" { name = "example" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_server.test", "name", "example"),
				),
			},
		},
	})
}
