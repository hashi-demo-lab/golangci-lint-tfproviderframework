package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Proper import test with ImportState and ImportStateVerify
func TestAccResourceDatabase_importBasic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_database" "test" {
  name = "testdb"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_database.test", "name", "testdb"),
				),
			},
			{
				ResourceName:      "example_database.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
