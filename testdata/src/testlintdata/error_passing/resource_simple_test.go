package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Basic test only - no error test needed since no validators
func TestAccResourceSimple_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_simple" "test" {
  name = "test"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_simple.test", "name", "test"),
				),
			},
		},
	})
}
