package inferred_matching

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// This test file has a random name that doesn't match any naming convention
// The linter should still link it to the "example_widget" resource via inferred matching
// because the Config contains: resource "example_widget" "test"

func TestSomethingCompletelyRandom_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `resource "example_widget" "test" { name = "example" }`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_widget.test", "name", "example"),
				),
			},
		},
	})
}
