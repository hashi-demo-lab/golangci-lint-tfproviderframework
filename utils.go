// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

// TestFunctionPrefixes are the common prefixes used in test function names.
// These are stripped when matching test functions to resources.
// The order matters - more specific patterns should come first.
var TestFunctionPrefixes = []string{
	"TestAccDataSource",
	"TestAccResource",
	"TestAcc",
	"TestDataSource",
	"TestResource",
	"Test",
}

// TestFunctionSuffixes are the common suffixes used in test function names.
// These are stripped when matching test functions to resources.
var TestFunctionSuffixes = []string{
	"_basic",
	"_generated",
	"_complete",
	"_update",
	"_import",
	"_disappears",
	"_migrate",
	"_full",
	"_minimal",
}

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

// testFuncPatterns matches various test function naming conventions.
// These patterns are used to extract resource names from test function names.
//
// Examples:
//   - TestAccAWSInstance_basic -> provider=AWS, resource=Instance
//   - TestAccWidget_basic -> provider="", resource=Widget
//   - TestAccDataSourceHTTP_basic -> resource=HTTP
//   - TestAccResourceWidget_basic -> resource=Widget
var testFuncPatterns = []*regexp.Regexp{
	// Pattern 1: TestAcc{Provider}{Resource}_{suffix} (e.g., TestAccAWSInstance_basic)
	// Provider is optional uppercase letters followed by resource name
	// The underscore is optional to match patterns like TestAccEDAEventStreamAfterCreateAction
	regexp.MustCompile(`^TestAcc([A-Z][a-z]+)?([A-Z][a-zA-Z0-9]+?)(?:_|Action|$)`),
	// Pattern 2: TestAccDataSource{Resource}_{suffix}
	regexp.MustCompile(`^TestAccDataSource([A-Z][a-zA-Z0-9]+?)(?:_|$)`),
	// Pattern 3: TestAccResource{Resource}_{suffix}
	regexp.MustCompile(`^TestAccResource([A-Z][a-zA-Z0-9]+?)(?:_|$)`),
	// Pattern 4: TestAcc{Resource}_{suffix} (no provider prefix, simple pattern)
	regexp.MustCompile(`^TestAcc([A-Z][a-zA-Z0-9]+?)(?:_|Action|$)`),
}

// actionLifecycleSuffixes are CamelCase suffixes that describe when/how an action runs,
// not what the action is. These should be stripped before matching.
var actionLifecycleSuffixes = []string{
	"AfterCreate",
	"AfterUpdate",
	"AfterDelete",
	"BeforeCreate",
	"BeforeUpdate",
	"BeforeDelete",
	"DoesNotTrigger",
	"Unrelated",
}

// ExtractResourceFromFuncName attempts to extract a resource name from a test function name.
// Returns the resource name in snake_case and whether a match was found.
// For action tests (e.g., TestAccAAPJobAction_basic), it strips the "Action" suffix.
//
// Examples:
//
//	TestAccAWSInstance_basic -> "aws_instance", true
//	TestAccWidget_basic -> "widget", true
//	TestAccDataSourceHTTP_basic -> "http", true
//	TestAccResourceWidget_basic -> "widget", true
//	TestAccAAPJobAction_basic -> "aap_job", true
//	TestHelper -> "", false
func ExtractResourceFromFuncName(funcName string) (string, bool) {
	var resourceName string

	// Try data source pattern first (more specific)
	if matches := testFuncPatterns[1].FindStringSubmatch(funcName); len(matches) > 1 {
		resourceName = matches[1]
	} else if matches := testFuncPatterns[2].FindStringSubmatch(funcName); len(matches) > 1 {
		// Try resource pattern
		resourceName = matches[1]
	} else if matches := testFuncPatterns[0].FindStringSubmatch(funcName); len(matches) > 2 {
		// Try provider+resource pattern
		// matches[1] = provider (optional), matches[2] = resource
		if matches[2] != "" {
			resourceName = matches[2]
		}
	} else if matches := testFuncPatterns[3].FindStringSubmatch(funcName); len(matches) > 1 {
		// Try simple pattern (no provider prefix)
		resourceName = matches[1]
	}

	if resourceName == "" {
		return "", false
	}

	// Strip "Action" suffix for action tests (e.g., JobAction -> Job)
	resourceName = strings.TrimSuffix(resourceName, "Action")

	// Strip action lifecycle suffixes (e.g., EventStreamAfterCreate -> EventStream)
	for _, suffix := range actionLifecycleSuffixes {
		if strings.HasSuffix(resourceName, suffix) {
			resourceName = strings.TrimSuffix(resourceName, suffix)
			break
		}
	}

	return toSnakeCase(resourceName), true
}

