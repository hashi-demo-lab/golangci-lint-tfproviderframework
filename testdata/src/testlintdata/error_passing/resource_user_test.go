package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Proper error case test with ExpectError
func TestAccResourceUser_invalidEmail(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_user" "test" {
  email = "invalid-email"
}
`,
				ExpectError: regexp.MustCompile("must be valid email"),
			},
		},
	})
}

func TestAccResourceUser_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_user" "test" {
  email = "test@example.com"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_user.test", "email", "test@example.com"),
				),
			},
		},
	})
}
