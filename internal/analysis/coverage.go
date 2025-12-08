// Package analysis implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package analysis

import (
	"github.com/example/tfprovidertest/internal/registry"
)

// CoverageCalculator computes test coverage statistics for resources.
// It wraps a ResourceRegistry and provides methods to analyze test coverage.
type CoverageCalculator struct {
	registry *registry.ResourceRegistry
}

// NewCoverageCalculator creates a new CoverageCalculator for the given registry.
func NewCoverageCalculator(reg *registry.ResourceRegistry) *CoverageCalculator {
	return &CoverageCalculator{
		registry: reg,
	}
}

// GetResourceCoverage computes aggregated test coverage for a resource.
func (c *CoverageCalculator) GetResourceCoverage(resourceName string) *registry.ResourceCoverage {
	resource := c.registry.GetResourceOrDataSource(resourceName)
	if resource == nil {
		return nil
	}

	tests := c.registry.GetResourceTests(resourceName)
	return c.computeCoverage(resource, tests)
}

// GetAllResourceCoverage returns coverage information for all resources and data sources.
func (c *CoverageCalculator) GetAllResourceCoverage() []*registry.ResourceCoverage {
	definitions := c.registry.GetAllDefinitions()

	var coverages []*registry.ResourceCoverage
	for name, resource := range definitions {
		tests := c.registry.GetResourceTests(name)
		coverage := c.computeCoverage(resource, tests)
		coverages = append(coverages, coverage)
	}

	return coverages
}

// computeCoverage is a shared helper that computes coverage from resource and tests.
// This consolidates the duplicate logic that was in GetResourceCoverage and GetAllResourceCoverage.
func (c *CoverageCalculator) computeCoverage(resource *registry.ResourceInfo, tests []*registry.TestFunctionInfo) *registry.ResourceCoverage {
	coverage := &registry.ResourceCoverage{
		Resource:  resource,
		Tests:     tests,
		TestCount: len(tests),
	}

	for _, test := range tests {
		coverage.HasBasicTest = true

		if test.HasCheckDestroy {
			coverage.HasCheckDestroy = true
		}
		if test.HasImportStep {
			coverage.HasImportTest = true
		}
		if test.HasErrorCase {
			coverage.HasErrorTest = true
		}

		for _, step := range test.TestSteps {
			coverage.StepCount++

			if step.HasCheck || step.HasConfigStateChecks {
				coverage.HasStateCheck = true
			}
			if step.HasPlanCheck {
				coverage.HasPlanCheck = true
			}
			if step.ImportState {
				coverage.ImportStepCount++
			}
			if step.IsRealUpdateStep() {
				coverage.HasUpdateTest = true
				coverage.UpdateStepCount++
			}
		}
	}

	return coverage
}

// GetUntestedResources returns all resources and data sources that lack test coverage.
func (c *CoverageCalculator) GetUntestedResources() []*registry.ResourceInfo {
	definitions := c.registry.GetAllDefinitions()

	var untested []*registry.ResourceInfo
	for name, info := range definitions {
		if len(c.registry.GetResourceTests(name)) == 0 {
			untested = append(untested, info)
		}
	}
	return untested
}

// GetResourcesMissingStateChecks returns resources that have tests but no state/plan checks.
func (c *CoverageCalculator) GetResourcesMissingStateChecks() []*registry.ResourceCoverage {
	coverages := c.GetAllResourceCoverage()
	var missing []*registry.ResourceCoverage
	for _, cov := range coverages {
		// Only report resources that have tests but lack validation
		if cov.HasBasicTest && !cov.HasStateCheck && !cov.HasPlanCheck {
			missing = append(missing, cov)
		}
	}
	return missing
}

// GetResourcesMissingCheckDestroy returns resources that have tests but no CheckDestroy.
func (c *CoverageCalculator) GetResourcesMissingCheckDestroy() []*registry.ResourceCoverage {
	coverages := c.GetAllResourceCoverage()
	var missing []*registry.ResourceCoverage
	for _, cov := range coverages {
		// Only report resources that have tests but lack CheckDestroy
		// Data sources typically don't need CheckDestroy
		if cov.HasBasicTest && !cov.HasCheckDestroy && cov.Resource.Kind != registry.KindDataSource {
			missing = append(missing, cov)
		}
	}
	return missing
}
