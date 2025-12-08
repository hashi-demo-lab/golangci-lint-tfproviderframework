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

	tfprovidertest "github.com/example/tfprovidertest"
	"golang.org/x/tools/go/analysis"
)

// MatchInfo represents a resource-test association for diagnostic output
type MatchInfo struct {
	ResourceName string  `json:"resource_name"`
	TestFunction string  `json:"test_function"`
	TestFile     string  `json:"test_file"`
	Confidence   float64 `json:"confidence"`
	MatchType    string  `json:"match_type"`
}

func main() {
	// Basic flags
	providerPath := flag.String("provider", "", "Path to the Terraform provider directory")
	verbose := flag.Bool("verbose", false, "Enable verbose output")

	// Diagnostic flags
	showMatches := flag.Bool("show-matches", false, "Show all resource -> test function associations")
	showUnmatched := flag.Bool("show-unmatched", false, "Show test functions without resource association")
	showOrphaned := flag.Bool("show-orphaned", false, "Show resources without any test coverage")
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

	// Find provider code directory
	providerCodeDir := findProviderCodeDir(*providerPath)
	if providerCodeDir == "" {
		fmt.Printf("Error: Could not find provider code directory in %s\n", *providerPath)
		fmt.Println("\nTried the following locations:")
		fmt.Println("  - internal/provider")
		fmt.Println("  - internal")
		fmt.Println("  - <provider-name> (extracted from directory name)")
		os.Exit(1)
	}

	fmt.Printf("Analyzing provider at: %s\n\n", providerCodeDir)

	// Build settings from flags
	settings := tfprovidertest.DefaultSettings()
	settings.Verbose = *verbose
	settings.ShowMatchConfidence = *showMatches
	settings.ShowUnmatchedTests = *showUnmatched
	settings.ShowOrphanedResources = *showOrphaned
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

	// Parse all Go files in the provider directory
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, providerCodeDir, nil, parser.ParseComments)
	if err != nil {
		fmt.Printf("Error parsing provider directory: %v\n", err)
		os.Exit(1)
	}

	if len(pkgs) == 0 {
		fmt.Printf("Error: No Go packages found in %s\n", providerCodeDir)
		os.Exit(1)
	}

	// Collect all files from all packages
	var allFiles []*ast.File
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			allFiles = append(allFiles, file)
		}
	}

	// Handle report command - comprehensive coverage report
	if *showReport {
		runReport(fset, allFiles, settings, *outputFormat)
		return
	}

	// Handle diagnostic commands
	if *showMatches || *showUnmatched || *showOrphaned {
		runDiagnostics(fset, allFiles, settings, *outputFormat, *showMatches, *showUnmatched, *showOrphaned)
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
	fmt.Println("Diagnostic Options:")
	fmt.Println("  -report")
	fmt.Println("        Show comprehensive coverage report with table views")
	fmt.Println("  -show-matches")
	fmt.Println("        Show all resource -> test function associations")
	fmt.Println("  -show-unmatched")
	fmt.Println("        Show test functions without resource association")
	fmt.Println("  -show-orphaned")
	fmt.Println("        Show resources without any test coverage")
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
	fmt.Println("  # Show all resource-test associations in table format")
	fmt.Println("  validate -provider ./provider -show-matches -format table")
	fmt.Println()
	fmt.Println("  # Find unmatched tests with verbose output")
	fmt.Println("  validate -provider ./provider -show-unmatched -verbose")
	fmt.Println()
	fmt.Println("  # Use function-only matching with custom threshold")
	fmt.Println("  validate -provider ./provider -match-strategy function -confidence-threshold 0.8")
	fmt.Println()
	fmt.Println("  # Export all matches as JSON")
	fmt.Println("  validate -provider ./provider -show-matches -format json > matches.json")
}

// validateSettings performs validation on the settings configuration
func validateSettings(settings tfprovidertest.Settings) error {
	// Validate confidence threshold range
	if settings.FuzzyMatchThreshold < 0.0 || settings.FuzzyMatchThreshold > 1.0 {
		return fmt.Errorf("confidence-threshold must be between 0.0 and 1.0, got %f", settings.FuzzyMatchThreshold)
	}

	// Function name matching and file-based matching always run (no validation needed)
	return nil
}

