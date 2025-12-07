package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Test step missing Check field - should be flagged
func TestAccResourceDatabase_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_database" "test" {
  name = "testdb"
  size = 100
}
`,
				// No Check field - this should be reported // want "test step for resource 'database' has no state validation checks"
			},
		},
	})
}
