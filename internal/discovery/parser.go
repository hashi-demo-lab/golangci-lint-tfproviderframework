// Package discovery implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/printer"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/example/tfprovidertest/internal/matching"
	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
)

// Regex to find HCL blocks: resource, data, or action
// Examples:
//   - resource "example_widget" "name" {
//   - data "example_datasource" "name" {
//   - action "example_action" "name" {
// Captures the type (e.g., "example_widget", "google_compute_disk")
var ResourceTypeRegex = regexp.MustCompile(`(?:resource|data|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)

// HCLBlockRegex captures both the block type (resource/data/action) and the resource type.
// Groups: [1] = block type (resource|data|action), [2] = resource type (e.g., "aws_instance")
var HCLBlockRegex = regexp.MustCompile(`(resource|data|action)\s+"([^"]+)"\s+"[^"]+"\s+\{`)

// InferredResource represents a resource found in HCL config with its block type.
type InferredResource struct {
	BlockType    string // "resource", "data", or "action"
	ResourceType string // e.g., "aws_instance", "aap_job_launch"
}

// LocalHelper represents a discovered local test helper function.
type LocalHelper struct {
	Name     string
	FilePath string
	FuncDecl *ast.FuncDecl
}

// ParserConfig holds all configuration options for parsing test files.
// This struct consolidates the various parameters that were previously
// spread across multiple parseTestFile* functions.
type ParserConfig struct {
	CustomHelpers         []string      // Custom test helper functions (e.g., "mypackage.AccTest")
	LocalHelpers          []LocalHelper // Local test helper functions discovered in the codebase
	TestNamePatterns      []string      // Custom test name patterns (e.g., "TestAcc*", "TestResource*")
	TestFilePattern       string        // Pattern for test files (e.g., "*_test.go")
	ResourceNamingPattern string        // Regex pattern for extracting resource names from identifiers
	ProviderPrefix        string        // Provider prefix for function name matching (e.g., "AWS", "Google")
	ResourcePathPattern   string        // Pattern for resource files (e.g., "resource_*.go")
	DataSourcePathPattern string        // Pattern for data source files (e.g., "data_source_*.go")
}

// DefaultParserConfig returns a ParserConfig with default/empty values.
func DefaultParserConfig() ParserConfig {
	return ParserConfig{
		CustomHelpers:         nil,
		LocalHelpers:          nil,
		TestNamePatterns:      nil,
		TestFilePattern:       "*_test.go",
		ResourceNamingPattern: "",
		ProviderPrefix:        "",
		ResourcePathPattern:   "resource_*.go",
		DataSourcePathPattern: "data_source_*.go",
	}
}

// ExclusionResult tracks why a file was excluded from analysis.
type ExclusionResult struct {
	FilePath       string
	Excluded       bool
	Reason         string
	MatchedPattern string
}

// ExclusionDiagnostics collects information about all excluded files.
type ExclusionDiagnostics struct {
	ExcludedFiles []ExclusionResult
	TotalExcluded int
}

// hashConfigExpr generates a hash of a config expression for comparison.
// This normalizes the AST representation to detect equivalent configs.
func hashConfigExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}

	var buf strings.Builder
	fset := token.NewFileSet()

	// Print the AST node to a string for hashing
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}

	// Normalize whitespace
	normalized := strings.Join(strings.Fields(buf.String()), " ")

	// Generate hash
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8]) // First 8 bytes for brevity
}

// DiscoveryStrategy represents a strategy for discovering resources in Go source files.
// Each strategy implements a different approach to finding Terraform provider resources,
// data sources, or actions by analyzing AST nodes.
type DiscoveryStrategy interface {
	// Name returns the name of the strategy for logging/debugging
	Name() string
	// Discover scans the file and returns discovered resources
	Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo
}

// DiscoveryState holds shared state used across multiple discovery strategies.
// This allows strategies to coordinate and share information (e.g., for override logic).
type DiscoveryState struct {
	// Seen tracks which resources have been discovered using "kind:name" keys
	Seen map[string]bool
	// RecvTypeToIndex maps receiver type names to resource indices for Metadata override logic
	RecvTypeToIndex map[string]int
	// ActionTypeNames tracks action type names discovered by ActionFactoryStrategy
	ActionTypeNames map[string]token.Pos
	// ProcessedActionTypes tracks which action types have been processed via Metadata
	ProcessedActionTypes map[string]bool
	// Resources accumulates all discovered resources across strategies
	Resources []*registry.ResourceInfo
}

// NewDiscoveryState creates a new DiscoveryState with initialized maps.
func NewDiscoveryState() *DiscoveryState {
	return &DiscoveryState{
		Seen:                 make(map[string]bool),
		RecvTypeToIndex:      make(map[string]int),
		ActionTypeNames:      make(map[string]token.Pos),
		ProcessedActionTypes: make(map[string]bool),
		Resources:            make([]*registry.ResourceInfo, 0),
	}
}

// SeenKey generates a unique key for tracking seen resources.
func (s *DiscoveryState) SeenKey(kind registry.ResourceKind, name string) string {
	return kind.String() + ":" + name
}

// SchemaMethodStrategy discovers resources by looking for Schema() methods on types
// ending with Resource/DataSource/Action.
type SchemaMethodStrategy struct{}

func (s *SchemaMethodStrategy) Name() string {
	return "SchemaMethod"
}

func (s *SchemaMethodStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	// Strategy 1: Look for Schema() methods on Resource/DataSource/Action types
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Schema" {
			return true
		}

		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType == "" {
			return true
		}

		var kind registry.ResourceKind
		isDataSource := strings.HasSuffix(recvType, "DataSource")
		isResource := strings.HasSuffix(recvType, "Resource")
		isAction := strings.HasSuffix(recvType, "Action")

		if isDataSource {
			kind = registry.KindDataSource
		} else if isAction {
			// Skip actions in Strategy 1 - they're handled by Strategy 4/4b
			// which properly extracts the TypeName from Metadata method
			return true
		} else if isResource {
			kind = registry.KindResource
		} else {
			return true
		}

		name := extractResourceName(recvType)
		key := state.SeenKey(kind, name)
		if name == "" || state.Seen[key] {
			return true
		}

		state.Seen[key] = true
		attrs := extractAttributes(funcDecl.Body)
		// Convert []*AttributeInfo to []AttributeInfo
		var attributes []registry.AttributeInfo
		for _, attr := range attrs {
			if attr != nil {
				attributes = append(attributes, *attr)
			}
		}
		resource := &registry.ResourceInfo{
			Name:       name,
			Kind:       kind,
			FilePath:   filePath,
			SchemaPos:  funcDecl.Pos(),
			Attributes: attributes,
		}

		resources = append(resources, resource)
		state.Resources = append(state.Resources, resource)
		// Track receiver type so Strategy 3 can replace with Metadata TypeName
		state.RecvTypeToIndex[recvType] = len(state.Resources) - 1
		return true
	})

	return resources
}

// FactoryFunctionStrategy discovers resources by looking for NewXxxDataSource/NewXxxResource
// factory functions with MetadataEntitySlug in StringDescriptions struct.
type FactoryFunctionStrategy struct{}

func (f *FactoryFunctionStrategy) Name() string {
	return "FactoryFunction"
}

func (f *FactoryFunctionStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	// Strategy 2: Look for NewXxxDataSource/NewXxxResource factory functions
	// with MetadataEntitySlug in StringDescriptions struct
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil {
			return true
		}

		funcName := funcDecl.Name.Name
		isDataSource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "DataSource")
		isResource := strings.HasPrefix(funcName, "New") && strings.Contains(funcName, "Resource") && !strings.Contains(funcName, "DataSource")

		if !isDataSource && !isResource {
			return true
		}

		// Look for MetadataEntitySlug in the function body
		if funcDecl.Body != nil {
			name := extractMetadataEntitySlug(funcDecl.Body)
			kind := registry.KindResource
			if isDataSource {
				kind = registry.KindDataSource
			}
			key := state.SeenKey(kind, name)
			if name != "" && !state.Seen[key] {
				state.Seen[key] = true
				resource := &registry.ResourceInfo{
					Name:      name,
					Kind:      kind,
					FilePath:  filePath,
					SchemaPos: funcDecl.Pos(),
				}
				resources = append(resources, resource)
				state.Resources = append(state.Resources, resource)
			}
		}

		return true
	})

	return resources
}

// MetadataMethodStrategy discovers resources by looking for Metadata() methods
// with resp.TypeName assignment. This is the authoritative source for resource names
// and overrides SchemaMethodStrategy.
type MetadataMethodStrategy struct{}

func (m *MetadataMethodStrategy) Name() string {
	return "MetadataMethod"
}

func (m *MetadataMethodStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	// Strategy 3: Look for Metadata() methods with resp.TypeName assignment
	// This is the authoritative source for resource names - it overrides Strategy 1
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Metadata" {
			return true
		}

		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType == "" {
			return true
		}

		isDataSource := strings.HasSuffix(recvType, "DataSource")
		isResource := strings.HasSuffix(recvType, "Resource")

		if !isResource && !isDataSource {
			return true
		}

		// Look for resp.TypeName = "..." assignment
		if funcDecl.Body != nil {
			name := extractTypeNameFromMetadata(funcDecl.Body)
			if name == "" {
				return true
			}

			kind := registry.KindResource
			if isDataSource {
				kind = registry.KindDataSource
			}
			key := state.SeenKey(kind, name)

			// Check if Strategy 1 already registered this receiver type with a different name
			if idx, exists := state.RecvTypeToIndex[recvType]; exists {
				// Replace Strategy 1's name with the authoritative Metadata TypeName
				oldResource := state.Resources[idx]
				oldKey := state.SeenKey(oldResource.Kind, oldResource.Name)
				delete(state.Seen, oldKey)
				oldResource.Name = name
				state.Seen[key] = true
			} else if !state.Seen[key] {
				// No Strategy 1 entry, add new resource
				state.Seen[key] = true
				resource := &registry.ResourceInfo{
					Name:      name,
					Kind:      kind,
					FilePath:  filePath,
					SchemaPos: funcDecl.Pos(),
				}
				resources = append(resources, resource)
				state.Resources = append(state.Resources, resource)
			}
		}

		return true
	})

	return resources
}

// ActionFactoryStrategy discovers actions by looking for NewXxxAction factory functions
// returning action.Action and extracting TypeName from Metadata methods.
type ActionFactoryStrategy struct{}

func (a *ActionFactoryStrategy) Name() string {
	return "ActionFactory"
}

func (a *ActionFactoryStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	// Strategy 4: Look for NewXxxAction factory functions returning action.Action
	// Also collect action type names for later Metadata extraction
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil {
			return true
		}

		funcName := funcDecl.Name.Name
		// Match patterns like NewJobAction, NewWorkflowJobAction, NewEDAEventStreamPostAction
		if !strings.HasPrefix(funcName, "New") || !strings.HasSuffix(funcName, "Action") {
			return true
		}

		// Verify return type is action.Action
		if funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) == 0 {
			return true
		}

		returnType := ""
		if sel, ok := funcDecl.Type.Results.List[0].Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				returnType = ident.Name + "." + sel.Sel.Name
			}
		}

		if returnType != "action.Action" {
			return true
		}

		// Extract action type name from factory function (e.g., NewJobAction -> JobAction)
		typeName := strings.TrimPrefix(funcName, "New")
		state.ActionTypeNames[typeName] = funcDecl.Pos()

		return true
	})

	// Strategy 4b: For each action type, find its Metadata method and extract TypeName
	// This gives us the canonical name used in HCL configs (e.g., "eda_eventstream_post")
	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Metadata" || funcDecl.Recv == nil {
			return true
		}

		// Check receiver type against known action types
		recvType := getReceiverTypeName(funcDecl.Recv)
		pos, isAction := state.ActionTypeNames[recvType]
		if !isAction {
			return true
		}

		// Mark this action type as processed (don't use fallback)
		state.ProcessedActionTypes[recvType] = true

		// Extract TypeName from Metadata method body
		if funcDecl.Body != nil {
			name := extractTypeNameFromMetadata(funcDecl.Body)
			key := state.SeenKey(registry.KindAction, name)
			if name != "" && !state.Seen[key] {
				state.Seen[key] = true
				resource := &registry.ResourceInfo{
					Name:      name,
					Kind:      registry.KindAction,
					FilePath:  filePath,
					SchemaPos: pos,
				}
				resources = append(resources, resource)
				state.Resources = append(state.Resources, resource)
			}
		}

		return true
	})

	// Fallback: For actions without Metadata methods, use the factory function name
	for typeName, pos := range state.ActionTypeNames {
		// Skip if we already processed this via Metadata
		if state.ProcessedActionTypes[typeName] {
			continue
		}
		name := extractActionName("New" + typeName)
		key := state.SeenKey(registry.KindAction, name)
		if name != "" && !state.Seen[key] {
			state.Seen[key] = true
			resource := &registry.ResourceInfo{
				Name:      name,
				Kind:      registry.KindAction,
				FilePath:  filePath,
				SchemaPos: pos,
			}
			resources = append(resources, resource)
			state.Resources = append(state.Resources, resource)
		}
	}

	return resources
}

// ReturnTypeStrategy discovers resources by analyzing factory function return types.
// It detects functions returning resource.Resource, datasource.DataSource, *schema.Resource, etc.
// This handles providers that don't follow standard naming conventions.
type ReturnTypeStrategy struct{}

func (r *ReturnTypeStrategy) Name() string {
	return "ReturnType"
}

func (r *ReturnTypeStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	// Build a map of import paths to their aliases for resolving return types
	importAliases := extractImportAliases(file)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Recv != nil || funcDecl.Type.Results == nil {
			return true
		}

		// Check return type
		for _, result := range funcDecl.Type.Results.List {
			returnType := typeToString(result.Type)
			kind, isResourceType := classifyReturnType(returnType, importAliases)
			if !isResourceType {
				continue
			}

			// For SDK v2 schema.Resource, differentiate based on filename
			// SDK v2 uses *schema.Resource for both resources and data sources
			if strings.HasSuffix(strings.TrimPrefix(returnType, "*"), "schema.Resource") {
				baseName := filepath.Base(filePath)
				if strings.HasPrefix(baseName, "data_source_") {
					kind = registry.KindDataSource
				}
			}

			// Extract resource name from Metadata method body or function name
			name := r.extractResourceName(funcDecl, file, kind)
			if name == "" {
				continue
			}

			key := state.SeenKey(kind, name)
			if state.Seen[key] {
				continue
			}

			state.Seen[key] = true
			resource := &registry.ResourceInfo{
				Name:      name,
				Kind:      kind,
				FilePath:  filePath,
				SchemaPos: funcDecl.Pos(),
			}
			resources = append(resources, resource)
			state.Resources = append(state.Resources, resource)

			// Track for Metadata resolution
			funcName := funcDecl.Name.Name
			state.RecvTypeToIndex[funcName] = len(state.Resources) - 1
		}
		return true
	})

	return resources
}

// RegistryFactoryStrategy discovers resources by looking for registry.AddResourceFactory()
// and registry.AddDataSourceFactory() calls, commonly used in AWSCC and similar providers.
type RegistryFactoryStrategy struct{}

func (r *RegistryFactoryStrategy) Name() string {
	return "RegistryFactory"
}

func (r *RegistryFactoryStrategy) Discover(file *ast.File, fset *token.FileSet, filePath string, state *DiscoveryState) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo

	ast.Inspect(file, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Look for registry.AddResourceFactory or registry.AddDataSourceFactory calls
		sel, ok := callExpr.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Check if it's a registry.Add*Factory call
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "registry" {
			return true
		}

		var kind registry.ResourceKind
		switch sel.Sel.Name {
		case "AddResourceFactory":
			kind = registry.KindResource
		case "AddDataSourceFactory":
			kind = registry.KindDataSource
		case "AddListResourceFactory":
			// List resources are a variant of data sources in AWSCC (plural data sources)
			kind = registry.KindDataSource
		default:
			return true
		}

		// First argument should be the resource name (string literal)
		if len(callExpr.Args) < 1 {
			return true
		}

		// Extract the resource name from the first argument
		var name string
		switch arg := callExpr.Args[0].(type) {
		case *ast.BasicLit:
			if arg.Kind == token.STRING {
				name = strings.Trim(arg.Value, `"`)
			}
		}

		if name == "" {
			return true
		}

		key := state.SeenKey(kind, name)
		if state.Seen[key] {
			return true
		}

		state.Seen[key] = true
		resource := &registry.ResourceInfo{
			Name:      name,
			Kind:      kind,
			FilePath:  filePath,
			SchemaPos: callExpr.Pos(),
		}
		resources = append(resources, resource)
		state.Resources = append(state.Resources, resource)

		return true
	})

	return resources
}

