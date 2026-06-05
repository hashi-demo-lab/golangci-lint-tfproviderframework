// Command validate runs the tfprovidertest analyzers against a Terraform provider.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/example/tfprovidertest"
	tfanalysis "github.com/example/tfprovidertest/internal/analysis"
	"github.com/example/tfprovidertest/internal/discovery"
	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
	"golang.org/x/tools/go/analysis"
)

func main() {
	// Basic flags
	providerPath := flag.String("provider", "", "Path to the Terraform provider directory")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	recursive := flag.Bool("recursive", false, "Recursively scan all subdirectories for Go packages")
	scanPath := flag.String("scan-path", "", "Explicit path within provider to scan (overrides auto-detection)")

	// Report flag
	showReport := flag.Bool("report", false, "Show comprehensive coverage report with table views")
	outputFormat := flag.String("format", "text", "Output format: text, json, or table")

	// Strategy flags
	matchStrategy := flag.String("match-strategy", "all", "Matching strategy: function, file, fuzzy, or all")
	confidenceThreshold := flag.Float64("confidence-threshold", 0.7, "Minimum confidence for matches (0.0-1.0)")

	// Provider-specific flags
	providerPrefix := flag.String("provider-prefix", "", "Provider prefix for function name matching (e.g., AWS, Google)")

	flag.Parse()

	if *providerPath == "" {
		printUsage()
		os.Exit(1)
	}

	// Determine directories to scan
	var scanDirs []string

	if *scanPath != "" {
		// Explicit scan path provided
		fullPath := filepath.Join(*providerPath, *scanPath)
		if stat, err := os.Stat(fullPath); err != nil || !stat.IsDir() {
			fmt.Printf("Error: Specified scan path does not exist: %s\n", fullPath)
			os.Exit(1)
		}
		scanDirs = []string{fullPath}
	} else if *recursive {
		// Recursive scanning - find all directories with Go files
		scanDirs = findAllGoPackageDirs(*providerPath)
		if len(scanDirs) == 0 {
			fmt.Printf("Error: No Go packages found in %s (recursive scan)\n", *providerPath)
			os.Exit(1)
		}
	} else {
		// Standard auto-detection
		providerCodeDir := findProviderCodeDir(*providerPath)
		if providerCodeDir == "" {
			fmt.Printf("Error: Could not find provider code directory in %s\n", *providerPath)
			fmt.Println("\nTried the following locations:")
			fmt.Println("  - internal/provider")
			fmt.Println("  - internal")
			fmt.Println("  - <provider-name> (extracted from directory name)")
			fmt.Println("\nTip: Use -recursive flag to scan all subdirectories")
			fmt.Println("     Use -scan-path to specify an explicit path")
			os.Exit(1)
		}
		scanDirs = []string{providerCodeDir}
	}

	// Display what we're scanning
	if len(scanDirs) == 1 {
		fmt.Printf("Analyzing provider at: %s\n\n", scanDirs[0])
	} else {
		fmt.Printf("Analyzing provider at: %s (%d directories)\n\n", *providerPath, len(scanDirs))
	}

	// Build settings from flags
	settings := config.DefaultSettings()
	settings.Verbose = *verbose
	settings.FuzzyMatchThreshold = *confidenceThreshold
	settings.ProviderPrefix = *providerPrefix

	// Configure matching strategy
	// Note: Function name matching and file-based matching always run (not configurable)
	switch *matchStrategy {
	case "function", "file", "all":
		// Function and file matching always run
		settings.EnableFuzzyMatching = false
	case "fuzzy":
		// Enable fuzzy matching in addition to function and file matching
		settings.EnableFuzzyMatching = true
	default:
		fmt.Printf("Error: Invalid match-strategy '%s'. Must be one of: function, file, fuzzy, all\n", *matchStrategy)
		os.Exit(1)
	}

	// Validate settings
	if err := validateSettings(settings); err != nil {
		fmt.Printf("Error: Invalid settings: %v\n", err)
		os.Exit(1)
	}

	// Parse all Go files from all scan directories
	fset := token.NewFileSet()
	var allFiles []*ast.File

	for _, dir := range scanDirs {
		pkgs, err := parser.ParseDir(fset, dir, nil, parser.ParseComments)
		if err != nil {
			if *verbose {
				fmt.Printf("Warning: Error parsing %s: %v\n", dir, err)
			}
			continue
		}

		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				allFiles = append(allFiles, file)
			}
		}
	}

	if len(allFiles) == 0 {
		fmt.Printf("Error: No Go files found in scanned directories\n")
		os.Exit(1)
	}

	// Handle report command - comprehensive coverage report
	if *showReport {
		runReport(fset, allFiles, settings, *outputFormat)
		return
	}

	// Run standard analysis
	runAnalyzers(fset, allFiles, settings)
}

