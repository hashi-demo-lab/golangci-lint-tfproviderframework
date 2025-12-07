package testlintdata

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// This resource has a proper acceptance test - should NOT trigger diagnostic
func TestAccResourceAccount_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_account" "test" {
  name = "test-account"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_account.test", "name", "test-account"),
				),
			},
		},
	})
}
