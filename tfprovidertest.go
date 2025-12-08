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
func (p *Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	var analyzers []*analysis.Analyzer

	if p.settings.EnableBasicTest {
		analyzers = append(analyzers, BasicTestAnalyzer)
	}
	if p.settings.EnableUpdateTest {
		analyzers = append(analyzers, UpdateTestAnalyzer)
	}
	if p.settings.EnableImportTest {
		analyzers = append(analyzers, ImportTestAnalyzer)
	}
	if p.settings.EnableErrorTest {
		analyzers = append(analyzers, ErrorTestAnalyzer)
	}
	if p.settings.EnableStateCheck {
		analyzers = append(analyzers, StateCheckAnalyzer)
	}

	return analyzers, nil
}

// GetLoadMode returns the AST load mode required by the analyzers.
func (p *Plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func init() {
	register.Plugin("tfprovidertest", New)
}