// printUsage outputs comprehensive help text for the validate command
func printUsage() {
	fmt.Println("Usage: validate -provider <path> [options]")
	fmt.Println()
	fmt.Println("tfprovidertest validates Terraform provider test coverage by analyzing")
	fmt.Println("resource definitions and their corresponding acceptance tests.")
	fmt.Println()
	fmt.Println("Basic Options:")
	fmt.Println("  -provider string")
	fmt.Println("        Path to the Terraform provider directory (required)")
	fmt.Println("  -verbose")
	fmt.Println("        Enable verbose diagnostic output")
	fmt.Println()
	fmt.Println("Report Options:")
	fmt.Println("  -report")
	fmt.Println("        Show comprehensive coverage report with table views")
	fmt.Println()
	fmt.Println("Matching Options:")
	fmt.Println("  -match-strategy string")
	fmt.Println("        Matching strategy: function, file, fuzzy, or all (default: all)")
	fmt.Println("        - function: Match via test function name analysis only")
	fmt.Println("        - file: Match via file proximity only (resource_x.go <-> resource_x_test.go)")
	fmt.Println("        - fuzzy: Enable fuzzy string matching for resource names")
	fmt.Println("        - all: Use both function and file matching (default)")
	fmt.Println("  -confidence-threshold float")
	fmt.Println("        Minimum confidence for matches, 0.0-1.0 (default: 0.7)")
	fmt.Println("  -provider-prefix string")
	fmt.Println("        Provider prefix for function name matching (e.g., AWS, Google)")
	fmt.Println("        Helps extract resource names from functions like TestAccAWSInstance_basic")
	fmt.Println()
	fmt.Println("Output Options:")
	fmt.Println("  -format string")
	fmt.Println("        Output format: text, json, or table (default: text)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Run standard analysis")
	fmt.Println("  validate -provider ./terraform-provider-aws")
	fmt.Println()
	fmt.Println("  # Show comprehensive coverage report in table format")
	fmt.Println("  validate -provider ./provider -report -format table")
	fmt.Println()
	fmt.Println("  # Use function-only matching with custom threshold")
	fmt.Println("  validate -provider ./provider -match-strategy function -confidence-threshold 0.8")
}

// validateSettings performs validation on the settings configuration
func validateSettings(settings config.Settings) error {
	// Validate confidence threshold range
	if settings.FuzzyMatchThreshold < 0.0 || settings.FuzzyMatchThreshold > 1.0 {
		return fmt.Errorf("confidence-threshold must be between 0.0 and 1.0, got %f", settings.FuzzyMatchThreshold)
	}

	// Function name matching and file-based matching always run (no validation needed)
	return nil
}

// runAnalyzers executes the standard analysis workflow
func runAnalyzers(fset *token.FileSet, files []*ast.File, settings config.Settings) {
	// Create plugin with settings map
	settingsMap := map[string]interface{}{
		"Verbose":               settings.Verbose,
		"EnableBasicTest":       settings.EnableBasicTest,
		"EnableUpdateTest":      settings.EnableUpdateTest,
		"EnableImportTest":      settings.EnableImportTest,
		"EnableErrorTest":       settings.EnableErrorTest,
		"EnableStateCheck":      settings.EnableStateCheck,
		"EnableFuzzyMatching":   settings.EnableFuzzyMatching,
		"FuzzyMatchThreshold":   settings.FuzzyMatchThreshold,
		"ProviderPrefix":        settings.ProviderPrefix,
		"ShowMatchConfidence":   settings.ShowMatchConfidence,
		"ShowUnmatchedTests":    settings.ShowUnmatchedTests,
		"ShowOrphanedResources": settings.ShowOrphanedResources,
	}

	plugin, err := tfprovidertest.New(settingsMap)
	if err != nil {
		fmt.Printf("Error creating plugin: %v\n", err)
		os.Exit(1)
	}

	// Get all analyzers
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		fmt.Printf("Error building analyzers: %v\n", err)
		os.Exit(1)
	}

	// Create a simple analysis pass for each analyzer
	totalIssues := 0
	for _, analyzer := range analyzers {
		fmt.Printf("Running %s...\n", analyzer.Name)

		pass := &analysis.Pass{
			Analyzer: analyzer,
			Fset:     fset,
			Files:    files,
			Report: func(diag analysis.Diagnostic) {
				pos := fset.Position(diag.Pos)
				fmt.Printf("\n[%s] %s:%d\n", analyzer.Name, pos.Filename, pos.Line)
				fmt.Printf("  %s\n", diag.Message)
				totalIssues++
			},
		}

		_, err := analyzer.Run(pass)
		if err != nil {
			fmt.Printf("  Error running analyzer: %v\n", err)
		}
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	if totalIssues == 0 {
		fmt.Println("No issues found - all resources have proper test coverage!")
	} else {
		fmt.Printf("Found %d issue(s)\n", totalIssues)
	}
}