// runDiagnostics handles diagnostic output modes
func runDiagnostics(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings, format string, showMatches, showUnmatched, showOrphaned bool) {
	// Validate output format
	if format != "text" && format != "json" && format != "table" {
		fmt.Printf("Error: Invalid format '%s'. Must be one of: text, json, table\n", format)
		os.Exit(1)
	}

	// TODO: Build registry and perform resource-test linking
	// This requires exposing BuildRegistry or creating a diagnostic-specific function
	// For now, output placeholder information

	if showMatches {
		fmt.Println("=== Resource -> Test Function Associations ===")
		fmt.Println()
		// TODO: Get actual matches from registry
		// matches := getResourceTestMatches(fset, files, settings)
		// outputMatches(matches, format)
		fmt.Println("  (Diagnostic output requires registry access - implementation pending)")
		fmt.Println("  This will show all detected resource -> test function mappings")
		fmt.Println("  including confidence scores and match types.")
		fmt.Println()
	}

	if showUnmatched {
		fmt.Println("=== Unmatched Test Functions ===")
		fmt.Println()
		// TODO: Get unmatched tests from registry
		// unmatched := registry.GetUnmatchedTestFunctions()
		// outputUnmatched(unmatched, format)
		fmt.Println("  (Diagnostic output requires registry access - implementation pending)")
		fmt.Println("  This will show test functions that could not be associated")
		fmt.Println("  with any known resource definition.")
		fmt.Println()
	}

	if showOrphaned {
		fmt.Println("=== Orphaned Resources (No Tests) ===")
		fmt.Println()
		// TODO: Get orphaned resources from registry
		// orphaned := registry.GetUntestedResources()
		// outputOrphaned(orphaned, format)
		fmt.Println("  (Diagnostic output requires registry access - implementation pending)")
		fmt.Println("  This will show resources that have no associated test coverage.")
		fmt.Println()
	}
}

// outputMatchesText outputs matches in human-readable text format
//
//nolint:unused // Prepared for future diagnostic output implementation
func outputMatchesText(matches []MatchInfo) {
	for _, m := range matches {
		fmt.Printf("  %s -> %s (%.0f%%, %s)\n", m.ResourceName, m.TestFunction, m.Confidence*100, m.MatchType)
		if m.TestFile != "" {
			fmt.Printf("    File: %s\n", m.TestFile)
		}
	}
}

// outputMatchesTable outputs matches in a formatted table
//
//nolint:unused // Prepared for future diagnostic output implementation
func outputMatchesTable(matches []MatchInfo) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RESOURCE\tTEST FUNCTION\tCONFIDENCE\tMATCH TYPE\tTEST FILE")
	fmt.Fprintln(w, "--------\t-------------\t----------\t----------\t---------")
	for _, m := range matches {
		fmt.Fprintf(w, "%s\t%s\t%.0f%%\t%s\t%s\n", m.ResourceName, m.TestFunction, m.Confidence*100, m.MatchType, m.TestFile)
	}
	w.Flush()
}

// outputMatchesJSON outputs matches as formatted JSON
//
//nolint:unused // Prepared for future diagnostic output implementation
func outputMatchesJSON(matches []MatchInfo) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(matches); err != nil {
		fmt.Printf("Error encoding JSON: %v\n", err)
	}
}

// outputMatches routes to the appropriate output formatter
//
//nolint:unused // Prepared for future diagnostic output implementation
func outputMatches(matches []MatchInfo, format string) {
	switch format {
	case "json":
		outputMatchesJSON(matches)
	case "table":
		outputMatchesTable(matches)
	default:
		outputMatchesText(matches)
	}
}

