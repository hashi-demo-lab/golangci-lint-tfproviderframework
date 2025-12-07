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
	Resources            []string
	DataSources          []string
	TestFiles            []string
	Issues               []Issue
	ResourcesWithTests   int
	DataSourcesWithTests int
}

type Issue struct {
	Analyzer string
	Message  string
	File     string
	Line     int
}

func validateProvider(providerPath string) ValidationResults {
	results := ValidationResults{}

	// Find provider code directory - try multiple common patterns
	var internalProvider string
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
			internalProvider = path
			break
		}
	}

	if internalProvider == "" {
		fmt.Printf("Warning: Could not find provider code directory in %s\n", providerPath)
		return results
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

		baseName := filepath.Base(path)

		// Try file naming convention first (Plugin Framework pattern)
		if strings.HasPrefix(baseName, "resource_") && strings.HasSuffix(baseName, ".go") {
			// Skip helper/utility files that aren't actual resources
			if strings.Contains(baseName, "stateupgrader") ||
			   strings.Contains(baseName, "migrate") ||
			   strings.Contains(baseName, "helper") {
				continue
			}
			// Extract resource name from filename: resource_helm_release.go -> helm_release
			name := strings.TrimPrefix(baseName, "resource_")
			name = strings.TrimSuffix(name, ".go")
			results.Resources = append(results.Resources, name)
			resourceMap[name] = path
			continue
		}

		// Also try reversed naming: group_resource.go -> group
		if strings.HasSuffix(baseName, "_resource.go") {
			// Skip base classes
			if strings.HasPrefix(baseName, "base") {
				continue
			}
			// Extract resource name from filename: group_resource.go -> group
			name := strings.TrimSuffix(baseName, "_resource.go")
			results.Resources = append(results.Resources, name)
			resourceMap[name] = path
			continue
		}

		if strings.HasPrefix(baseName, "data_source_") && strings.HasSuffix(baseName, ".go") {
			// Extract data source name from filename: data_source_helm_template.go -> helm_template
			name := strings.TrimPrefix(baseName, "data_source_")
			name = strings.TrimSuffix(name, ".go")
			results.DataSources = append(results.DataSources, name)
			dataSourceMap[name] = path
			continue
		}

		if strings.HasPrefix(baseName, "data_") && strings.HasSuffix(baseName, ".go") {
			// Extract data source name from filename: data_helm_template.go -> helm_template
			name := strings.TrimPrefix(baseName, "data_")
			name = strings.TrimSuffix(name, ".go")
			results.DataSources = append(results.DataSources, name)
			dataSourceMap[name] = path
			continue
		}

		// Also try reversed naming: inventory_data_source.go or group_datasource.go
		if strings.HasSuffix(baseName, "_data_source.go") || strings.HasSuffix(baseName, "_datasource.go") {
			// Skip base classes
			if strings.HasPrefix(baseName, "base") {
				continue
			}
			// Extract data source name: inventory_data_source.go -> inventory
			name := strings.TrimSuffix(baseName, "_data_source.go")
			if strings.HasSuffix(baseName, "_datasource.go") {
				name = strings.TrimSuffix(baseName, "_datasource.go")
			}
			results.DataSources = append(results.DataSources, name)
			dataSourceMap[name] = path
			continue
		}

		if strings.HasPrefix(baseName, "ephemeral_") && strings.HasSuffix(baseName, ".go") {
			// Skip test files
			if strings.HasSuffix(baseName, "_test.go") {
				continue
			}
			// Extract ephemeral resource name from filename: ephemeral_private_key.go -> private_key_ephemeral
			name := strings.TrimPrefix(baseName, "ephemeral_")
			name = strings.TrimSuffix(name, ".go")
			name = name + "_ephemeral" // Match convention
			results.Resources = append(results.Resources, name)
			resourceMap[name] = path
			continue
		}

		// Fallback: Look for Schema methods that indicate resources/data sources (SDKv2 pattern)
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
	// Also use file-based matching: if data_source_http_test.go exists with Test* functions,
	// mark "http" as having tests
	for path := range testFiles {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}

		baseName := filepath.Base(path)
		var fileResourceName string
		var isDataSourceTest bool

		// Extract resource name from filename (file-based matching)
		if strings.HasPrefix(baseName, "data_source_") && strings.HasSuffix(baseName, "_test.go") {
			fileResourceName = strings.TrimPrefix(baseName, "data_source_")
			fileResourceName = strings.TrimSuffix(fileResourceName, "_test.go")
			isDataSourceTest = true
		} else if strings.HasPrefix(baseName, "resource_") && strings.HasSuffix(baseName, "_test.go") {
			fileResourceName = strings.TrimPrefix(baseName, "resource_")
			fileResourceName = strings.TrimSuffix(fileResourceName, "_test.go")
		} else if strings.HasPrefix(baseName, "ephemeral_") && strings.HasSuffix(baseName, "_test.go") {
			// Ephemeral resources (TLS provider pattern)
			fileResourceName = strings.TrimPrefix(baseName, "ephemeral_")
			fileResourceName = strings.TrimSuffix(fileResourceName, "_test.go")
			// Add _ephemeral suffix to match the resource name
			fileResourceName = fileResourceName + "_ephemeral"
		} else if strings.HasSuffix(baseName, "_resource_test.go") {
			// Reversed naming: group_resource_test.go -> group
			fileResourceName = strings.TrimSuffix(baseName, "_resource_test.go")
		} else if strings.HasSuffix(baseName, "_data_source_test.go") || strings.HasSuffix(baseName, "_datasource_test.go") {
			// Reversed naming: inventory_data_source_test.go -> inventory
			fileResourceName = strings.TrimSuffix(baseName, "_data_source_test.go")
			if strings.HasSuffix(baseName, "_datasource_test.go") {
				fileResourceName = strings.TrimSuffix(baseName, "_datasource_test.go")
			}
			isDataSourceTest = true
		}

		// Count test functions in the file
		testFunctionCount := 0
		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}

			testName := funcDecl.Name.Name

			// Match any test function: Test*, not just TestAcc*
			if !strings.HasPrefix(testName, "Test") {
				return true
			}

			testFunctionCount++

			// Extract resource name from test function using multiple patterns
			// Try multiple patterns for test function names:
			// 1. TestAccDataSourceHelmTemplate_* -> helm_template
			// 2. TestAccDataTemplate_* -> helm_template (abbreviated pattern)
			// 3. TestAccHelmRelease_* -> helm_release
			// 4. TestAccResourceHelmRelease_* -> helm_release
			// 5. TestDataSource_200 -> (use file-based matching)
			// 6. TestPrivateKeyRSA -> private_key

			if strings.HasPrefix(testName, "TestAccDataSource") {
				// Data source test: TestAccDataSourceHelmTemplate_* -> helm_template
				parts := strings.SplitN(strings.TrimPrefix(testName, "TestAccDataSource"), "_", 2)
				if len(parts) > 0 && parts[0] != "" {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			} else if strings.HasPrefix(testName, "TestAccData") {
				// Abbreviated data source test: TestAccDataTemplate_* -> helm_template
				parts := strings.SplitN(strings.TrimPrefix(testName, "TestAccData"), "_", 2)
				if len(parts) > 0 && parts[0] != "" {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			} else if strings.HasPrefix(testName, "TestAccResource") {
				// Resource test: TestAccResourceHelmRelease_* -> helm_release
				parts := strings.SplitN(strings.TrimPrefix(testName, "TestAccResource"), "_", 2)
				if len(parts) > 0 && parts[0] != "" {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			} else if strings.HasPrefix(testName, "TestAcc") {
				// Generic resource test: TestAccHelmRelease_* -> helm_release
				name := strings.TrimPrefix(testName, "TestAcc")
				parts := strings.SplitN(name, "_", 2)
				if len(parts) > 0 && parts[0] != "" {
					resourceTests[toSnakeCase(parts[0])] = true
				}
			} else if strings.HasPrefix(testName, "TestDataSource") {
				// Non-Acc data source test: TestDataSource_200 or TestDataSourceHttp_basic
				name := strings.TrimPrefix(testName, "TestDataSource")
				if !strings.HasPrefix(name, "_") && name != "" {
					// Has resource name in function: TestDataSourceHttp_basic -> http
					parts := strings.SplitN(name, "_", 2)
					if len(parts) > 0 && parts[0] != "" {
						resourceTests[toSnakeCase(parts[0])] = true
					}
				}
				// If starts with underscore (TestDataSource_200), rely on file-based matching
			} else if strings.HasPrefix(testName, "TestResource") {
				// Non-Acc resource test: TestResourceWidget_basic -> widget
				name := strings.TrimPrefix(testName, "TestResource")
				if !strings.HasPrefix(name, "_") && name != "" {
					parts := strings.SplitN(name, "_", 2)
					if len(parts) > 0 && parts[0] != "" {
						resourceTests[toSnakeCase(parts[0])] = true
					}
				}
			} else if strings.HasPrefix(testName, "Test") {
				// Generic test: TestPrivateKeyRSA -> private_key
				name := strings.TrimPrefix(testName, "Test")
				if name != "" && !strings.HasPrefix(name, "_") {
					// Skip type identifiers like DataSource, Resource
					if name != "DataSource" && name != "Resource" && !strings.HasPrefix(name, "DataSource_") && !strings.HasPrefix(name, "Resource_") {
						parts := strings.SplitN(name, "_", 2)
						if len(parts) > 0 && parts[0] != "" {
							// Clean up CamelCase resource names
							extracted := extractResourceFromCamelCase(parts[0])
							if extracted != "" {
								resourceTests[toSnakeCase(extracted)] = true
							}
						}
					}
				}
			}
			return true
		})

		// File-based matching: if file matches pattern and has test functions, mark resource as tested
		if fileResourceName != "" && testFunctionCount > 0 {
			resourceTests[fileResourceName] = true
			_ = isDataSourceTest // For future use if needed
		}
	}

	// Check for untested resources
	for name, file := range resourceMap {
		// Check both exact match and partial match (e.g., helm_release matches both "helm_release" and "release")
		hasTest := resourceTests[name]
		if !hasTest {
			// Try partial match by removing common prefixes
			parts := strings.Split(name, "_")
			if len(parts) > 1 {
				// Try without first part: helm_release -> release
				shortName := strings.Join(parts[1:], "_")
				hasTest = resourceTests[shortName]
			}
		}

		if hasTest {
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
		// Check both exact match and partial match (e.g., helm_template matches both "helm_template" and "template")
		hasTest := resourceTests[name]
		if !hasTest {
			// Try partial match by removing common prefixes
			parts := strings.Split(name, "_")
			if len(parts) > 1 {
				// Try without first part: helm_template -> template
				shortName := strings.Join(parts[1:], "_")
				hasTest = resourceTests[shortName]
			}
		}

		if hasTest {
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

// extractResourceFromCamelCase extracts the resource name from a CamelCase string,
// removing known suffixes like RSA, ECDSA, ED25519, etc.
func extractResourceFromCamelCase(s string) string {
	if s == "" {
		return ""
	}

	// Known algorithm/type suffixes to strip
	suffixes := []string{
		"RSA",
		"ECDSA",
		"ED25519",
		"SHA256",
		"SHA384",
		"SHA512",
		"V1",
		"V2",
		"V3",
	}

	result := s
	for _, suffix := range suffixes {
		if strings.HasSuffix(result, suffix) {
			result = strings.TrimSuffix(result, suffix)
			break
		}
	}

	// If we stripped everything or nothing remains, return original
	if result == "" {
		return s
	}

	return result
}
