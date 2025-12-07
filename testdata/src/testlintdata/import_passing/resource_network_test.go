package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Basic test only - no import test needed since ImportState not implemented
func TestAccResourceNetwork_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_network" "test" {
  cidr = "10.0.0.0/16"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_network.test", "cidr", "10.0.0.0/16"),
				),
			},
		},
	})
}