// extractResourceName tries to extract the resource name from the factory function.
// It first looks for Metadata method calls or TypeName assignments, then falls back to function name parsing.
func (r *ReturnTypeStrategy) extractResourceName(funcDecl *ast.FuncDecl, file *ast.File, kind registry.ResourceKind) string {
	funcName := funcDecl.Name.Name

	// Try to find the type being returned and look for its Metadata method
	if funcDecl.Body != nil {
		// Look for return statements that return a struct literal or &Type{}
		returnedType := extractReturnedTypeName(funcDecl.Body)
		if returnedType != "" {
			// Look for Metadata method on this type
			name := findMetadataTypeNameForType(file, returnedType)
			if name != "" {
				return name
			}
		}
	}

	// Fall back to extracting name from function name
	return extractNameFromFactoryFunc(funcName, kind)
}

// extractImportAliases builds a map from package alias to import path
func extractImportAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		importPath := strings.Trim(imp.Path.Value, `"`)
		var alias string
		if imp.Name != nil {
			alias = imp.Name.Name
		} else {
			// Default alias is last segment of path
			parts := strings.Split(importPath, "/")
			alias = parts[len(parts)-1]
		}
		aliases[alias] = importPath
	}
	return aliases
}

// classifyReturnType determines if a return type represents a resource, data source, or neither.
// Returns the kind and whether it's a resource type.
func classifyReturnType(returnType string, importAliases map[string]string) (registry.ResourceKind, bool) {
	// Remove pointer indicator
	returnType = strings.TrimPrefix(returnType, "*")

	// Check for plugin-framework resource types
	// Patterns: resource.Resource, datasource.DataSource
	if strings.HasSuffix(returnType, ".Resource") {
		pkg := strings.TrimSuffix(returnType, ".Resource")
		if importPath, ok := importAliases[pkg]; ok {
			if strings.Contains(importPath, "datasource") {
				return registry.KindDataSource, true
			}
			if strings.Contains(importPath, "resource") {
				return registry.KindResource, true
			}
		}
		// Heuristic: if package name suggests resource vs datasource
		if pkg == "datasource" {
			return registry.KindDataSource, true
		}
		if pkg == "resource" {
			return registry.KindResource, true
		}
	}

	// Check for plugin-framework datasource type explicitly
	if strings.HasSuffix(returnType, ".DataSource") {
		return registry.KindDataSource, true
	}

	// Check for SDK v2 schema.Resource
	// This is used for both resources and data sources in SDK v2
	if strings.HasSuffix(returnType, "schema.Resource") {
		// In SDK v2, both resources and data sources use *schema.Resource
		// We differentiate by function name or file name later
		return registry.KindResource, true
	}

	return registry.KindResource, false
}

