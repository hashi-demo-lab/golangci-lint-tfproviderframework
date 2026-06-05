// Package discovery implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package discovery

import (
	"go/ast"
	"path/filepath"
	"strings"

	"github.com/example/tfprovidertest/internal/fileutil"
	"github.com/example/tfprovidertest/internal/registry"
)

// isNilIdent reports whether expr is the predeclared identifier `nil`.
// Used to distinguish an inert `Field: nil` from a real value.
func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// getReceiverTypeName extracts the receiver type name from a function declaration.
// For example: func (r *WidgetResource) Schema(...) returns "WidgetResource"
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
// For example: "WidgetResource" -> "widget", "HttpDataSource" -> "http", "JobAction" -> "job"
func extractResourceName(typeName string) string {
	return fileutil.ExtractResourceName(typeName)
}

// isBaseClassType checks if a type name represents a base/infrastructure class
// that should not be registered as an actual resource.
// Base classes are used for composition (embedding) and define common Schema
// methods that are inherited by concrete resource types.
// Examples: BaseDataSource, BaseResource, BaseEdaDataSource, BaseDataSourceWithOrg
func isBaseClassType(typeName string) bool {
	// Check if the type name starts with "Base" (case-sensitive)
	// This catches common patterns like BaseDataSource, BaseResource, BaseEdaDataSource
	if strings.HasPrefix(typeName, "Base") {
		return true
	}

	// Also check for common generic infrastructure patterns
	lowerName := strings.ToLower(typeName)
	genericPrefixes := []string{"generic", "common", "abstract", "internal"}
	for _, prefix := range genericPrefixes {
		if strings.HasPrefix(lowerName, prefix) {
			return true
		}
	}

	return false
}

// toSnakeCase converts CamelCase to snake_case (e.g., "MyResource" -> "my_resource")
func toSnakeCase(s string) string {
	return fileutil.SnakeCase(s)
}

// toTitleCase converts snake_case to TitleCase (e.g., "my_resource" -> "MyResource")
func toTitleCase(s string) string {
	return fileutil.TitleCase(s)
}

// hasImportStateMethod reports whether the file declares an ImportState method
// on the given receiver type.
//
// recvType must be the ACTUAL Go receiver type name (e.g. "systemConfigResource"),
// resolved from discovery's RecvTypeToIndex map. An earlier version reconstructed
// the expected receiver as toTitleCase(resourceName)+"Resource", which failed for
// unexported receivers and for resources whose canonical name was renamed via
// Metadata() — e.g. terraform-provider-powerhmc's `systemConfigResource` with
// canonical name "sys_config" would never match the reconstructed
// "SysConfigResource", producing a false "missing ImportState" diagnostic in the
// analyzer/plugin path (finding F1).
func hasImportStateMethod(file *ast.File, recvType string) bool {
	if recvType == "" {
		return false
	}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "ImportState" {
			return true
		}
		if funcDecl.Recv != nil && getReceiverTypeName(funcDecl.Recv) == recvType {
			found = true
			return false
		}
		return true
	})
	return found
}

// extractAttributes parses the schema attributes from a Schema() function body
func extractAttributes(body *ast.BlockStmt) []*registry.AttributeInfo {
	var attributes []*registry.AttributeInfo
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
func parseAttributesMap(mapLit *ast.CompositeLit) []*registry.AttributeInfo {
	var attributes []*registry.AttributeInfo

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
		attr := &registry.AttributeInfo{
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

				// Get the field name being set
				var fieldName string
				if ident, ok := attrKV.Key.(*ast.Ident); ok {
					fieldName = ident.Name
				}

				switch fieldName {
				case "Computed":
					attr.Computed = isTrue(attrKV.Value)
				case "Optional":
					attr.Optional = isTrue(attrKV.Value)
				case "Required":
					attr.Required = isTrue(attrKV.Value)
				case "Type":
					// Extract type from attribute
					attr.Type = extractTypeString(attrKV.Value)
				case "Validators":
					// Check if there are validators
					attr.HasValidators = true
					attr.ValidatorTypes = extractValidatorTypes(attrKV.Value)
				case "PlanModifiers":
					// Check for RequiresReplace
					if hasRequiresReplace(attrKV.Value) {
						attr.IsUpdatable = false
					}
				}
			}
		}

		attributes = append(attributes, attr)
	}

	return attributes
}