// ExtractResourceFromFuncNameWithoutPrefix extracts a resource name and also tries
// stripping provider prefixes. Returns both the full name and the stripped name.
func ExtractResourceFromFuncNameWithoutPrefix(funcName string) (fullName string, strippedName string, found bool) {
	fullName, found = ExtractResourceFromFuncName(funcName)
	if !found {
		return "", "", false
	}

	strippedName = stripProviderPrefix(fullName)
	return fullName, strippedName, true
}

// multiResourcePattern matches patterns like "ResourceWithDataSource" or "ResourceAndDataSource"
var multiResourcePattern = regexp.MustCompile(`^TestAcc([A-Z][a-zA-Z0-9]+?)(?:Resource)?(?:With|And)([A-Z][a-zA-Z0-9]+?)(?:DataSource|Resource)?(?:_|$)`)

// ExtractAllResourcesFromFuncName extracts all resource names mentioned in a test function name.
// This handles multi-resource integration tests like "TestAccInventoryResourceWithOrganizationDataSource".
// Returns all extracted names (both with and without provider prefix stripped).
func ExtractAllResourcesFromFuncName(funcName string) []string {
	var results []string
	seen := make(map[string]bool)

	addResult := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			results = append(results, name)
		}
	}

	// Try multi-resource pattern first
	if matches := multiResourcePattern.FindStringSubmatch(funcName); len(matches) > 2 {
		// matches[1] = first resource, matches[2] = second resource
		name1 := toSnakeCase(matches[1])
		name2 := toSnakeCase(matches[2])
		addResult(name1)
		addResult(stripProviderPrefix(name1))
		addResult(name2)
		addResult(stripProviderPrefix(name2))
	}

	// Also try standard extraction
	if fullName, strippedName, found := ExtractResourceFromFuncNameWithoutPrefix(funcName); found {
		addResult(fullName)
		addResult(strippedName)
	}

	return results
}

// stripProviderPrefix removes the first underscore-separated segment if it looks like
// a provider prefix (2-6 lowercase letters). This handles cases like:
// - aap_job -> job
// - eda_event_stream -> event_stream
// - aws_instance -> instance
//
// It only strips if:
// 1. There's at least one underscore
// 2. The first segment is 2-6 characters (typical provider prefix length)
// 3. The remaining name is non-empty
func stripProviderPrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx < 2 || idx > 6 {
		// No underscore, or prefix too short/long to be a provider
		return name
	}

	remainder := name[idx+1:]
	if remainder == "" {
		return name
	}

	return remainder
}

// ExtractProviderFromFuncName extracts the provider prefix from a test function name.
// Returns empty string if no provider prefix found.
//
// Examples:
//
//	TestAccAWSInstance_basic -> "aws"
//	TestAccGoogleComputeInstance_update -> "google"
//	TestAccWidget_basic -> ""
func ExtractProviderFromFuncName(funcName string) string {
	if matches := testFuncPatterns[0].FindStringSubmatch(funcName); len(matches) > 1 {
		if matches[1] != "" {
			return strings.ToLower(matches[1])
		}
	}
	return ""
}

// IsBaseClassFile checks if a file is a base class file that should be excluded.
// Base class files typically follow the naming pattern base_*.go and contain
// abstract/base implementations that are not actual Terraform resources.
func IsBaseClassFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasPrefix(base, "base_") || strings.HasPrefix(base, "base.")
}