// typeToString converts an ast.Expr type to a string representation
func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok {
			return x.Name + "." + t.Sel.Name
		}
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.InterfaceType:
		return "interface{}"
	}
	return ""
}

// extractReturnedTypeName finds the type name being instantiated in return statements
func extractReturnedTypeName(body *ast.BlockStmt) string {
	var typeName string
	ast.Inspect(body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return true
		}

		for _, result := range ret.Results {
			switch r := result.(type) {
			case *ast.UnaryExpr:
				// &TypeName{}
				if r.Op == token.AND {
					if comp, ok := r.X.(*ast.CompositeLit); ok {
						if ident, ok := comp.Type.(*ast.Ident); ok {
							typeName = ident.Name
							return false
						}
					}
				}
			case *ast.CompositeLit:
				// TypeName{}
				if ident, ok := r.Type.(*ast.Ident); ok {
					typeName = ident.Name
					return false
				}
			}
		}
		return true
	})
	return typeName
}

// findMetadataTypeNameForType looks for a Metadata method on the given type and extracts TypeName
func findMetadataTypeNameForType(file *ast.File, typeName string) string {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name.Name != "Metadata" {
			continue
		}

		recvType := getReceiverTypeName(funcDecl.Recv)
		if recvType != typeName {
			continue
		}

		if funcDecl.Body != nil {
			return extractTypeNameFromMetadata(funcDecl.Body)
		}
	}
	return ""
}

// extractNameFromFactoryFunc extracts a resource name from a factory function name
func extractNameFromFactoryFunc(funcName string, kind registry.ResourceKind) string {
	// Handle various patterns:
	// NewXxxResource -> xxx
	// NewXxxDataSource -> xxx
	// ResourceXxx -> xxx (SDK v2 pattern)
	// DataSourceXxx -> xxx (SDK v2 pattern)
	// dataSourceXxx -> xxx (SDK v2 camelCase pattern)
	// NewXxx -> xxx (when return type indicates resource/datasource)

	name := funcName

	// Remove common prefixes (case-sensitive patterns first, then case-insensitive)
	if strings.HasPrefix(name, "New") {
		name = strings.TrimPrefix(name, "New")
	} else if strings.HasPrefix(name, "Resource") {
		name = strings.TrimPrefix(name, "Resource")
	} else if strings.HasPrefix(name, "DataSource") {
		name = strings.TrimPrefix(name, "DataSource")
		kind = registry.KindDataSource
	} else if strings.HasPrefix(name, "dataSource") {
		// Handle SDK v2 camelCase pattern: dataSourceXxx
		name = strings.TrimPrefix(name, "dataSource")
		kind = registry.KindDataSource
	} else if strings.HasPrefix(name, "resource") {
		// Handle SDK v2 camelCase pattern: resourceXxx
		name = strings.TrimPrefix(name, "resource")
	}

	// Remove common suffixes
	name = strings.TrimSuffix(name, "Resource")
	name = strings.TrimSuffix(name, "DataSource")

	if name == "" {
		return ""
	}

	// Convert to snake_case
	return toSnakeCase(name)
}

// DefaultNestedSchemaPatterns returns the default patterns for identifying nested schema types.
// These patterns use glob-like syntax:
// - "*suffix" matches names ending with "suffix"
// - "*contains*" matches names containing "contains"
func DefaultNestedSchemaPatterns() []string {
	return []string{
		"*_schema",   // Suffix pattern: names ending with _schema
		"*_schema_*", // Contains pattern: names containing _schema_
	}
}

// isNestedSchemaType checks if a resource name represents a nested schema definition
// rather than a standalone resource. Uses default patterns.
func isNestedSchemaType(name string) bool {
	return isNestedSchemaTypeWithPatterns(name, DefaultNestedSchemaPatterns())
}

// isNestedSchemaTypeWithPatterns checks if a resource name matches any of the provided patterns.
// Pattern syntax:
// - "*suffix" matches names ending with "suffix" (e.g., "*_schema" matches "foo_schema")
// - "*contains*" matches names containing "contains" (e.g., "*_schema_*" matches "foo_schema_bar")
// - "prefix*" matches names starting with "prefix"
// - "exact" matches the exact name
func isNestedSchemaTypeWithPatterns(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchesNestedPattern(name, pattern) {
			return true
		}
	}
	return false
}

