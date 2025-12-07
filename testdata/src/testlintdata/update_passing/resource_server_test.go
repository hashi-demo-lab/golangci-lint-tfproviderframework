package testlintdata

import (
	"testing"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Multi-step test with configuration changes - proper update test
func TestAccResourceServer_update(t *testing.T) {
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: `
resource "example_server" "test" {
  hostname  = "server1"
  cpu_count = 2
  memory_gb = 4
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_server.test", "cpu_count", "2"),
				),
			},
			{
				Config: `
resource "example_server" "test" {
  hostname  = "server1"
  cpu_count = 4
  memory_gb = 8
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("example_server.test", "cpu_count", "4"),
					resource.TestCheckResourceAttr("example_server.test", "memory_gb", "8"),
				),
			},
		},
	})
}
