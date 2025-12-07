// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

// toSnakeCase converts CamelCase to snake_case (e.g., "MyResource" -> "my_resource")
func toSnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			// Add underscore before uppercase if:
			// 1. Previous char is lowercase, OR
			// 2. Next char exists and is lowercase (handles acronyms like "HTTPServer" -> "http_server")
			prev := runes[i-1]
			if unicode.IsLower(prev) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// toTitleCase converts snake_case to TitleCase (e.g., "my_resource" -> "MyResource")
func toTitleCase(s string) string {
	var result strings.Builder
	capitalizeNext := true
	for _, r := range s {
		if r == '_' {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			result.WriteRune(unicode.ToUpper(r))
			capitalizeNext = false
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// isBaseClassFile checks if a file is a base class file that should be excluded
func isBaseClassFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasPrefix(base, "base_") || strings.HasPrefix(base, "base.")
}

// IsSweeperFile checks if a file is a sweeper file that should be excluded.
// Sweeper files are test infrastructure files for cleaning up resources after
// acceptance tests. They follow the naming pattern *_sweeper.go.
func IsSweeperFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_sweeper.go")
}

// IsMigrationFile checks if a file is a state migration file that should be excluded.
// Migration files are state migration utilities, not production resources. They follow
// naming patterns: *_migrate.go, *_migration*.go, *_state_upgrader.go
func IsMigrationFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_migrate.go") ||
		strings.Contains(base, "_migration") ||
		strings.HasSuffix(base, "_state_upgrader.go")
}

// shouldExcludeFile checks if a file path matches any of the exclude patterns
func shouldExcludeFile(filePath string, excludePaths []string) bool {
	for _, pattern := range excludePaths {
		// Try matching the full path
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		// Try matching just the base name
		if matched, _ := filepath.Match(pattern, filepath.Base(filePath)); matched {
			return true
		}
		// Try matching with Contains for patterns like "vendor/"
		if strings.Contains(filePath, strings.TrimSuffix(pattern, "/")) {
			return true
		}
	}
	return false
}

// isTestFunction checks if a function name matches any of the test naming patterns.
// With file-based matching, we're more permissive - any Test* function is valid
// as long as it's in the correct test file.
func isTestFunction(funcName string, customPatterns []string) bool {
	// Always require "Test" prefix (capital T for exported tests)
	if !strings.HasPrefix(funcName, "Test") {
		return false
	}

	// With file-based matching, any Test* function is valid
	// Custom patterns can be used to further restrict if needed
	if len(customPatterns) == 0 {
		// No custom patterns - accept any Test* function
		return true
	}

	// If custom patterns are provided, check against them
	for _, pattern := range customPatterns {
		if strings.HasPrefix(funcName, pattern) {
			return true
		}
	}

	return false
}

// IsTestFunctionExported is the exported version of isTestFunction for testing
func IsTestFunctionExported(funcName string, customPatterns []string) bool {
	return isTestFunction(funcName, customPatterns)
}

// CamelCaseToSnakeCaseExported is the exported version of toSnakeCase for testing
func CamelCaseToSnakeCaseExported(s string) string {
	return toSnakeCase(s)
}

// formatResourceLocation formats the resource location for enhanced issue reporting.
// Returns a string like "Resource: /path/to/file.go:45"
func formatResourceLocation(pass *analysis.Pass, resource *ResourceInfo) string {
	pos := pass.Fset.Position(resource.SchemaPos)
	return fmt.Sprintf("Resource: %s:%d", pos.Filename, pos.Line)
}

// getReceiverTypeName extracts the type name from a function receiver.
func getReceiverTypeName(recv *ast.FieldList) string {
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

// extractResourceName extracts the resource name from a type name.
// For example: "WidgetResource" -> "widget", "HttpDataSource" -> "http"
func extractResourceName(typeName string) string {
	// Remove "Resource" or "DataSource" suffix
	name := strings.TrimSuffix(typeName, "Resource")
	name = strings.TrimSuffix(name, "DataSource")

	// Convert CamelCase to snake_case
	return toSnakeCase(name)
}

// hasRequiresReplace checks if a node contains RequiresReplace plan modifier
func hasRequiresReplace(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		// Look for function calls like stringplanmodifier.RequiresReplace()
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if the function name contains "RequiresReplace"
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if strings.Contains(sel.Sel.Name, "RequiresReplace") {
				found = true
				return false
			}
		}

		return true
	})
	return found
}

// hasImportStateMethod checks if a file has ImportState method for a resource
func hasImportStateMethod(file *ast.File, resourceName string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "ImportState" {
			return true
		}

		if funcDecl.Recv != nil {
			recvType := getReceiverTypeName(funcDecl.Recv)
			expectedType := toTitleCase(resourceName) + "Resource"
			if recvType == expectedType || recvType == "*"+expectedType {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// extractAttributes parses the schema attributes from a Schema() function body
func extractAttributes(body *ast.BlockStmt) []AttributeInfo {
	var attributes []AttributeInfo
	if body == nil {
		return attributes
	}

	// Find the schema.Schema composite literal
	ast.Inspect(body, func(n ast.Node) bool {
		// Look for CompositeLit that represents schema.Schema{}
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if this is schema.Schema type
		if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
			if sel.Sel.Name != "Schema" {
				return true
			}
		}

		// Find the Attributes field in schema.Schema
		for _, elt := range compLit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// Check if this is the Attributes field
			if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == "Attributes" {
				// Parse the attributes map
				if mapLit, ok := kv.Value.(*ast.CompositeLit); ok {
					attributes = parseAttributesMap(mapLit)
				}
			}
		}

		return false // Don't recurse into nested schemas
	})

	return attributes
}

// parseAttributesMap parses the attributes map from a schema
func parseAttributesMap(mapLit *ast.CompositeLit) []AttributeInfo {
	var attributes []AttributeInfo

	for _, elt := range mapLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		// Get attribute name
		var attrName string
		if lit, ok := kv.Key.(*ast.BasicLit); ok {
			attrName = strings.Trim(lit.Value, `"`)
		}

		if attrName == "" {
			continue
		}

		// Parse attribute properties
		attr := AttributeInfo{
			Name:        attrName,
			IsUpdatable: true, // Default to updatable unless RequiresReplace found
		}

		// Parse the attribute composite literal
		if attrLit, ok := kv.Value.(*ast.CompositeLit); ok {
			for _, attrElt := range attrLit.Elts {
				attrKV, ok := attrElt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}

				fieldName := ""
				if ident, ok := attrKV.Key.(*ast.Ident); ok {
					fieldName = ident.Name
				}

				switch fieldName {
				case "Required":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Required = ident.Name == "true"
					}
				case "Optional":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Optional = ident.Name == "true"
					}
				case "Computed":
					if ident, ok := attrKV.Value.(*ast.Ident); ok {
						attr.Computed = ident.Name == "true"
					}
				case "PlanModifiers":
					// Check if RequiresReplace is present
					attr.IsUpdatable = !hasRequiresReplace(attrKV.Value)
				case "Validators":
					// Check for validators
					if compLit, ok := attrKV.Value.(*ast.CompositeLit); ok {
						attr.HasValidators = len(compLit.Elts) > 0
					}
				}
			}

			// Determine attribute type from the composite literal type
			if sel, ok := attrLit.Type.(*ast.SelectorExpr); ok {
				attr.Type = sel.Sel.Name
			}
		}

		attributes = append(attributes, attr)
	}

	return attributes
}