// matchesNestedPattern checks if a name matches a single pattern.
func matchesNestedPattern(name, pattern string) bool {
	// Handle different pattern types based on wildcard position
	starCount := strings.Count(pattern, "*")

	switch {
	case starCount == 0:
		// Exact match
		return name == pattern
	case starCount == 1 && strings.HasPrefix(pattern, "*"):
		// Suffix pattern: *suffix
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	case starCount == 1 && strings.HasSuffix(pattern, "*"):
		// Prefix pattern: prefix*
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	case starCount == 2 && strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*"):
		// Contains pattern: *contains*
		contains := strings.TrimPrefix(strings.TrimSuffix(pattern, "*"), "*")
		return strings.Contains(name, contains)
	default:
		// Unsupported pattern, skip
		return false
	}
}

// parseResources extracts all resources, data sources, and actions from a Go source file.
// It uses multiple detection strategies executed in priority order:
// 1. Schema() method on types ending with Resource/DataSource/Action
// 2. MetadataEntitySlug in factory functions (NewXxxDataSource, NewXxxResource)
// 3. Metadata() method with resp.TypeName assignment (preferred over Strategy 1)
// 4. NewXxxAction factory functions returning action.Action
// 5. Return type analysis for functions returning resource.Resource, datasource.DataSource, *schema.Resource
func parseResources(file *ast.File, fset *token.FileSet, filePath string) []*registry.ResourceInfo {
	// Initialize shared discovery state
	state := NewDiscoveryState()

	// Define strategies in execution order
	strategies := []DiscoveryStrategy{
		&SchemaMethodStrategy{},
		&FactoryFunctionStrategy{},
		&MetadataMethodStrategy{},
		&ActionFactoryStrategy{},
		&ReturnTypeStrategy{},
		&RegistryFactoryStrategy{},
	}

	// Execute each strategy in order
	for _, strategy := range strategies {
		strategy.Discover(file, fset, filePath, state)
	}

	// Post-processing: filter out nested schema types and check for ImportState
	var filtered []*registry.ResourceInfo
	for _, resource := range state.Resources {
		// Skip nested schema types (false positives)
		if isNestedSchemaType(resource.Name) {
			continue
		}

		if resource.Kind == registry.KindResource {
			resource.HasImportState = hasImportStateMethod(file, resource.Name)
		}
		filtered = append(filtered, resource)
	}

	return filtered
}

// extractActionName extracts the action name from a factory function name.
// Examples: NewJobAction -> job, NewWorkflowJobAction -> workflow_job
// Note: Actions may share base names with resources (e.g., both "job" resource and "job" action exist).
// The registry uses Kind to differentiate them.
func extractActionName(funcName string) string {
	// Remove "New" prefix and "Action" suffix
	name := strings.TrimPrefix(funcName, "New")
	name = strings.TrimSuffix(name, "Action")
	if name == "" {
		return ""
	}

	// Convert PascalCase to snake_case
	return toSnakeCase(name)
}

// extractMetadataEntitySlug extracts the resource name from MetadataEntitySlug in a function body.
// It looks for patterns like: MetadataEntitySlug: "organization"
func extractMetadataEntitySlug(body *ast.BlockStmt) string {
	var name string
	ast.Inspect(body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}

		// Check if key is MetadataEntitySlug
		if ident, ok := kv.Key.(*ast.Ident); ok {
			if ident.Name == "MetadataEntitySlug" {
				// Extract string value
				if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					name = strings.Trim(lit.Value, `"`)
					return false
				}
			}
		}
		return true
	})
	return name
}

// extractTypeNameFromMetadata extracts the resource name from resp.TypeName assignment.
// It looks for patterns like: resp.TypeName = "provider_name" or resp.TypeName = req.ProviderTypeName + "_name"
func extractTypeNameFromMetadata(body *ast.BlockStmt) string {
	var name string
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}

		// Check for resp.TypeName on LHS
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "resp" || sel.Sel.Name != "TypeName" {
			return true
		}

		// Try to extract the resource name from RHS
		switch rhs := assign.Rhs[0].(type) {
		case *ast.BasicLit:
			// Direct string assignment: resp.TypeName = "resource_name"
			if rhs.Kind == token.STRING {
				fullName := strings.Trim(rhs.Value, `"`)
				// Remove provider prefix if present (e.g., "provider_resource" -> "resource")
				if idx := strings.Index(fullName, "_"); idx > 0 {
					name = fullName[idx+1:]
				} else {
					name = fullName
				}
				return false
			}
		case *ast.BinaryExpr:
			// Concatenation: resp.TypeName = req.ProviderTypeName + "_name"
			if rhs.Op == token.ADD {
				if lit, ok := rhs.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					suffix := strings.Trim(lit.Value, `"`)
					// Remove leading underscore if present
					name = strings.TrimPrefix(suffix, "_")
					return false
				}
			}
		}
		return true
	})
	return name
}

// ParseTestFileWithConfig parses a test file with full configuration support.
// This is the main parsing function that all other parse functions delegate to.
func ParseTestFileWithConfig(file *ast.File, fset *token.FileSet, filePath string, config ParserConfig) *registry.TestFileInfo {
	packageName := ""
	if file.Name != nil {
		packageName = file.Name.Name
	}

	resourceName, isDataSource := extractResourceNameFromFilePath(filePath)

	// Build helper function maps:
	// - helperPatterns: function name -> resource type names (for legacy InferredResources)
	// - typedHelperPatterns: function name -> typed blocks (for InferredHCLBlocks)
	helperPatterns := buildHelperPatternMap(file)
	typedHelperPatterns := buildTypedHelperPatternMap(file)

	// Extract resource package aliases from imports (handles aliased imports like r "...helper/resource")
	resourceAliases := ExtractResourcePackageAliases(file)

	var testFuncs []registry.TestFunctionInfo

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok {
			return true
		}

		name := funcDecl.Name.Name

		// Must be a Test function (starts with "Test")
		if !strings.HasPrefix(name, "Test") {
			return true
		}

		// Content-based detection: check if the function calls resource.Test() or resource.ParallelTest()
		usesResourceTest := checkUsesResourceTestWithAliases(funcDecl.Body, config.CustomHelpers, config.LocalHelpers, resourceAliases)

		// When custom patterns are provided, they take precedence as a filter
		if len(config.TestNamePatterns) > 0 {
			// Must match custom pattern AND use resource test
			if !matchesTestPattern(name, config.TestNamePatterns) {
				return true
			}
			if !usesResourceTest {
				return true
			}
		} else {
			// No custom patterns - use content-based detection as primary method
			// This allows detection of tests like TestPrivateKeyRSA that don't follow
			// standard naming conventions but do call resource.Test()
			if !usesResourceTest {
				// Fall back to default pattern matching
				if !matchesTestPattern(name, nil) {
					return true
				}
				// If matches default pattern but doesn't use resource.Test(), skip
				return true
			}
		}

		steps, hasCheckDestroy, hasPreCheck, inferred, inferredBlocks := extractTestStepsWithHelpers(funcDecl.Body, helperPatterns, typedHelperPatterns)
		testFunc := registry.TestFunctionInfo{
			Name:              funcDecl.Name.Name,
			FilePath:          filePath,
			FunctionPos:       funcDecl.Pos(),
			UsesResourceTest:  true,
			TestSteps:         steps,
			HelperUsed:        detectHelperUsed(funcDecl.Body, config.LocalHelpers),
			HasCheckDestroy:   hasCheckDestroy,
			HasPreCheck:       hasPreCheck,
			InferredResources: inferred,
			InferredHCLBlocks: inferredBlocks,
		}

		for _, step := range testFunc.TestSteps {
			if step.ExpectError {
				testFunc.HasErrorCase = true
			}
			if step.ImportState {
				testFunc.HasImportStep = true
			}
		}

		testFuncs = append(testFuncs, testFunc)
		return true
	})

	// Return TestFileInfo even if no resource name extracted from filename
	// Resource association now happens via the Linker in Phase 3
	if len(testFuncs) == 0 {
		return nil
	}

	return &registry.TestFileInfo{
		FilePath:      filePath,
		PackageName:   packageName,
		ResourceName:  resourceName,
		IsDataSource:  isDataSource,
		TestFunctions: testFuncs,
	}
}

