package error_missing

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Only has happy path test, no error case
func TestAccValidated_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_validated" "test" { email = "test@example.com" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_validated.test", "email", "test@example.com"),
				),
			},
		},
	})
}
