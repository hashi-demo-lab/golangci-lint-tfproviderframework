package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Single-step test only - missing update test
func TestAccResourceConfig_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_config" "test" {
  name        = "initial"
  description = "initial description"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_config.test", "name", "initial"),
				),
			},
		},
	})
}