// parseTestFile parses a test file and extracts test function information.
// Deprecated: Use ParseTestFileWithConfig with DefaultParserConfig() instead.
func parseTestFile(file *ast.File, fset *token.FileSet, filePath string) *registry.TestFileInfo {
	return ParseTestFileWithConfig(file, fset, filePath, DefaultParserConfig())
}

// parseTestFileWithHelpers parses a test file with support for custom test helpers.
// Deprecated: Use ParseTestFileWithConfig instead.
func parseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *registry.TestFileInfo {
	config := ParserConfig{
		CustomHelpers: customHelpers,
	}
	return ParseTestFileWithConfig(file, fset, filePath, config)
}

// extractResourceNameFromFilePath extracts resource name from file path.
// This function delegates to ExtractResourceNameFromPath for the actual extraction logic.
func extractResourceNameFromFilePath(filePath string) (string, bool) {
	return ExtractResourceNameFromPath(filePath)
}

// findLocalTestHelpers discovers functions that wrap resource.Test().
func findLocalTestHelpers(files []*ast.File, fset *token.FileSet) []LocalHelper {
	var helpers []LocalHelper

	for _, file := range files {
		filePath := fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			funcDecl, ok := n.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				return true
			}

			name := funcDecl.Name.Name
			if strings.HasPrefix(name, "Test") {
				return true
			}
			if len(name) == 0 || (name[0] >= 'a' && name[0] <= 'z') {
				return true
			}
			if !acceptsTestingT(funcDecl) {
				return true
			}
			if !checkUsesResourceTest(funcDecl.Body) {
				return true
			}

			helpers = append(helpers, LocalHelper{
				Name:     name,
				FilePath: filePath,
				FuncDecl: funcDecl,
			})

			return true
		})
	}

	return helpers
}

// acceptsTestingT checks if a function has *testing.T as a parameter.
func acceptsTestingT(funcDecl *ast.FuncDecl) bool {
	if funcDecl == nil || funcDecl.Type == nil || funcDecl.Type.Params == nil {
		return false
	}

	for _, param := range funcDecl.Type.Params.List {
		if starExpr, ok := param.Type.(*ast.StarExpr); ok {
			if selExpr, ok := starExpr.X.(*ast.SelectorExpr); ok {
				if ident, ok := selExpr.X.(*ast.Ident); ok {
					if ident.Name == "testing" && selExpr.Sel.Name == "T" {
						return true
					}
				}
			}
		}
	}

	return false
}

// matchesExcludePattern checks if a file should be excluded.
func matchesExcludePattern(filePath string, patterns []string) ExclusionResult {
	baseName := filepath.Base(filePath)

	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, baseName); matched {
			return ExclusionResult{
				FilePath:       filePath,
				Excluded:       true,
				Reason:         "matched exclusion pattern",
				MatchedPattern: pattern,
			}
		}
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return ExclusionResult{
				FilePath:       filePath,
				Excluded:       true,
				Reason:         "matched exclusion pattern (full path)",
				MatchedPattern: pattern,
			}
		}
	}

	return ExclusionResult{FilePath: filePath, Excluded: false}
}

// buildRegistry constructs a resource registry by scanning all files.
// It uses a three-phase approach:
//  1. Scan for Resources (Type-based discovery via AST)
//  2. Scan ALL Test Files (unconditionally, to support function-first matching)
//  3. Link tests to resources using the Linker (function name, file proximity, fuzzy)
func BuildRegistry(pass *analysis.Pass, settings config.Settings) *registry.ResourceRegistry {
	reg := registry.NewResourceRegistry()

	// Discover local test helpers first
	localHelpers := findLocalTestHelpers(pass.Files, pass.Fset)

	// PHASE 1: Scan for Resources (Type-based discovery via AST)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if settings.ExcludeBaseClasses && IsBaseClassFile(filename) {
			continue
		}
		if settings.ExcludeSweeperFiles && IsSweeperFile(filename) {
			continue
		}
		if settings.ExcludeMigrationFiles && IsMigrationFile(filename) {
			continue
		}
		if shouldExcludeFile(filename, settings.ExcludePaths) {
			continue
		}
		// Check custom exclude patterns
		if len(settings.ExcludePatterns) > 0 {
			if result := matchesExcludePattern(filename, settings.ExcludePatterns); result.Excluded {
				continue
			}
		}

		resources := parseResources(file, pass.Fset, filename)
		for _, resource := range resources {
			reg.RegisterResource(resource)
		}
	}

	// PHASE 2: Scan ALL Test Files (unconditionally)
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if !strings.HasSuffix(filename, "_test.go") {
			continue
		}

		// Skip sweeper test files
		if settings.ExcludeSweeperFiles && IsSweeperFile(filename) {
			continue
		}

		// Check custom exclude patterns
		if len(settings.ExcludePatterns) > 0 {
			if result := matchesExcludePattern(filename, settings.ExcludePatterns); result.Excluded {
				continue
			}
		}

		// Parse test file with custom and local helpers and test name patterns
		config := ParserConfig{
			CustomHelpers:         settings.CustomTestHelpers,
			LocalHelpers:          localHelpers,
			TestNamePatterns:      settings.TestNamePatterns,
			TestFilePattern:       settings.TestFilePattern,
			ResourceNamingPattern: settings.ResourceNamingPattern,
			ProviderPrefix:        settings.ProviderPrefix,
			ResourcePathPattern:   settings.ResourcePathPattern,
			DataSourcePathPattern: settings.DataSourcePathPattern,
		}
		testFileInfo := ParseTestFileWithConfig(file, pass.Fset, filename, config)
		if testFileInfo == nil {
			continue
		}

		// Register each test function in global index
		for i := range testFileInfo.TestFunctions {
			fn := &testFileInfo.TestFunctions[i]
			fn.FilePath = filename
			reg.RegisterTestFunction(fn)
		}
	}

	// PHASE 3: Link tests to resources using the Linker
	linker := matching.NewLinker(reg, settings)
	linker.LinkTestsToResources()

	return reg
}

// matchesTestPattern checks if a function name matches the test patterns.
// If testNamePatterns is empty, it uses default patterns (TestAcc*, TestResource*, etc.)
func matchesTestPattern(funcName string, testNamePatterns []string) bool {
	// Always require "Test" prefix (capital T for exported tests)
	if !strings.HasPrefix(funcName, "Test") {
		return false
	}

	// If custom patterns are provided, check against them
	if len(testNamePatterns) > 0 {
		for _, pattern := range testNamePatterns {
			// Support glob-style patterns (* as wildcard)
			if strings.HasSuffix(pattern, "*") {
				prefix := strings.TrimSuffix(pattern, "*")
				if strings.HasPrefix(funcName, prefix) {
					return true
				}
			} else if funcName == pattern {
				return true
			}
		}
		return false
	}

	// Default patterns: TestAcc*, TestResource*, TestDataSource*, Test*_
	defaultPatterns := []string{
		"TestAcc",
		"TestResource",
		"TestDataSource",
	}

	for _, pattern := range defaultPatterns {
		if strings.HasPrefix(funcName, pattern) {
			return true
		}
	}

	// Also accept Test*_ pattern (e.g., TestWidget_basic)
	if strings.Contains(funcName, "_") {
		return true
	}

	return false
}

// MatchesTestPattern is the public API for checking test patterns.
func MatchesTestPattern(funcName string, testNamePatterns []string) bool {
	return matchesTestPattern(funcName, testNamePatterns)
}

// checkUsesResourceTest checks if a function body contains a call to resource.Test()
func checkUsesResourceTest(body *ast.BlockStmt) bool {
	return checkUsesResourceTestWithHelpers(body, nil)
}

