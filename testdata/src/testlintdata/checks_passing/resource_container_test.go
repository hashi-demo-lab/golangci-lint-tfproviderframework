package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Import test steps don't require Check fields
func TestAccResourceContainer_import(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_container" "test" {
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("example_container.test", "id"),
				),
			},
			{
				ResourceName:      "example_container.test",
				ImportState:       true,
				ImportStateVerify: true,
				// No Check needed for import steps
			},
		},
	})
}
