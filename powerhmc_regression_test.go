package tfprovidertest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/example/tfprovidertest/internal/analysis"
	"github.com/example/tfprovidertest/internal/discovery"
	"github.com/example/tfprovidertest/pkg/config"
)

// TestPowerhmcCoverageRegression locks the expected coverage numbers for the
// vendored terraform-provider-powerhmc validation provider (U8). powerhmc is a
// pure plugin-framework provider with deterministic single-file-per-resource
// discovery, chosen as the guard for the U1 findings and their U4-U6 fixes:
//
//   - 3 resources; lpar untested; sys_config flagged "missing CheckDestroy"
//     because all its tests use `CheckDestroy: nil` (finding F2 / U6).
//   - 3 data sources, all untested.
//   - 2 actions, all untested.
//   - 0 orphan test functions.
//
// If the vendored source is not present (e.g. a fresh clone where the gitlink
// is unpopulated), the test skips rather than failing.
func TestPowerhmcCoverageRegression(t *testing.T) {
	const providerDir = "validation/terraform-provider-powerhmc/internal/provider"

	if _, err := os.Stat(providerDir); err != nil {
		t.Skipf("vendored powerhmc source not present (%v); skipping regression lock", err)
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, providerDir, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", providerDir, err)
	}

	// Collect all files and build the registry via the shared routine.
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			files = append(files, f)
		}
	}

	reg := discovery.BuildRegistryFromFiles(files, fset, config.DefaultSettings())
	s := analysis.NewCoverageCalculator(reg).Summarize()

	if s.TotalResources != 3 || s.UntestedResources != 1 {
		t.Errorf("resources: total=%d untested=%d; want 3/1 (lpar untested)", s.TotalResources, s.UntestedResources)
	}
	if s.MissingCheckDestroy != 1 {
		t.Errorf("missingCheckDestroy=%d; want 1 (sys_config tests all use CheckDestroy: nil — F2)", s.MissingCheckDestroy)
	}
	if s.TotalDataSources != 3 || s.UntestedDataSources != 3 {
		t.Errorf("data sources: total=%d untested=%d; want 3/3", s.TotalDataSources, s.UntestedDataSources)
	}
	if s.TotalActions != 2 || s.UntestedActions != 2 {
		t.Errorf("actions: total=%d untested=%d; want 2/2", s.TotalActions, s.UntestedActions)
	}
}
