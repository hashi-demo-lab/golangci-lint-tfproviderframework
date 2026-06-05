package analysis

import (
	"testing"

	"github.com/example/tfprovidertest/internal/registry"
)

// White-box tests for the coverage calculator, added in U3. These lock the
// coverage math so U5 (which consolidates the three overlapping coverage
// computations onto this CoverageCalculator) can proceed test-first and prove
// the JSON and table renderers agree with this single source of truth.
//
// The fixture mirrors powerhmc: a tested resource with a real CheckDestroy
// (vios), a tested resource, and an untested resource (lpar).

func buildPowerhmcLikeRegistry() *registry.ResourceRegistry {
	reg := registry.NewResourceRegistry()

	vios := &registry.ResourceInfo{Name: "vios", Kind: registry.KindResource}
	sysConfig := &registry.ResourceInfo{Name: "sys_config", Kind: registry.KindResource}
	lpar := &registry.ResourceInfo{Name: "lpar", Kind: registry.KindResource}
	reg.RegisterResource(vios)
	reg.RegisterResource(sysConfig)
	reg.RegisterResource(lpar)

	// vios: a real test with CheckDestroy + import step.
	viosTest := &registry.TestFunctionInfo{
		Name:             "TestAccViosResource_BasicLifecycle",
		UsesResourceTest: true,
		HasCheckDestroy:  true,
		HasImportStep:    true,
	}
	reg.RegisterTestFunction(viosTest)
	reg.LinkTestToResource("vios", viosTest)

	// sys_config: a test WITHOUT CheckDestroy (models the post-F2-fix reality
	// where CheckDestroy: nil is treated as absent).
	sysTest := &registry.TestFunctionInfo{
		Name:             "TestAccSystemConfigResource_ValidateUpdate",
		UsesResourceTest: true,
		HasCheckDestroy:  false,
	}
	reg.RegisterTestFunction(sysTest)
	reg.LinkTestToResource("sys_config", sysTest)

	// lpar: no tests -> untested.

	return reg
}

func TestCoverage_UntestedResources(t *testing.T) {
	calc := NewCoverageCalculator(buildPowerhmcLikeRegistry())

	untested := calc.GetUntestedResources()
	if len(untested) != 1 || untested[0].Name != "lpar" {
		var names []string
		for _, r := range untested {
			names = append(names, r.Name)
		}
		t.Fatalf("GetUntestedResources() = %v; want exactly [lpar]", names)
	}
}

func TestCoverage_MissingCheckDestroy(t *testing.T) {
	calc := NewCoverageCalculator(buildPowerhmcLikeRegistry())

	missing := map[string]bool{}
	for _, cov := range calc.GetResourcesMissingCheckDestroy() {
		missing[cov.Resource.Name] = true
	}

	// sys_config HAS tests but no CheckDestroy -> flagged.
	if !missing["sys_config"] {
		t.Errorf("sys_config should be flagged missing CheckDestroy (has tests, no destroy check)")
	}
	// vios HAS a real CheckDestroy -> not flagged.
	if missing["vios"] {
		t.Errorf("vios should NOT be flagged missing CheckDestroy (has testAccCheckViosDestroy)")
	}
	// lpar is untested. GetResourcesMissingCheckDestroy only flags resources
	// that HAVE tests but lack CheckDestroy (untested resources are reported
	// separately). This documents the chosen semantics U5 must preserve.
	if missing["lpar"] {
		t.Errorf("untested lpar should not appear in MissingCheckDestroy (it is reported as untested, not missing-destroy)")
	}
}

// TestCoverage_Summarize covers the single summary computation U5 introduced so
// the CLI table and JSON renderers can agree. It also pins the divergence fix:
// an action whose only state check is a plan check (ConfigPlanChecks) must NOT
// be counted as missing state checks — the old JSON path checked only
// HasCheck/HasConfigStateChecks and missed plan checks, disagreeing with the
// table path.
func TestCoverage_Summarize(t *testing.T) {
	reg := registry.NewResourceRegistry()

	reg.RegisterResource(&registry.ResourceInfo{Name: "widget", Kind: registry.KindResource})
	reg.RegisterResource(&registry.ResourceInfo{Name: "widget", Kind: registry.KindDataSource})
	reg.RegisterResource(&registry.ResourceInfo{Name: "do_thing", Kind: registry.KindAction})

	// widget resource: tested, no CheckDestroy -> missing CheckDestroy.
	rTest := &registry.TestFunctionInfo{Name: "TestAccWidgetResource_basic", UsesResourceTest: true}
	reg.RegisterTestFunction(rTest)
	reg.LinkTestToResource("resource:widget", rTest)

	// do_thing action: tested, with a PLAN check only (no Check / ConfigStateChecks).
	aTest := &registry.TestFunctionInfo{
		Name:             "TestAccDoThingAction_basic",
		UsesResourceTest: true,
		TestSteps:        []registry.TestStepInfo{{HasPlanCheck: true}},
	}
	reg.RegisterTestFunction(aTest)
	reg.LinkTestToResource("action:do_thing", aTest)

	// widget data source: untested.

	s := NewCoverageCalculator(reg).Summarize()

	if s.TotalResources != 1 || s.UntestedResources != 0 || s.MissingCheckDestroy != 1 {
		t.Errorf("resources: got total=%d untested=%d missingCD=%d; want 1/0/1", s.TotalResources, s.UntestedResources, s.MissingCheckDestroy)
	}
	if s.TotalDataSources != 1 || s.UntestedDataSources != 1 {
		t.Errorf("data sources: got total=%d untested=%d; want 1/1", s.TotalDataSources, s.UntestedDataSources)
	}
	if s.TotalActions != 1 || s.UntestedActions != 0 {
		t.Errorf("actions: got total=%d untested=%d; want 1/0", s.TotalActions, s.UntestedActions)
	}
	// The plan-check-only action must count as covered, not missing state checks.
	if s.MissingStateChecks != 0 {
		t.Errorf("actions missingStateChecks = %d; want 0 (a ConfigPlanChecks-only action has a state/plan check)", s.MissingStateChecks)
	}
}
