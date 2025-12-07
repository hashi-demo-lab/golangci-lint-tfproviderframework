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
	switch *matchStrategy {
	case "function":
		settings.EnableFunctionMatching = true
		settings.EnableFileBasedMatching = false
		settings.EnableFuzzyMatching = false
	case "file":
		settings.EnableFunctionMatching = false
		settings.EnableFileBasedMatching = true
		settings.EnableFuzzyMatching = false
	case "fuzzy":
		settings.EnableFunctionMatching = true
		settings.EnableFileBasedMatching = true
		settings.EnableFuzzyMatching = true
	case "all":
		settings.EnableFunctionMatching = true
		settings.EnableFileBasedMatching = true
		settings.EnableFuzzyMatching = false // Still disabled by default in "all" mode
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

	// Validate that at least one matching strategy is enabled
	if !settings.EnableFunctionMatching && !settings.EnableFileBasedMatching && !settings.EnableFuzzyMatching {
		return fmt.Errorf("at least one matching strategy must be enabled")
	}

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
func outputMatchesText(matches []MatchInfo) {
	for _, m := range matches {
		fmt.Printf("  %s -> %s (%.0f%%, %s)\n", m.ResourceName, m.TestFunction, m.Confidence*100, m.MatchType)
		if m.TestFile != "" {
			fmt.Printf("    File: %s\n", m.TestFile)
		}
	}
}

// outputMatchesTable outputs matches in a formatted table
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
func outputMatchesJSON(matches []MatchInfo) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(matches); err != nil {
		fmt.Printf("Error encoding JSON: %v\n", err)
	}
}

// outputMatches routes to the appropriate output formatter
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
		"Verbose":                 settings.Verbose,
		"EnableBasicTest":         settings.EnableBasicTest,
		"EnableUpdateTest":        settings.EnableUpdateTest,
		"EnableImportTest":        settings.EnableImportTest,
		"EnableErrorTest":         settings.EnableErrorTest,
		"EnableStateCheck":        settings.EnableStateCheck,
		"EnableFunctionMatching":  settings.EnableFunctionMatching,
		"EnableFileBasedMatching": settings.EnableFileBasedMatching,
		"EnableFuzzyMatching":     settings.EnableFuzzyMatching,
		"FuzzyMatchThreshold":     settings.FuzzyMatchThreshold,
		"ProviderPrefix":          settings.ProviderPrefix,
		"ShowMatchConfidence":     settings.ShowMatchConfidence,
		"ShowUnmatchedTests":      settings.ShowUnmatchedTests,
		"ShowOrphanedResources":   settings.ShowOrphanedResources,
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
