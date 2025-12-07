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
	"time"
)

func main() {
	providerPath := flag.String("provider", "", "Path to the Terraform provider directory")
	flag.Parse()

	if *providerPath == "" {
		fmt.Println("Usage: validate -provider <path>")
		os.Exit(1)
	}

	start := time.Now()
	results := validateProvider(*providerPath)
	elapsed := time.Since(start)

	fmt.Printf("\n=== Validation Results for %s ===\n", filepath.Base(*providerPath))
	fmt.Printf("Time: %v\n\n", elapsed)

	fmt.Printf("Resources found: %d\n", len(results.Resources))
	fmt.Printf("Data sources found: %d\n", len(results.DataSources))
	fmt.Printf("Test files found: %d\n", len(results.TestFiles))
	fmt.Printf("\n")

	// Report issues
	if len(results.Issues) == 0 {
		fmt.Println("✅ No issues found - all resources have proper test coverage!")
	} else {
		fmt.Printf("⚠️  Found %d potential issues:\n\n", len(results.Issues))
		for i, issue := range results.Issues {
			fmt.Printf("%d. [%s] %s\n", i+1, issue.Analyzer, issue.Message)
			fmt.Printf("   File: %s\n\n", issue.File)
		}
	}

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Printf("Resources with tests: %d/%d\n", results.ResourcesWithTests, len(results.Resources))
	fmt.Printf("Data sources with tests: %d/%d\n", results.DataSourcesWithTests, len(results.DataSources))
}

type ValidationResults struct {
	Resources             []string
	DataSources           []string
	TestFiles             []string
	Issues                []Issue
	ResourcesWithTests    int
	DataSourcesWithTests  int
}

type Issue struct {
	Analyzer string
	Message  string
	File     string
	Line     int
}

func validateProvider(providerPath string) ValidationResults {
	results := ValidationResults{}

	// Find internal/provider directory
	internalProvider := filepath.Join(providerPath, "internal", "provider")
	if _, err := os.Stat(internalProvider); os.IsNotExist(err) {
		// Try just "internal"
		internalProvider = filepath.Join(providerPath, "internal")
	}

	// Parse all Go files
	fset := token.NewFileSet()
	resourceFiles := make(map[string]bool)
	testFiles := make(map[string]bool)

	err := filepath.Walk(internalProvider, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			testFiles[path] = true
			results.TestFiles = append(results.TestFiles, path)
		} else {
			resourceFiles[path] = true
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		return results
	}

	// Parse resource files to find resources and data sources
	resourceMap := make(map[string]string)   // name -> file
	dataSourceMap := make(map[string]string) // name -> file
	resourceTests := make(map[string]bool)   // which resources have tests

	for path := range resourceFiles {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}

		// Look for Schema methods that indicate resources/data sources
		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Schema" {
				return true
			}

			recvType := getReceiverType(funcDecl.Recv)
			if strings.HasSuffix(recvType, "Resource") && !strings.HasSuffix(recvType, "DataSource") {
				name := extractName(recvType, "Resource")
				if name != "" {
					results.Resources = append(results.Resources, name)
					resourceMap[name] = path
				}
			} else if strings.HasSuffix(recvType, "DataSource") {
				name := extractName(recvType, "DataSource")
				if name != "" {
					results.DataSources = append(results.DataSources, name)
					dataSourceMap[name] = path
				}
			}
			return true
		})
	}

	// Parse test files to find which resources have tests
	for path := range testFiles {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(funcDecl.Name.Name, "TestAcc") {
				return true
			}

			// Extract resource name from test function
			testName := funcDecl.Name.Name
			if strings.HasPrefix(testName, "TestAccDataSource") {
				// Data source test
				parts := strings.SplitN(strings.TrimPrefix(testName, "TestAccDataSource"), "_", 2)
				if len(parts) > 0 {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			} else if strings.HasPrefix(testName, "TestAcc") {
				// Resource test - try various patterns
				name := strings.TrimPrefix(testName, "TestAcc")
				parts := strings.SplitN(name, "_", 2)
				if len(parts) > 0 {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			}
			return true
		})
	}

	// Check for untested resources
	for name, file := range resourceMap {
		if resourceTests[name] {
			results.ResourcesWithTests++
		} else {
			results.Issues = append(results.Issues, Issue{
				Analyzer: "tfprovider-resource-basic-test",
				Message:  fmt.Sprintf("Resource '%s' has no acceptance test", name),
				File:     file,
			})
		}
	}

	for name, file := range dataSourceMap {
		if resourceTests[name] {
			results.DataSourcesWithTests++
		} else {
			results.Issues = append(results.Issues, Issue{
				Analyzer: "tfprovider-datasource-basic-test",
				Message:  fmt.Sprintf("Data source '%s' has no acceptance test", name),
				File:     file,
			})
		}
	}

	return results
}

func getReceiverType(recv *ast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func extractName(typeName, suffix string) string {
	name := strings.TrimSuffix(typeName, suffix)
	return toSnakeCase(name)
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
