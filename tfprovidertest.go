// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
//
// The plugin provides five analyzers that enforce HashiCorp's testing best practices:
//   - Basic Test Coverage: Detects resources without acceptance tests
//   - Update Test Coverage: Validates multi-step tests for updatable attributes
//   - Import Test Coverage: Ensures ImportState methods have import tests
//   - Error Test Coverage: Verifies validation rules have error case tests
//   - State Check Validation: Confirms test steps include state validation functions
//
// This implementation uses a simplified "File-First" approach for test association:
// - Resources are identified by AST analysis (Schema() methods)
// - Tests are associated by file naming convention (resource_widget.go -> resource_widget_test.go)
// - This is more reliable than function name parsing and follows Go best practices
package tfprovidertest

import (
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

// Plugin implements the golangci-lint plugin interface.
type Plugin struct {
	settings Settings
}

// New creates a new plugin instance with the given settings.
func New(settings any) (register.LinterPlugin, error) {
	s := DefaultSettings()
	if settings != nil {
		decoded, err := register.DecodeSettings[Settings](settings)
		if err != nil {
			return nil, fmt.Errorf("failed to decode settings: %w", err)
		}
		s = decoded
	}
	return &Plugin{settings: s}, nil
}

// BuildAnalyzers returns the list of enabled analyzers based on settings.
// Each analyzer is created dynamically with a closure that captures the plugin's settings.
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	var analyzers []*analysis.Analyzer

	if p.settings.EnableBasicTest {
		analyzers = append(analyzers, p.createBasicTestAnalyzer())
	}
	if p.settings.EnableUpdateTest {
		analyzers = append(analyzers, p.createUpdateTestAnalyzer())
	}
	if p.settings.EnableImportTest {
		analyzers = append(analyzers, p.createImportTestAnalyzer())
	}
	if p.settings.EnableErrorTest {
		analyzers = append(analyzers, p.createErrorTestAnalyzer())
	}
	if p.settings.EnableStateCheck {
		analyzers = append(analyzers, p.createStateCheckAnalyzer())
	}
	if p.settings.EnableBasicTest || p.settings.EnableUpdateTest ||
	   p.settings.EnableImportTest || p.settings.EnableErrorTest || p.settings.EnableStateCheck {
		analyzers = append(analyzers, p.createDriftCheckAnalyzer())
		analyzers = append(analyzers, p.createSweeperAnalyzer())
	}

	return analyzers, nil
}

// createBasicTestAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createBasicTestAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-resource-basic-test",
		Doc:  "Checks that every resource and data source has at least one acceptance test.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runBasicTestAnalyzer(pass, p.settings)
		},
	}
}

// createUpdateTestAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createUpdateTestAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-resource-update-test",
		Doc:  "Checks that resources with updatable attributes have multi-step update tests.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runUpdateTestAnalyzer(pass, p.settings)
		},
	}
}

// createImportTestAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createImportTestAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-resource-import-test",
		Doc:  "Checks that resources implementing ImportState have import tests.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runImportTestAnalyzer(pass, p.settings)
		},
	}
}

// createErrorTestAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createErrorTestAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-test-error-cases",
		Doc:  "Checks that resources with validation rules have error case tests.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runErrorTestAnalyzer(pass, p.settings)
		},
	}
}

// createStateCheckAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createStateCheckAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-test-check-functions",
		Doc:  "Checks that test steps include state validation check functions.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runStateCheckAnalyzer(pass, p.settings)
		},
	}
}

// createDriftCheckAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createDriftCheckAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-test-drift-check",
		Doc:  "Checks that acceptance tests include CheckDestroy for drift detection.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runDriftCheckAnalyzer(pass, p.settings)
		},
	}
}

// createSweeperAnalyzer creates an analyzer with settings captured via closure.
func (p *Plugin) createSweeperAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tfprovider-test-sweepers",
		Doc:  "Checks that packages have test sweeper registrations for cleanup.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			return runSweeperAnalyzer(pass, p.settings)
		},
	}
}

// GetLoadMode returns the AST load mode required by the analyzers.
func (p *Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func init() {
	register.Plugin("tfprovidertest", New)
}

