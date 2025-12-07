package error_passing

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Has error case test with ExpectError
func TestAccValidated_invalid(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config:      `resource "example_validated" "test" { email = "" }`,
				ExpectError: regexp.MustCompile("string length must be at least 1"),
			},
		},
	})
}

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
