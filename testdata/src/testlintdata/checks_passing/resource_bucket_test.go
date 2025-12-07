package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Test with proper Check field and validation functions
func TestAccResourceBucket_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_bucket" "test" {
  name = "test-bucket"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_bucket.test", "name", "test-bucket"),
					resource.TestCheckResourceAttrSet("example_bucket.test", "id"),
				),
			},
		},
	})
}