// runAnalyzers executes the standard analysis workflow
func runAnalyzers(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings) {
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

// buildRegistryFromFiles creates a registry from parsed AST files
func buildRegistryFromFiles(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings) *tfprovidertest.ResourceRegistry {
	registry := tfprovidertest.NewResourceRegistry()
	config := tfprovidertest.DefaultParserConfig()

	for _, file := range files {
		filePath := fset.Position(file.Pos()).Filename

		// Apply exclusion settings
		if settings.ExcludeBaseClasses && tfprovidertest.IsBaseClassFile(filePath) {
			continue
		}
		if settings.ExcludeSweeperFiles && tfprovidertest.IsSweeperFile(filePath) {
			continue
		}
		if settings.ExcludeMigrationFiles && tfprovidertest.IsMigrationFile(filePath) {
			continue
		}

		if strings.HasSuffix(filePath, "_test.go") {
			testInfo := tfprovidertest.ParseTestFileWithConfig(file, fset, filePath, config)
			if testInfo == nil {
				continue
			}
			for i := range testInfo.TestFunctions {
				registry.RegisterTestFunction(&testInfo.TestFunctions[i])
			}
		} else {
			resources := tfprovidertest.ParseResources(file, fset, filePath)
			for _, resource := range resources {
				registry.RegisterResource(resource)
			}
		}
	}

	// Run linking
	linker := tfprovidertest.NewLinker(registry, settings)
	linker.LinkTestsToResources()

	return registry
}

// runReport generates a comprehensive coverage report with table views
func runReport(fset *token.FileSet, files []*ast.File, settings tfprovidertest.Settings, format string) {
	registry := buildRegistryFromFiles(fset, files, settings)
	allDefs := registry.GetAllDefinitions()

	// Group definitions by kind
	var resources, dataSources, actions []*tfprovidertest.ResourceInfo
	for _, info := range allDefs {
		switch info.Kind {
		case tfprovidertest.KindResource:
			resources = append(resources, info)
		case tfprovidertest.KindDataSource:
			dataSources = append(dataSources, info)
		case tfprovidertest.KindAction:
			actions = append(actions, info)
		}
	}

	// Sort each slice by name
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	sort.Slice(dataSources, func(i, j int) bool { return dataSources[i].Name < dataSources[j].Name })
	sort.Slice(actions, func(i, j int) bool { return actions[i].Name < actions[j].Name })

	orphans := registry.GetUnmatchedTestFunctions()

	switch format {
	case "json":
		outputReportJSON(registry, resources, dataSources, actions, orphans)
	case "table":
		outputReportTable(registry, resources, dataSources, actions, orphans)
	default:
		outputReportTable(registry, resources, dataSources, actions, orphans)
	}
}

// ReportData holds all data for JSON output
type ReportData struct {
	Summary     ReportSummary      `json:"summary"`
	Resources   []ResourceReport   `json:"resources"`
	DataSources []ResourceReport   `json:"data_sources"`
	Actions     []ResourceReport   `json:"actions"`
	Orphans     []OrphanReport     `json:"orphan_tests"`
}

type ReportSummary struct {
	TotalResources          int `json:"total_resources"`
	UntestedResources       int `json:"untested_resources"`
	TotalDataSources        int `json:"total_data_sources"`
	UntestedDataSources     int `json:"untested_data_sources"`
	TotalActions            int `json:"total_actions"`
	UntestedActions         int `json:"untested_actions"`
	OrphanTests             int `json:"orphan_tests"`
	MissingCheckDestroy     int `json:"missing_check_destroy"`
	MissingStateChecks      int `json:"missing_state_checks"`
}

type ResourceReport struct {
	Name            string       `json:"name"`
	File            string       `json:"file"`
	TestCount       int          `json:"test_count"`
	HasCheckDestroy bool         `json:"has_check_destroy"`
	HasStateCheck   bool         `json:"has_state_check"`
	HasPlanCheck    bool         `json:"has_plan_check"`
	HasImportTest   bool         `json:"has_import_test"`
	HasUpdateTest   bool         `json:"has_update_test"`
	HasExpectError  bool         `json:"has_expect_error"`
	Tests           []TestReport `json:"tests"`
}

type TestReport struct {
	Name      string `json:"name"`
	MatchType string `json:"match_type"`
}

type OrphanReport struct {
	Name              string   `json:"name"`
	File              string   `json:"file"`
	InferredResources []string `json:"inferred_resources,omitempty"`
}

func buildResourceReport(registry *tfprovidertest.ResourceRegistry, info *tfprovidertest.ResourceInfo) ResourceReport {
	key := info.Kind.String() + ":" + info.Name
	tests := registry.GetResourceTests(key)

	report := ResourceReport{
		Name:      info.Name,
		File:      filepath.Base(info.FilePath),
		TestCount: len(tests),
	}

	for _, t := range tests {
		report.Tests = append(report.Tests, TestReport{
			Name:      t.Name,
			MatchType: t.MatchType.String(),
		})
		if t.HasCheckDestroy {
			report.HasCheckDestroy = true
		}
		if t.HasStateOrPlanCheck() {
			report.HasStateCheck = true
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
		}
	}

	return report
}

func outputReportJSON(registry *tfprovidertest.ResourceRegistry, resources, dataSources, actions []*tfprovidertest.ResourceInfo, orphans []*tfprovidertest.TestFunctionInfo) {
	data := ReportData{}

	// Build resource reports
	for _, info := range resources {
		report := buildResourceReport(registry, info)
		data.Resources = append(data.Resources, report)
		if report.TestCount == 0 {
			data.Summary.UntestedResources++
		} else if !report.HasCheckDestroy {
			data.Summary.MissingCheckDestroy++
		}
	}
	data.Summary.TotalResources = len(resources)

	// Build data source reports
	for _, info := range dataSources {
		report := buildResourceReport(registry, info)
		data.DataSources = append(data.DataSources, report)
		if report.TestCount == 0 {
			data.Summary.UntestedDataSources++
		}
	}
	data.Summary.TotalDataSources = len(dataSources)

	// Build action reports
	for _, info := range actions {
		report := buildResourceReport(registry, info)
		data.Actions = append(data.Actions, report)
		if report.TestCount == 0 {
			data.Summary.UntestedActions++
		} else if !report.HasStateCheck {
			data.Summary.MissingStateChecks++
		}
	}
	data.Summary.TotalActions = len(actions)

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

func outputReportTable(registry *tfprovidertest.ResourceRegistry, resources, dataSources, actions []*tfprovidertest.ResourceInfo, orphans []*tfprovidertest.TestFunctionInfo) {
	// Calculate summary stats first
	var untestedResources, untestedDataSources, untestedActions int
	var missingCheckDestroy, missingStateCheck int

	for _, info := range resources {
		key := tfprovidertest.KindResource.String() + ":" + info.Name
		tests := registry.GetResourceTests(key)
		if len(tests) == 0 {
			untestedResources++
		} else {
			hasCheckDestroy := false
			for _, t := range tests {
				if t.HasCheckDestroy {
					hasCheckDestroy = true
					break
				}
			}
			if !hasCheckDestroy {
				missingCheckDestroy++
			}
		}
	}

	for _, info := range dataSources {
		key := tfprovidertest.KindDataSource.String() + ":" + info.Name
		tests := registry.GetResourceTests(key)
		if len(tests) == 0 {
			untestedDataSources++
		}
	}

	for _, info := range actions {
		key := tfprovidertest.KindAction.String() + ":" + info.Name
		tests := registry.GetResourceTests(key)
		if len(tests) == 0 {
			untestedActions++
		} else {
			hasStateCheck := false
			for _, t := range tests {
				if t.HasStateOrPlanCheck() {
					hasStateCheck = true
					break
				}
			}
			if !hasStateCheck {
				missingStateCheck++
			}
		}
	}

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
		fmt.Fprintln(w, "  NAME\tTESTS\tSteps>1\tImportState\tCheckDestroy\tExpectError\tCheck\tPlanChecks\tFILE")
		fmt.Fprintln(w, "  ────\t─────\t───────\t───────────\t────────────\t───────────\t─────\t──────────\t────")
		for _, info := range resources {
			report := buildResourceReport(registry, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasUpdateTest),
				checkMark(report.HasImportTest),
				checkMark(report.HasCheckDestroy),
				checkMark(report.HasExpectError),
				checkMark(report.HasStateCheck),
				checkMark(report.HasPlanCheck),
				report.File,
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
		fmt.Fprintln(w, "  NAME\tTESTS\tCheck\tFILE")
		fmt.Fprintln(w, "  ────\t─────\t─────\t────")
		for _, info := range dataSources {
			report := buildResourceReport(registry, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasStateCheck),
				report.File,
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
		fmt.Fprintln(w, "  NAME\tTESTS\tCheck\tFILE")
		fmt.Fprintln(w, "  ────\t─────\t─────\t────")
		for _, info := range actions {
			report := buildResourceReport(registry, info)
			fmt.Fprintf(w, "  %s\t%d\t%s\t%s\n",
				info.Name,
				report.TestCount,
				checkMark(report.HasStateCheck),
				report.File,
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
		info *tfprovidertest.ResourceInfo
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
		tests := registry.GetResourceTests(key)
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
}

func checkMark(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}
