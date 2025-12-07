package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Single-step test is OK for immutable resources
func TestAccResourceImmutable_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_immutable" "test" {
  name = "test"
  zone = "us-east-1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_immutable.test", "name", "test"),
				),
			},
		},
	})
}