// checkUsesResourceTestWithHelpers checks if a function body contains a call to resource.Test()
// or any of the custom test helper functions.
func checkUsesResourceTestWithHelpers(body *ast.BlockStmt, customHelpers []string) bool {
	return checkUsesResourceTestWithLocalHelpers(body, customHelpers, nil)
}

// ExtractResourcePackageAliases finds all import aliases for the terraform-plugin-testing resource package.
// Returns a set of package names/aliases that refer to the resource testing package.
func ExtractResourcePackageAliases(file *ast.File) map[string]bool {
	aliases := make(map[string]bool)

	// Known import paths for the resource testing package
	resourcePackagePaths := []string{
		"github.com/hashicorp/terraform-plugin-testing/helper/resource",
		"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource",
		"github.com/hashicorp/terraform-plugin-sdk/helper/resource",
	}

	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}

		// Remove quotes from import path
		importPath := strings.Trim(imp.Path.Value, `"`)

		// Check if this is a resource package import
		isResourcePkg := false
		for _, knownPath := range resourcePackagePaths {
			if importPath == knownPath {
				isResourcePkg = true
				break
			}
		}

		if !isResourcePkg {
			continue
		}

		// Determine the alias used
		if imp.Name != nil {
			// Explicit alias: import r "..."
			aliases[imp.Name.Name] = true
		} else {
			// Default: use last segment of path (typically "resource")
			parts := strings.Split(importPath, "/")
			if len(parts) > 0 {
				aliases[parts[len(parts)-1]] = true
			}
		}
	}

	return aliases
}

// checkUsesResourceTestWithLocalHelpers checks if a function body contains a call to resource.Test(),
// custom helpers, or local helpers.
func checkUsesResourceTestWithLocalHelpers(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper) bool {
	return checkUsesResourceTestWithAliases(body, customHelpers, localHelpers, nil)
}

// checkUsesResourceTestWithAliases checks if a function body contains a call to resource.Test(),
// custom helpers, local helpers, or calls using the provided package aliases.
// It also detects calls that pass resource.TestCase as an argument (e.g., acctest.VcrTest).
func checkUsesResourceTestWithAliases(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper, resourceAliases map[string]bool) bool {
	if body == nil {
		return false
	}

	localHelperNames := make(map[string]bool)
	for _, h := range localHelpers {
		localHelperNames[h.Name] = true
	}

	// If no aliases provided, use "resource" as default
	if resourceAliases == nil {
		resourceAliases = map[string]bool{"resource": true}
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				methodName := sel.Sel.Name
				// Check for Test(), ParallelTest(), or UnitTest() on the resource package
				if methodName == "Test" || methodName == "ParallelTest" || methodName == "UnitTest" {
					if resourceAliases[ident.Name] {
						found = true
						return false
					}
				}

				// Check custom helpers
				for _, helper := range customHelpers {
					parts := strings.SplitN(helper, ".", 2)
					if len(parts) == 2 {
						if ident.Name == parts[0] && sel.Sel.Name == parts[1] {
							found = true
							return false
						}
					}
				}
			}
		}

		if ident, ok := call.Fun.(*ast.Ident); ok {
			if localHelperNames[ident.Name] {
				found = true
				return false
			}
		}

		// Generic detection: check if any argument is resource.TestCase{...}
		// This catches wrapper functions like acctest.VcrTest(t, resource.TestCase{...})
		for _, arg := range call.Args {
			if hasTestCaseArg(arg, resourceAliases) {
				found = true
				return false
			}
		}

		return true
	})
	return found
}

// hasTestCaseArg checks if an expression is a resource.TestCase composite literal
func hasTestCaseArg(expr ast.Expr, resourceAliases map[string]bool) bool {
	compLit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}

	// Check if type is resource.TestCase or similar
	sel, ok := compLit.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	if sel.Sel.Name != "TestCase" {
		return false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	return resourceAliases[ident.Name]
}

// detectHelperUsed determines which helper function is used in a test function body.
func detectHelperUsed(body *ast.BlockStmt, localHelpers []LocalHelper) string {
	if body == nil {
		return ""
	}

	localHelperNames := make(map[string]bool)
	for _, h := range localHelpers {
		localHelperNames[h.Name] = true
	}

	var helperUsed string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest") {
					helperUsed = "resource." + sel.Sel.Name
					return false
				}
			}
		}

		if ident, ok := call.Fun.(*ast.Ident); ok {
			if localHelperNames[ident.Name] {
				helperUsed = ident.Name
				return false
			}
		}

		return true
	})

	return helperUsed
}

// buildHelperPatternMap scans a file for helper functions that return HCL strings
// and extracts resource/action patterns from them.
func buildHelperPatternMap(file *ast.File) map[string][]string {
	patterns := make(map[string][]string)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return true
		}

		funcName := funcDecl.Name.Name

		// Look for return statements with string literals or fmt.Sprintf
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok || len(ret.Results) == 0 {
				return true
			}

			for _, result := range ret.Results {
				extractPatternsFromExpr(result, func(pattern string) {
					patterns[funcName] = append(patterns[funcName], pattern)
				})
			}
			return true
		})
		return true
	})

	return patterns
}

// buildTypedHelperPatternMap builds a map from helper function names to typed HCL blocks.
// This preserves both block type (resource/data/action) and resource type information.
func buildTypedHelperPatternMap(file *ast.File) map[string][]InferredResource {
	patterns := make(map[string][]InferredResource)

	ast.Inspect(file, func(n ast.Node) bool {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return true
		}

		funcName := funcDecl.Name.Name

		// Look for return statements with string literals or fmt.Sprintf
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			ret, ok := n.(*ast.ReturnStmt)
			if !ok || len(ret.Results) == 0 {
				return true
			}

			for _, result := range ret.Results {
				extractTypedPatternsFromExpr(result, func(block InferredResource) {
					patterns[funcName] = append(patterns[funcName], block)
				})
			}
			return true
		})
		return true
	})

	return patterns
}

// extractPatternsFromExpr extracts resource/action patterns from an expression.
// It handles string literals, fmt.Sprintf calls, and string concatenation.
func extractPatternsFromExpr(expr ast.Expr, addPattern func(string)) {
	extractTypedPatternsFromExpr(expr, func(block InferredResource) {
		addPattern(block.ResourceType)
	})
}

// extractTypedPatternsFromExpr extracts typed HCL blocks (resource/data/action) from an expression.
// It handles string literals, any function calls with string arguments (fmt.Sprintf, acctest.Nprintf, etc.),
// and string concatenation.
func extractTypedPatternsFromExpr(expr ast.Expr, addBlock func(InferredResource)) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			content := strings.Trim(e.Value, "`\"")
			// Use HCLBlockRegex to capture both block type and resource type
			matches := HCLBlockRegex.FindAllStringSubmatch(content, -1)
			for _, match := range matches {
				if len(match) > 2 {
					addBlock(InferredResource{
						BlockType:    match[1], // "resource", "data", or "action"
						ResourceType: match[2], // e.g., "aws_instance"
					})
				}
			}
		}
	case *ast.CallExpr:
		// Handle any function call that takes a string argument containing HCL
		// This generically handles: fmt.Sprintf, acctest.Nprintf, custom helpers, etc.
		// Process all arguments - many formatting functions take the template as first arg
		for _, arg := range e.Args {
			extractTypedPatternsFromExpr(arg, addBlock)
		}
		// Also check if it's a function identifier (simple function call like myConfig())
		if ident, ok := e.Fun.(*ast.Ident); ok {
			_ = ident // Function call without selector - still process args above
		}
	case *ast.BinaryExpr:
		// Handle string concatenation
		if e.Op == token.ADD {
			extractTypedPatternsFromExpr(e.X, addBlock)
			extractTypedPatternsFromExpr(e.Y, addBlock)
		}
	}
}

