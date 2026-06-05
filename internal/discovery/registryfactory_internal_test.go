package discovery

import (
	"testing"

	"github.com/example/tfprovidertest/internal/registry"
)

// awsccResourceSrc mirrors terraform-provider-awscc's generated resource file
// shape: an init() that calls registry.AddResourceFactory with the authoritative
// Terraform type name, plus the factory function it references (which returns
// resource.Resource). Both the RegistryFactoryStrategy (full name) and the
// ReturnTypeStrategy (factory function -> stripped name) can see this resource.
const awsccResourceSrc = `
package accessanalyzer

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-provider-awscc/internal/registry"
)

func init() {
	registry.AddResourceFactory("awscc_accessanalyzer_analyzer", newResourceAnalyzer)
}

func newResourceAnalyzer(ctx context.Context) (resource.Resource, error) {
	return &analyzerResource{}, nil
}

type analyzerResource struct{}
`

// TestRegistryFactory_NoDoubleCountWithReturnType pins finding #11: the awscc
// double-count. A resource registered via registry.AddResourceFactory and also
// reachable via its factory function's return type must be counted ONCE, under
// the authoritative full name from the AddResourceFactory call — not twice
// (once as the full name and once as the ReturnType-stripped name).
func TestRegistryFactory_NoDoubleCountWithReturnType(t *testing.T) {
	resources := parseResourcesFromSrc(t, "analyzer_resource_gen.go", awsccResourceSrc)

	var resourceNames []string
	for _, r := range resources {
		if r.Kind == registry.KindResource {
			resourceNames = append(resourceNames, r.Name)
		}
	}

	if len(resourceNames) != 1 {
		t.Fatalf("expected exactly 1 resource, got %d: %v", len(resourceNames), resourceNames)
	}
	if resourceNames[0] != "awscc_accessanalyzer_analyzer" {
		t.Errorf("resource name = %q; want the authoritative %q", resourceNames[0], "awscc_accessanalyzer_analyzer")
	}
}