// findProviderCodeDir attempts to locate the provider code directory
func findProviderCodeDir(providerPath string) string {
	possiblePaths := []string{
		filepath.Join(providerPath, "internal", "provider"),
		filepath.Join(providerPath, "internal"),
		filepath.Join(providerPath, filepath.Base(providerPath)),
	}

	// For providers named terraform-provider-X, also try just X
	baseName := filepath.Base(providerPath)
	if strings.HasPrefix(baseName, "terraform-provider-") {
		shortName := strings.TrimPrefix(baseName, "terraform-provider-")
		possiblePaths = append(possiblePaths, filepath.Join(providerPath, shortName))
	}

	for _, path := range possiblePaths {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return path
		}
	}

	return ""
}

// buildRegistryFromFiles creates a registry from parsed AST files.
// It delegates to discovery.BuildRegistryFromFiles — the single
// registry-construction routine shared with the golangci-lint plugin — so the
// CLI and plugin always produce identical results.
func buildRegistryFromFiles(fset *token.FileSet, files []*ast.File, settings config.Settings) *registry.ResourceRegistry {
	return discovery.BuildRegistryFromFiles(files, fset, settings)
}

// runReport generates a comprehensive coverage report with table views
func runReport(fset *token.FileSet, files []*ast.File, settings config.Settings, format string) {
	reg := buildRegistryFromFiles(fset, files, settings)

	// Flag resources defined in source but not listed in the provider's
	// Resources()/DataSources()/Actions() aggregators (no-op for non-aggregator
	// providers).
	discovery.DetectUnregisteredResources(files, fset, reg)

	allDefs := reg.GetAllDefinitions()

	// Group definitions by kind
	var resources, dataSources, actions []*registry.ResourceInfo
	for _, info := range allDefs {
		switch info.Kind {
		case registry.KindResource:
			resources = append(resources, info)
		case registry.KindDataSource:
			dataSources = append(dataSources, info)
		case registry.KindAction:
			actions = append(actions, info)
		}
	}

	// Sort each slice by name
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	sort.Slice(dataSources, func(i, j int) bool { return dataSources[i].Name < dataSources[j].Name })
	sort.Slice(actions, func(i, j int) bool { return actions[i].Name < actions[j].Name })

	orphans := reg.GetUnmatchedTestFunctions()

	switch format {
	case "json":
		outputReportJSON(reg, resources, dataSources, actions, orphans)
	case "table":
		outputReportTable(reg, resources, dataSources, actions, orphans)
	default:
		outputReportTable(reg, resources, dataSources, actions, orphans)
	}
}

// ReportData holds all data for JSON output
type ReportData struct {
	Summary     ReportSummary    `json:"summary"`
	Resources   []ResourceReport `json:"resources"`
	DataSources []ResourceReport `json:"data_sources"`
	Actions     []ResourceReport `json:"actions"`
	Orphans     []OrphanReport   `json:"orphan_tests"`
}

type ReportSummary struct {
	TotalResources      int `json:"total_resources"`
	UntestedResources   int `json:"untested_resources"`
	TotalDataSources    int `json:"total_data_sources"`
	UntestedDataSources int `json:"untested_data_sources"`
	TotalActions        int `json:"total_actions"`
	UntestedActions     int `json:"untested_actions"`
	OrphanTests         int `json:"orphan_tests"`
	MissingCheckDestroy int `json:"missing_check_destroy"`
	MissingStateChecks  int `json:"missing_state_checks"`
}