// extractTestStepsWithHelpers is like extractTestSteps but also looks up helper patterns.
// Returns: steps, hasCheckDestroy, hasPreCheck, inferredResources (legacy), inferredHCLBlocks (typed)
func extractTestStepsWithHelpers(body *ast.BlockStmt, helperPatterns map[string][]string, typedHelperPatterns map[string][]InferredResource) ([]registry.TestStepInfo, bool, bool, []string, []registry.InferredHCLBlock) {
	var steps []registry.TestStepInfo
	var hasCheckDestroy bool
	var hasPreCheck bool
	uniqueInferred := make(map[string]bool)
	uniqueBlocks := make(map[string]registry.InferredHCLBlock) // key: "blockType:resourceType"
	stepNumber := 1

	ast.Inspect(body, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check for resource.Test() or resource.ParallelTest()
		if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "resource" && (sel.Sel.Name == "Test" || sel.Sel.Name == "ParallelTest" || sel.Sel.Name == "UnitTest") {
					// Direct resource.Test() call - TestCase is second argument
					if len(callExpr.Args) >= 2 {
						testSteps, foundCheckDestroy, foundPreCheck := extractStepsFromTestCaseWithHelpersTyped(callExpr.Args[1], &stepNumber, uniqueInferred, uniqueBlocks, helperPatterns, typedHelperPatterns)
						steps = append(steps, testSteps...)
						if foundCheckDestroy {
							hasCheckDestroy = true
						}
						if foundPreCheck {
							hasPreCheck = true
						}
					}
					return true
				}
			}
		}

		// Also check for wrapper functions like acctest.VcrTest(t, resource.TestCase{...})
		// Look for resource.TestCase composite literals in any function call arguments
		for _, arg := range callExpr.Args {
			if compLit, ok := arg.(*ast.CompositeLit); ok {
				// Check if it's a resource.TestCase type
				if sel, ok := compLit.Type.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok {
						if ident.Name == "resource" && sel.Sel.Name == "TestCase" {
							testSteps, foundCheckDestroy, foundPreCheck := extractStepsFromTestCaseWithHelpersTyped(compLit, &stepNumber, uniqueInferred, uniqueBlocks, helperPatterns, typedHelperPatterns)
							steps = append(steps, testSteps...)
							if foundCheckDestroy {
								hasCheckDestroy = true
							}
							if foundPreCheck {
								hasPreCheck = true
							}
						}
					}
				}
				// Also check for []resource.TestStep slice literals passed directly
				// This handles patterns like td.ResourceTest(t, []resource.TestStep{...})
				if arrayType, ok := compLit.Type.(*ast.ArrayType); ok {
					if sel, ok := arrayType.Elt.(*ast.SelectorExpr); ok {
						if ident, ok := sel.X.(*ast.Ident); ok {
							if ident.Name == "resource" && sel.Sel.Name == "TestStep" {
								// Extract steps directly from the slice literal
								extractedSteps := extractStepsFromSliceLiteral(compLit, &stepNumber, uniqueInferred, uniqueBlocks, helperPatterns, typedHelperPatterns)
								steps = append(steps, extractedSteps...)
							}
						}
					}
				}
			}
		}

		return true
	})

	var inferredResources []string
	for resourceName := range uniqueInferred {
		inferredResources = append(inferredResources, resourceName)
	}

	var inferredBlocks []registry.InferredHCLBlock
	for _, block := range uniqueBlocks {
		inferredBlocks = append(inferredBlocks, block)
	}

	return steps, hasCheckDestroy, hasPreCheck, inferredResources, inferredBlocks
}

// extractStepsFromTestCaseWithHelpers extracts steps and looks up helper patterns.
func extractStepsFromTestCaseWithHelpers(testCaseExpr ast.Expr, stepNumber *int, inferred map[string]bool, helperPatterns map[string][]string) ([]registry.TestStepInfo, bool, bool) {
	// Delegate to typed version and ignore the blocks
	blocks := make(map[string]registry.InferredHCLBlock)
	return extractStepsFromTestCaseWithHelpersTyped(testCaseExpr, stepNumber, inferred, blocks, helperPatterns, nil)
}

// extractStepsFromTestCaseWithHelpersTyped extracts steps with typed HCL block information.
func extractStepsFromTestCaseWithHelpersTyped(testCaseExpr ast.Expr, stepNumber *int, inferred map[string]bool, blocks map[string]registry.InferredHCLBlock, helperPatterns map[string][]string, typedHelperPatterns map[string][]InferredResource) ([]registry.TestStepInfo, bool, bool) {
	var steps []registry.TestStepInfo
	hasCheckDestroy := false
	hasPreCheck := false

	compLit, ok := testCaseExpr.(*ast.CompositeLit)
	if !ok {
		return steps, hasCheckDestroy, hasPreCheck
	}

	for _, elt := range compLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "CheckDestroy":
			hasCheckDestroy = true
		case "PreCheck":
			hasPreCheck = true
		case "Steps":
			stepsLit, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}

			for _, stepExpr := range stepsLit.Elts {
				step := parseTestStepWithHashAndHelpersTyped(stepExpr, *stepNumber, inferred, blocks, helperPatterns, typedHelperPatterns)
				steps = append(steps, step)
				*stepNumber++
			}
		}
	}

	for i := range steps {
		if i > 0 {
			steps[i].PreviousConfigHash = steps[i-1].ConfigHash
			steps[i].IsUpdateStepFlag = steps[i].DetermineIfUpdateStep(&steps[i-1])
		}
	}

	return steps, hasCheckDestroy, hasPreCheck
}

// extractStepsFromSliceLiteral extracts test steps directly from a []resource.TestStep slice literal.
// This handles patterns like td.ResourceTest(t, []resource.TestStep{...}) where steps are passed directly.
func extractStepsFromSliceLiteral(stepsLit *ast.CompositeLit, stepNumber *int, inferred map[string]bool, blocks map[string]registry.InferredHCLBlock, helperPatterns map[string][]string, typedHelperPatterns map[string][]InferredResource) []registry.TestStepInfo {
	var steps []registry.TestStepInfo

	for _, stepExpr := range stepsLit.Elts {
		step := parseTestStepWithHashAndHelpersTyped(stepExpr, *stepNumber, inferred, blocks, helperPatterns, typedHelperPatterns)
		steps = append(steps, step)
		*stepNumber++
	}

	// Set previous config hashes for update step detection
	for i := range steps {
		if i > 0 {
			steps[i].PreviousConfigHash = steps[i-1].ConfigHash
			steps[i].IsUpdateStepFlag = steps[i].DetermineIfUpdateStep(&steps[i-1])
		}
	}

	return steps
}

// parseTestStepWithHashAndHelpers parses a step and looks up helper patterns for Config.
func parseTestStepWithHashAndHelpers(stepExpr ast.Expr, stepNum int, inferred map[string]bool, helperPatterns map[string][]string) registry.TestStepInfo {
	blocks := make(map[string]registry.InferredHCLBlock)
	return parseTestStepWithHashAndHelpersTyped(stepExpr, stepNum, inferred, blocks, helperPatterns, nil)
}