// isTrue checks if an AST expression represents a boolean true value
func isTrue(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "true"
	}
	return false
}

// extractTypeString extracts the type name from an AST expression
func extractTypeString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// Handle types like basetypes.StringType, types.ObjectType, etc.
		if sel, ok := e.X.(*ast.Ident); ok {
			return sel.Name + "." + e.Sel.Name
		}
	case *ast.Ident:
		return e.Name
	}
	return ""
}

// extractValidatorTypes extracts the types of validators from a composite literal
func extractValidatorTypes(expr ast.Expr) []string {
	var validators []string

	if compLit, ok := expr.(*ast.CompositeLit); ok {
		for _, elt := range compLit.Elts {
			if callExpr, ok := elt.(*ast.CallExpr); ok {
				if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					validators = append(validators, sel.Sel.Name)
				}
			}
		}
	}

	return validators
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

// isBaseClassFile checks if a file is a base class file that should be excluded.
// Base class files typically contain abstract base classes for resources.
// They follow naming patterns: base_*.go or base.*.go
func IsBaseClassFile(filePath string) bool {
	return fileutil.IsBaseClassFile(filePath)
}

// IsSweeperFile checks if a file is a sweeper file that should be excluded.
// Sweeper files are test infrastructure files for cleaning up resources after
// acceptance tests. They follow the naming pattern *_sweeper.go.
func IsSweeperFile(filePath string) bool {
	return fileutil.IsSweeperFile(filePath)
}

// IsMigrationFile checks if a file is a state migration file that should be excluded.
// Migration files are state migration utilities, not production resources. They follow
// naming patterns: *_migrate.go, *_migration*.go, *_state_upgrader.go
func IsMigrationFile(filePath string) bool {
	return fileutil.IsMigrationFile(filePath)
}

// shouldExcludeFile checks if a file path matches any of the exclude patterns
func shouldExcludeFile(filePath string, excludePaths []string) bool {
	return fileutil.ShouldExcludeFile(filePath, excludePaths)
}

// ExtractResourceNameFromPath extracts resource name from file path.
func ExtractResourceNameFromPath(filePath string) (string, bool) {
	baseName := filepath.Base(filePath)

	// Must be a test file
	if !strings.HasSuffix(baseName, "_test.go") {
		return "", false
	}

	nameWithoutTest := strings.TrimSuffix(baseName, "_test.go")

	// Check prefix patterns
	if strings.HasPrefix(nameWithoutTest, "resource_") {
		return strings.TrimPrefix(nameWithoutTest, "resource_"), false
	}
	if strings.HasPrefix(nameWithoutTest, "data_source_") {
		return strings.TrimPrefix(nameWithoutTest, "data_source_"), true
	}
	if strings.HasPrefix(nameWithoutTest, "ephemeral_") {
		return strings.TrimPrefix(nameWithoutTest, "ephemeral_"), false
	}

	// Check suffix patterns
	if strings.HasSuffix(nameWithoutTest, "_resource") {
		return strings.TrimSuffix(nameWithoutTest, "_resource"), false
	}
	if strings.HasSuffix(nameWithoutTest, "_data_source") {
		return strings.TrimSuffix(nameWithoutTest, "_data_source"), true
	}
	if strings.HasSuffix(nameWithoutTest, "_datasource") {
		return strings.TrimSuffix(nameWithoutTest, "_datasource"), true
	}

	return "", false
}