type ResourceReport struct {
	Name                 string       `json:"name"`
	File                 string       `json:"file"`
	TestFile             string       `json:"test_file"`
	TestCount            int          `json:"test_count"`
	HasCheckDestroy      bool         `json:"has_check_destroy"`
	HasCheck             bool         `json:"has_check"`               // Legacy Check field
	HasConfigStateChecks bool         `json:"has_config_state_checks"` // Modern ConfigStateChecks field
	HasPlanCheck         bool         `json:"has_plan_check"`
	HasImportTest        bool         `json:"has_import_test"`
	HasUpdateTest        bool         `json:"has_update_test"`
	HasExpectError       bool         `json:"has_expect_error"`
	HasPreCheck          bool         `json:"has_pre_check"`
	Tests                []TestReport `json:"tests"`
}

type TestReport struct {
	Name      string `json:"name"`
	File      string `json:"file"`
	MatchType string `json:"match_type"`
}

type OrphanReport struct {
	Name              string   `json:"name"`
	File              string   `json:"file"`
	InferredResources []string `json:"inferred_resources,omitempty"`
}

func buildResourceReport(reg *registry.ResourceRegistry, info *registry.ResourceInfo) ResourceReport {
	key := info.Kind.String() + ":" + info.Name
	tests := reg.GetResourceTests(key)

	report := ResourceReport{
		Name:      info.Name,
		File:      filepath.Base(info.FilePath),
		TestCount: len(tests),
	}

	// Track unique test files
	testFiles := make(map[string]bool)

	for _, t := range tests {
		testFile := filepath.Base(t.FilePath)
		testFiles[testFile] = true
		report.Tests = append(report.Tests, TestReport{
			Name:      t.Name,
			File:      testFile,
			MatchType: t.MatchType.String(),
		})
		if t.HasCheckDestroy {
			report.HasCheckDestroy = true
		}
		if t.HasImportStep {
			report.HasImportTest = true
		}
		for _, step := range t.TestSteps {
			if step.IsRealUpdateStep() {
				report.HasUpdateTest = true
			}
			if step.ExpectError {
				report.HasExpectError = true
			}
			if step.HasPlanCheck {
				report.HasPlanCheck = true
			}
			// Track legacy Check vs modern ConfigStateChecks separately
			if step.HasCheck {
				report.HasCheck = true
			}
			if step.HasConfigStateChecks {
				report.HasConfigStateChecks = true
			}
		}
	}

	// Consolidate test files into a single string
	if len(testFiles) == 1 {
		for f := range testFiles {
			report.TestFile = f
		}
	} else if len(testFiles) > 1 {
		// Multiple test files - show count
		report.TestFile = fmt.Sprintf("(%d files)", len(testFiles))
	} else {
		report.TestFile = "-"
	}

	return report
}

// buildActionReport creates a report for an action, focusing on action-relevant test patterns
func buildActionReport(reg *registry.ResourceRegistry, info *registry.ResourceInfo) ResourceReport {
	key := info.Kind.String() + ":" + info.Name
	tests := reg.GetResourceTests(key)

	report := ResourceReport{
		Name:      info.Name,
		File:      filepath.Base(info.FilePath),
		TestCount: len(tests),
	}

	// Track unique test files
	testFiles := make(map[string]bool)

	for _, t := range tests {
		testFile := filepath.Base(t.FilePath)
		testFiles[testFile] = true
		report.Tests = append(report.Tests, TestReport{
			Name:      t.Name,
			File:      testFile,
			MatchType: t.MatchType.String(),
		})
		if t.HasPreCheck {
			report.HasPreCheck = true
		}
		for _, step := range t.TestSteps {
			if step.IsRealUpdateStep() {
				report.HasUpdateTest = true
			}
			if step.ExpectError {
				report.HasExpectError = true
			}
			// Track legacy Check vs modern ConfigStateChecks separately
			if step.HasCheck {
				report.HasCheck = true
			}
			if step.HasConfigStateChecks {
				report.HasConfigStateChecks = true
			}
		}
	}

	// Consolidate test files into a single string
	if len(testFiles) == 1 {
		for f := range testFiles {
			report.TestFile = f
		}
	} else if len(testFiles) > 1 {
		report.TestFile = fmt.Sprintf("(%d files)", len(testFiles))
	} else {
		report.TestFile = "-"
	}

	return report
}