// parseTestStepWithHashAndHelpersTyped parses a step with typed HCL block extraction.
func parseTestStepWithHashAndHelpersTyped(stepExpr ast.Expr, stepNum int, inferred map[string]bool, blocks map[string]registry.InferredHCLBlock, helperPatterns map[string][]string, typedHelperPatterns map[string][]InferredResource) registry.TestStepInfo {
	step := registry.TestStepInfo{
		StepNumber: stepNum,
	}

	stepLit, ok := stepExpr.(*ast.CompositeLit)
	if !ok {
		return step
	}

	if len(stepLit.Elts) > 0 {
		step.StepPos = stepLit.Elts[0].Pos()
	}

	for _, elt := range stepLit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "Config":
			step.HasConfig = true
			step.ConfigHash = hashConfigExpr(kv.Value)

			// Extract typed HCL blocks
			extractTypedPatternsFromExpr(kv.Value, func(block InferredResource) {
				if inferred != nil {
					inferred[block.ResourceType] = true
				}
				if blocks != nil {
					key := block.BlockType + ":" + block.ResourceType
					blocks[key] = registry.InferredHCLBlock{
						BlockType:    block.BlockType,
						ResourceType: block.ResourceType,
					}
				}
			})

			// If Config is a function call, look up helper patterns (both legacy and typed)
			if callExpr, ok := kv.Value.(*ast.CallExpr); ok {
				if ident, ok := callExpr.Fun.(*ast.Ident); ok {
					// Legacy string patterns (for InferredResources)
					if patterns, exists := helperPatterns[ident.Name]; exists {
						for _, p := range patterns {
							if inferred != nil {
								inferred[p] = true
							}
						}
					}
					// Typed patterns (for InferredHCLBlocks)
					if typedHelperPatterns != nil {
						if typedPatterns, exists := typedHelperPatterns[ident.Name]; exists {
							for _, block := range typedPatterns {
								if blocks != nil {
									key := block.BlockType + ":" + block.ResourceType
									blocks[key] = registry.InferredHCLBlock{
										BlockType:    block.BlockType,
										ResourceType: block.ResourceType,
									}
								}
							}
						}
					}
				}
			}
		case "Check":
			step.HasCheck = true
			step.CheckFunctions = extractCheckFunctions(kv.Value)
		case "ImportState":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.ImportState = ident.Name == "true"
			}
		case "ImportStateVerify":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.ImportStateVerify = ident.Name == "true"
			}
		case "ExpectError":
			step.ExpectError = true
		case "ExpectNonEmptyPlan":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.ExpectNonEmptyPlan = ident.Name == "true"
			}
		case "RefreshState":
			if ident, ok := kv.Value.(*ast.Ident); ok {
				step.RefreshState = ident.Name == "true"
			}
		case "ConfigPlanChecks":
			// Detect ConfigPlanChecks field (plan validation)
			step.HasPlanCheck = true
		case "ConfigStateChecks":
			// Detect ConfigStateChecks field (newer state validation pattern)
			step.HasConfigStateChecks = true
		}
	}

	return step
}

// extractResourceNamesFromConfigValue extracts resource/action names from a Config value.
func extractResourceNamesFromConfigValue(expr ast.Expr, inferred map[string]bool) {
	extractPatternsFromExpr(expr, func(pattern string) {
		inferred[pattern] = true
	})
}

// extractCheckFunctions extracts check function names from a Check field
func extractCheckFunctions(checkExpr ast.Expr) []string {
	var functions []string

	ast.Inspect(checkExpr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			functions = append(functions, sel.Sel.Name)
		}

		return true
	})

	return functions
}

// CheckHasSweepers scans a file for resource.AddTestSweepers calls.
// This is typically found in TestMain or init() functions.
func CheckHasSweepers(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if ident.Name == "resource" && sel.Sel.Name == "AddTestSweepers" {
						found = true
						return false
					}
				}
			}
		}
		return true
	})
	return found
}

// Public API functions

// ParseResources is the public API for parsing resources from a file.
func ParseResources(file *ast.File, fset *token.FileSet, filePath string) []*registry.ResourceInfo {
	return parseResources(file, fset, filePath)
}

// ParseTestFile is the public API for parsing test files.
func ParseTestFile(file *ast.File, fset *token.FileSet, filePath string) *registry.TestFileInfo {
	return parseTestFile(file, fset, filePath)
}

// ParseTestFileWithHelpers is the public API for parsing test files with custom helpers.
func ParseTestFileWithHelpers(file *ast.File, fset *token.FileSet, filePath string, customHelpers []string) *registry.TestFileInfo {
	return parseTestFileWithHelpers(file, fset, filePath, customHelpers)
}

// FindLocalTestHelpers is the public API for discovering local test helpers.
func FindLocalTestHelpers(files []*ast.File, fset *token.FileSet) []LocalHelper {
	return findLocalTestHelpers(files, fset)
}

// AcceptsTestingT is the public API for checking if a function accepts *testing.T.
func AcceptsTestingT(funcDecl *ast.FuncDecl) bool {
	return acceptsTestingT(funcDecl)
}

// MatchesExcludePattern is the public API for checking if a file should be excluded.
func MatchesExcludePattern(filePath string, patterns []string) ExclusionResult {
	return matchesExcludePattern(filePath, patterns)
}

// CheckUsesResourceTestWithLocalHelpers is the public API for checking helper usage.
func CheckUsesResourceTestWithLocalHelpers(body *ast.BlockStmt, customHelpers []string, localHelpers []LocalHelper) bool {
	return checkUsesResourceTestWithLocalHelpers(body, customHelpers, localHelpers)
}

// DetectHelperUsed is the public API for detecting helper function usage.
func DetectHelperUsed(body *ast.BlockStmt, localHelpers []LocalHelper) string {
	return detectHelperUsed(body, localHelpers)
}

// HashConfigExpr is the public API for hashing config expressions.
func HashConfigExpr(expr ast.Expr) string {
	return hashConfigExpr(expr)
}

// ParseProviderRegistryMaps extracts resource and data source names from provider registry map literals.
// This is useful for providers like Google that define resources in central map variables like:
//
//	var generatedResources = map[string]*schema.Resource{
//	    "google_compute_instance": compute.ResourceComputeInstance(),
//	    "google_iap_web_iam_binding": tpgiamresource.ResourceIamBinding(...),
//	}
//
// The function looks for map literals with string keys that look like Terraform resource names
// (containing underscores, typically prefixed with provider name).
func ParseProviderRegistryMaps(file *ast.File, fset *token.FileSet, filePath string) []*registry.ResourceInfo {
	var resources []*registry.ResourceInfo
	seen := make(map[string]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		// Look for variable declarations with map literal values
		genDecl, ok := n.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Check if this looks like a resource/datasource map
			// Variable names like: generatedResources, handwrittenResources, generatedIAMDatasources
			varName := ""
			if len(valueSpec.Names) > 0 {
				varName = valueSpec.Names[0].Name
			}

			// Determine kind from variable name
			var kind registry.ResourceKind
			isResourceMap := false
			if strings.Contains(strings.ToLower(varName), "datasource") ||
				strings.Contains(strings.ToLower(varName), "data_source") {
				kind = registry.KindDataSource
				isResourceMap = true
			} else if strings.Contains(strings.ToLower(varName), "resource") {
				kind = registry.KindResource
				isResourceMap = true
			}

			if !isResourceMap {
				continue
			}

			// Look for map literal in values
			for _, value := range valueSpec.Values {
				compLit, ok := value.(*ast.CompositeLit)
				if !ok {
					continue
				}

				// Check if it's a map type
				mapType, ok := compLit.Type.(*ast.MapType)
				if !ok {
					continue
				}

				// Verify key type is string
				keyIdent, ok := mapType.Key.(*ast.Ident)
				if !ok || keyIdent.Name != "string" {
					continue
				}

				// Extract resource names from map keys
				for _, elt := range compLit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}

					// Get the string key (resource name)
					keyLit, ok := kv.Key.(*ast.BasicLit)
					if !ok || keyLit.Kind != token.STRING {
						continue
					}

					// Remove quotes from string literal
					resourceName := strings.Trim(keyLit.Value, `"`)

					// Skip if doesn't look like a resource name (should have underscores)
					if !strings.Contains(resourceName, "_") {
						continue
					}

					// Use the full resource name from the registry map
					// This is the actual Terraform resource type (e.g., "google_bigquery_table")
					// which matches what appears in HCL configs

					// Skip if already seen (using full name)
					key := kind.String() + ":" + resourceName
					if seen[key] {
						continue
					}
					seen[key] = true

					resources = append(resources, &registry.ResourceInfo{
						Name:      resourceName,
						Kind:      kind,
						FilePath:  filePath,
						SchemaPos: keyLit.Pos(),
					})
				}
			}
		}

		return true
	})

	return resources
}
