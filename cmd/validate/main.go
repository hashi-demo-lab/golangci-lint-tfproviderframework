// Command validate runs the tfprovidertest analyzers against a Terraform provider.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	tfprovidertest "github.com/example/tfprovidertest"
	"golang.org/x/tools/go/analysis"
)

func main() {
	providerPath := flag.String("provider", "", "Path to the Terraform provider directory")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	if *providerPath == "" {
		fmt.Println("Usage: validate -provider <path> [-verbose]")
		fmt.Println("\nOptions:")
		fmt.Println("  -provider string")
		fmt.Println("        Path to the Terraform provider directory")
		fmt.Println("  -verbose")
		fmt.Println("        Enable verbose diagnostic output")
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

	fmt.Printf("Analyzing provider at: %s\n", providerCodeDir)
	fmt.Println()

	// Create plugin with settings
	settings := map[string]interface{}{
		"Verbose": *verbose,
	}
	plugin, err := tfprovidertest.New(settings)
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

	// Create a simple analysis pass for each analyzer
	totalIssues := 0
	for _, analyzer := range analyzers {
		fmt.Printf("Running %s...\n", analyzer.Name)
		
		pass := &analysis.Pass{
			Analyzer: analyzer,
			Fset:     fset,
			Files:    allFiles,
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
		fmt.Println("✅ No issues found - all resources have proper test coverage!")
	} else {
		fmt.Printf("⚠️  Found %d issue(s)\n", totalIssues)
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