func outputReportJSON(reg *registry.ResourceRegistry, resources, dataSources, actions []*registry.ResourceInfo, orphans []*registry.TestFunctionInfo) {
	data := ReportData{}

	// Summary counts come from the single shared computation so the JSON and
	// table renderers cannot disagree.
	summary := tfanalysis.NewCoverageCalculator(reg).Summarize()
	data.Summary = ReportSummary{
		TotalResources:      summary.TotalResources,
		UntestedResources:   summary.UntestedResources,
		MissingCheckDestroy: summary.MissingCheckDestroy,
		TotalDataSources:    summary.TotalDataSources,
		UntestedDataSources: summary.UntestedDataSources,
		TotalActions:        summary.TotalActions,
		UntestedActions:     summary.UntestedActions,
		MissingStateChecks:  summary.MissingStateChecks,
		OrphanTests:         len(orphans),
	}

	// Build per-item detail reports.
	for _, info := range resources {
		data.Resources = append(data.Resources, buildResourceReport(reg, info))
	}
	for _, info := range dataSources {
		data.DataSources = append(data.DataSources, buildResourceReport(reg, info))
	}
	for _, info := range actions {
		data.Actions = append(data.Actions, buildActionReport(reg, info))
	}

	// Build orphan reports
	for _, fn := range orphans {
		data.Orphans = append(data.Orphans, OrphanReport{
			Name:              fn.Name,
			File:              filepath.Base(fn.FilePath),
			InferredResources: fn.InferredResources,
		})
	}
	data.Summary.OrphanTests = len(orphans)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

func outputReportTable(reg *registry.ResourceRegistry, resources, dataSources, actions []*registry.ResourceInfo, orphans []*registry.TestFunctionInfo) {
	// Calculate summary stats via the single shared computation so the table
	// and JSON renderers cannot disagree.
	summary := tfanalysis.NewCoverageCalculator(reg).Summarize()
	untestedResources := summary.UntestedResources
	untestedDataSources := summary.UntestedDataSources
	untestedActions := summary.UntestedActions
	missingCheckDestroy := summary.MissingCheckDestroy
	missingStateCheck := summary.MissingStateChecks

	// Print header
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                        TERRAFORM PROVIDER TEST COVERAGE REPORT                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")

	// Summary table
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ SUMMARY                                                                         │")
	fmt.Println("├──────────────┬───────┬──────────┬─────────────────────────────────────────────────┤")
	fmt.Println("│ Category     │ Total │ Untested │ Issues                                          │")
	fmt.Println("├──────────────┼───────┼──────────┼─────────────────────────────────────────────────┤")
	fmt.Printf("│ Resources    │ %5d │ %8d │ %d without CheckDestroy                          │\n", len(resources), untestedResources, missingCheckDestroy)
	fmt.Printf("│ Data Sources │ %5d │ %8d │ -                                               │\n", len(dataSources), untestedDataSources)
	fmt.Printf("│ Actions      │ %5d │ %8d │ %d without Check func                            │\n", len(actions), untestedActions, missingStateCheck)
	fmt.Printf("│ Orphan Tests │ %5d │        - │ -                                               │\n", len(orphans))
	fmt.Println("└──────────────┴───────┴──────────┴─────────────────────────────────────────────────┘")

	// Resources table
	if len(resources) > 0 {
		fmt.Println()
		fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
		fmt.Println("│ RESOURCES                                                                       │")
		fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tTESTS\tUpdate\tImportState\tCheckDestroy\tExpectError\tCheck\tConfigStateChecks\tPlanChecks\tFILE\tTEST FILE")
		fmt.Fprintln(w, "  ────\t─────\t──────\t───────────\t────────────\t───────────\t─────\t─────────────────\t──────────\t────\t─────────")
		for _, info := range resources {
			report := buildResourceReport(reg, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasUpdateTest),
				checkMark(report.HasImportTest),
				checkMark(report.HasCheckDestroy),
				checkMark(report.HasExpectError),
				checkMark(report.HasCheck),
				checkMark(report.HasConfigStateChecks),
				checkMark(report.HasPlanCheck),
				report.File,
				report.TestFile,
			)
		}
		w.Flush()
	}

	// Data Sources table
	if len(dataSources) > 0 {
		fmt.Println()
		fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
		fmt.Println("│ DATA SOURCES                                                                    │")
		fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tTESTS\tCheck\tConfigStateChecks\tFILE\tTEST FILE")
		fmt.Fprintln(w, "  ────\t─────\t─────\t─────────────────\t────\t─────────")
		for _, info := range dataSources {
			report := buildResourceReport(reg, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasCheck),
				checkMark(report.HasConfigStateChecks),
				report.File,
				report.TestFile,
			)
		}
		w.Flush()
	}

	// Actions table
	if len(actions) > 0 {
		fmt.Println()
		fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
		fmt.Println("│ ACTIONS                                                                         │")
		fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tTESTS\tUpdate\tExpectError\tCheck\tConfigStateChecks\tPreCheck\tFILE\tTEST FILE")
		fmt.Fprintln(w, "  ────\t─────\t──────\t───────────\t─────\t─────────────────\t────────\t────\t─────────")
		for _, info := range actions {
			report := buildActionReport(reg, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasUpdateTest),
				checkMark(report.HasExpectError),
				checkMark(report.HasCheck),
				checkMark(report.HasConfigStateChecks),
				checkMark(report.HasPreCheck),
				report.File,
				report.TestFile,
			)
		}
		w.Flush()
	}

	// Orphans table
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ ORPHAN TESTS                                                                    │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
	if len(orphans) == 0 {
		fmt.Println("  ✓ All test functions are associated with resources!")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  TEST FUNCTION\tFILE\tINFERRED RESOURCES")
		fmt.Fprintln(w, "  ─────────────\t────\t──────────────────")
		for _, fn := range orphans {
			inferred := "-"
			if len(fn.InferredResources) > 0 {
				inferred = strings.Join(fn.InferredResources, ", ")
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\n", fn.Name, filepath.Base(fn.FilePath), inferred)
		}
		w.Flush()
	}

	// Test details table
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ TEST ASSOCIATIONS                                                               │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  RESOURCE\tKIND\tTEST FUNCTION\tMATCH TYPE")
	fmt.Fprintln(w, "  ────────\t────\t─────────────\t──────────")

	// Combine all definitions
	type defWithKind struct {
		info *registry.ResourceInfo
		kind string
	}
	var allDefs []defWithKind
	for _, info := range resources {
		allDefs = append(allDefs, defWithKind{info, "resource"})
	}
	for _, info := range dataSources {
		allDefs = append(allDefs, defWithKind{info, "data"})
	}
	for _, info := range actions {
		allDefs = append(allDefs, defWithKind{info, "action"})
	}

	for _, def := range allDefs {
		key := def.info.Kind.String() + ":" + def.info.Name
		tests := reg.GetResourceTests(key)
		if len(tests) == 0 {
			fmt.Fprintf(w, "  %s\t%s\t-\t-\n", def.info.Name, def.kind)
		} else {
			for i, t := range tests {
				name := def.info.Name
				kind := def.kind
				if i > 0 {
					name = ""
					kind = ""
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", name, kind, t.Name, t.MatchType.String())
			}
		}
	}
	w.Flush()
	fmt.Println()

	printUnregisteredSection(resources, dataSources, actions)
}

// printUnregisteredSection prints a section listing resources that are defined
// in source but not registered in the provider's aggregator methods. It prints
// nothing when there are none, so reports for providers without the issue (or
// without the aggregator pattern) are unchanged.
func printUnregisteredSection(resources, dataSources, actions []*registry.ResourceInfo) {
	type row struct{ name, kind string }
	var rows []row
	for _, info := range resources {
		if info.Unregistered {
			rows = append(rows, row{info.Name, "resource"})
		}
	}
	for _, info := range dataSources {
		if info.Unregistered {
			rows = append(rows, row{info.Name, "data source"})
		}
	}
	for _, info := range actions {
		if info.Unregistered {
			rows = append(rows, row{info.Name, "action"})
		}
	}
	if len(rows) == 0 {
		return
	}

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ DEFINED BUT NOT REGISTERED                                                       │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println("  These types have a constructor and schema but are not listed in the provider's")
	fmt.Println("  Resources()/DataSources()/Actions() aggregator, so they do not ship:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tKIND")
	fmt.Fprintln(w, "  ────\t────")
	for _, r := range rows {
		fmt.Fprintf(w, "  %s\t%s\n", r.name, r.kind)
	}
	w.Flush()
	fmt.Println()
}

func checkMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}

// findAllGoPackageDirs recursively finds all directories containing Go files
func findAllGoPackageDirs(root string) []string {
	var dirs []string
	seen := make(map[string]bool)

	// Directories to exclude from scanning
	excludeDirs := map[string]bool{
		"vendor":       true,
		"testdata":     true,
		".git":         true,
		".github":      true,
		"node_modules": true,
		".terraform":   true,
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip directories we can't access
		}

		if d.IsDir() {
			// Skip excluded directories
			if excludeDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if this is a Go file
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		// Add the directory containing this Go file
		dir := filepath.Dir(path)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
		return nil
	})

	if err != nil {
		return nil
	}

	// Sort directories for consistent output
	sort.Strings(dirs)
	return dirs
}