// isBaseClassFile is an unexported alias for backward compatibility
func isBaseClassFile(filePath string) bool {
	return IsBaseClassFile(filePath)
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

// FormatResourceLocation formats the resource location for enhanced issue reporting.
// Returns a string like "Resource: /path/to/file.go:45"
// Exported for use by external tools that need to format resource locations.
func FormatResourceLocation(pass *analysis.Pass, resource *ResourceInfo) string {
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
// For example: "WidgetResource" -> "widget", "HttpDataSource" -> "http", "JobAction" -> "job"
func extractResourceName(typeName string) string {
	// Remove "Resource", "DataSource", or "Action" suffix
	name := strings.TrimSuffix(typeName, "Resource")
	name = strings.TrimSuffix(name, "DataSource")
	name = strings.TrimSuffix(name, "Action")

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

// standardRequiresReplaceModifiers maps known plan modifier names that indicate RequiresReplace.
// These are the standard modifiers from terraform-plugin-framework.
var standardRequiresReplaceModifiers = map[string]bool{
	"RequiresReplace":              true,
	"RequiresReplaceIf":            true,
	"RequiresReplaceIfConfigured":  true,
	"UseStateForUnknown":           false, // Not a replace modifier
	"RequiresReplaceIfFuncReturns": true,
}

// RequiresReplaceResult holds the result of checking for RequiresReplace modifiers.
type RequiresReplaceResult struct {
	Found        bool    // Whether RequiresReplace was found
	Confidence   float64 // 0.0-1.0 confidence score
	ModifierName string  // Name of the modifier found
}

// hasRequiresReplaceWithConfidence checks if a node contains RequiresReplace plan modifier
// and returns detailed information about the finding including confidence level.
func hasRequiresReplaceWithConfidence(node ast.Node) RequiresReplaceResult {
	result := RequiresReplaceResult{
		Found:      false,
		Confidence: 0.0,
	}

	// Handle nil node
	if node == nil {
		return result
	}

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for function calls like stringplanmodifier.RequiresReplace()
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			modifierName := sel.Sel.Name

			// Check against standard modifiers
			if isReplace, known := standardRequiresReplaceModifiers[modifierName]; known {
				if isReplace {
					result.Found = true
					result.ModifierName = modifierName
					result.Confidence = 1.0 // High confidence for known modifiers
					return false
				}
			} else if strings.Contains(modifierName, "RequiresReplace") {
				// Unknown modifier containing "RequiresReplace" - lower confidence
				result.Found = true
				result.ModifierName = modifierName
				result.Confidence = 0.8 // Lower confidence for unknown patterns
				return false
			}
		}

		return true
	})

	return result
}

// HasRequiresReplaceWithConfidence is the public API for checking RequiresReplace with confidence.
func HasRequiresReplaceWithConfidence(node ast.Node) RequiresReplaceResult {
	return hasRequiresReplaceWithConfidence(node)
}

// suppressionPatterns defines patterns for lint suppression comments.
var suppressionPatterns = []*regexp.Regexp{
	// nolint:checkname format (golangci-lint style)
	regexp.MustCompile(`//\s*nolint:\s*([a-zA-Z0-9_,\-]+)`),
	// lint:ignore checkname format
	regexp.MustCompile(`//\s*lint:ignore\s+([a-zA-Z0-9_,\-]+)`),
	// tfprovidertest:disable checkname format (our custom format)
	regexp.MustCompile(`//\s*tfprovidertest:disable\s+([a-zA-Z0-9_,\-]+)`),
}

// CheckSuppressionComment checks if a specific check is suppressed in comments.
// Returns true if the checkName is found in any suppression comment.
func CheckSuppressionComment(comments []*ast.CommentGroup, checkName string) bool {
	suppressed := GetSuppressedChecks(comments)
	for _, s := range suppressed {
		if s == checkName || s == "all" {
			return true
		}
	}
	return false
}

// GetSuppressedChecks extracts all suppressed check names from comments.
// Returns a slice of check names that are suppressed.
func GetSuppressedChecks(comments []*ast.CommentGroup) []string {
	var suppressed []string

	for _, group := range comments {
		if group == nil {
			continue
		}

		for _, comment := range group.List {
			text := comment.Text

			for _, pattern := range suppressionPatterns {
				matches := pattern.FindStringSubmatch(text)
				if len(matches) > 1 {
					// Split by comma for multiple checks
					checks := strings.Split(matches[1], ",")
					for _, check := range checks {
						check = strings.TrimSpace(check)
						if check != "" {
							suppressed = append(suppressed, check)
						}
					}
				}
			}
		}
	}

	return suppressed
}

// ExtractResourceNameFromPath extracts a resource name from a file path.
// It handles various naming patterns commonly used in Terraform provider test files:
//   - resource_<name>_test.go -> (<name>, false)
//   - data_source_<name>_test.go -> (<name>, true)
//   - ephemeral_<name>_test.go -> (<name>, false)
//   - <name>_resource_test.go -> (<name>, false)
//   - <name>_data_source_test.go -> (<name>, true)
//   - <name>_datasource_test.go -> (<name>, true)
//
// Returns the extracted resource name and a boolean indicating if it's a data source.
// If no pattern matches, returns ("", false).
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
