// Package matching implements resource matching strategies for test functions.
package matching

import (
	"path/filepath"
	"reflect"
	"strings"

	"github.com/example/tfprovidertest/internal/registry"
)

// Linker associates test functions with resources using multiple strategies.
// It implements a prioritized matching approach:
// 1. Function name extraction (highest confidence - TestAccWidgetResource_Basic)
// 2. File proximity matching (high confidence - widget_resource_test.go)
// 3. Inferred content matching (medium confidence - from Config string parsing)
// 4. Fuzzy matching (lowest confidence, optional)
type Linker struct {
	registry *registry.ResourceRegistry
	settings interface{} // Settings - using interface{} to avoid circular imports during migration
}

// NewLinker creates a new Linker instance.
func NewLinker(registry *registry.ResourceRegistry, settings interface{}) *Linker {
	return &Linker{
		registry: registry,
		settings: settings,
	}
}

// ResourceMatch represents a potential resource match for a test function.
type ResourceMatch struct {
	ResourceName string
	Confidence   float64
	MatchType    registry.MatchType
}

// LinkTestsToResources iterates over all test functions and associates them with resources.
// It uses multiple strategies in order of confidence to find the best match.
// Priority order (highest to lowest):
// 1. Inferred Content - based on actual HCL parsing of Config strings (most reliable)
// 2. Function name extraction - based on test function naming conventions
// 3. File proximity - based on test file naming conventions
// 4. Fuzzy matching - optional, disabled by default
func (l *Linker) LinkTestsToResources() {
	// Get all definitions and test functions
	allDefinitions := l.GetAllDefinitions()
	allTests := l.GetAllTestFunctions()

	// Build simple name map for quick lookup: "widget" -> true
	simpleNames := make(map[string]bool)
	for key := range allDefinitions {
		// Extract the simple name from compound keys like "resource:widget"
		if idx := strings.LastIndex(key, ":"); idx != -1 {
			simpleNames[key[idx+1:]] = true
		}
	}

	// Process each test function
	for _, fn := range allTests {
		var bestMatch *ResourceMatch
		matchFound := false

		// Build a set of inferred resources for quick lookup
		inferredSet := make(map[string]bool)
		for _, name := range fn.InferredResources {
			inferredSet[name] = true
			// Also add stripped version
			if idx := strings.Index(name, "_"); idx != -1 {
				inferredSet[name[idx+1:]] = true
			}
		}

		// Strategy 1: Function name extraction validated by InferredContent (HIGHEST confidence)
		// Combines the reliability of HCL parsing with the intent clarity of function naming
		// This solves the problem of tests that use multiple resources (e.g., group test uses inventory as dependency)
		if resourceName, found := matchResourceByName(fn.Name, simpleNames); found {
			// If we also have inferred resources, validate that this resource is in the config
			if len(fn.InferredResources) > 0 {
				// Check if function-derived resource is in inferred list
				if inferredSet[resourceName] {
					// Function name matches an inferred resource - highest confidence
					bestMatch = &ResourceMatch{
						ResourceName: resourceName,
						Confidence:   1.0,
						MatchType:    registry.MatchTypeInferred, // Use Inferred type since it's validated
					}
					matchFound = true
				}
			}
			// If no inferred resources or not in inferred set, still use function name match
			if !matchFound {
				bestMatch = &ResourceMatch{
					ResourceName: resourceName,
					Confidence:   0.95,
					MatchType:    registry.MatchTypeFunctionName,
				}
				matchFound = true
			}
		}

		// Strategy 2: Typed HCL Block Matching (exact matching using parsed block types)
		// Uses InferredHCLBlocks which contain both block type (resource/data/action) and resource type
		// This gives us exact matches without guessing based on function name hints
		if !matchFound && len(fn.InferredHCLBlocks) > 0 {
			// Map HCL block types to registry key prefixes
			blockTypeToPrefix := map[string]string{
				"resource": "resource:",
				"data":     "data source:",
				"action":   "action:",
			}

			// Priority order: actions (most specific) > resources > data sources (often dependencies)
			priorityOrder := []string{"action", "resource", "data"}

			for _, blockType := range priorityOrder {
				if matchFound {
					break
				}
				prefix := blockTypeToPrefix[blockType]
				for _, block := range fn.InferredHCLBlocks {
					if block.BlockType != blockType {
						continue
					}
					// Try exact match
					key := prefix + block.ResourceType
					if _, exists := allDefinitions[key]; exists {
						bestMatch = &ResourceMatch{
							ResourceName: block.ResourceType,
							Confidence:   1.0, // Exact match from HCL
							MatchType:    registry.MatchTypeInferred,
						}
						matchFound = true
						break
					}
					// Try stripping provider prefix
					if idx := strings.Index(block.ResourceType, "_"); idx != -1 {
						shortName := block.ResourceType[idx+1:]
						key = prefix + shortName
						if _, exists := allDefinitions[key]; exists {
							bestMatch = &ResourceMatch{
								ResourceName: shortName,
								Confidence:   1.0, // Exact match from HCL
								MatchType:    registry.MatchTypeInferred,
							}
							matchFound = true
							break
						}
					}
				}
			}
		}

		// Strategy 3: Legacy Inferred Content Matching (fallback for helper functions without direct HCL)
		if !matchFound && len(fn.InferredResources) > 0 {
			// Helper to match against a specific kind
			matchKind := func(kindPrefix string) bool {
				for _, inferredName := range fn.InferredResources {
					// First try the full name (e.g., "google_bigquery_table")
					// This matches resources registered with full names from provider registry maps
					key := kindPrefix + inferredName
					if _, exists := allDefinitions[key]; exists {
						bestMatch = &ResourceMatch{
							ResourceName: inferredName,
							Confidence:   0.85,
							MatchType:    registry.MatchTypeInferred,
						}
						return true
					}
					// Try stripping provider prefix (e.g., google_bigquery_table -> bigquery_table)
					if idx := strings.Index(inferredName, "_"); idx != -1 {
						shortName := inferredName[idx+1:]
						key = kindPrefix + shortName
						if _, exists := allDefinitions[key]; exists {
							bestMatch = &ResourceMatch{
								ResourceName: shortName,
								Confidence:   0.85,
								MatchType:    registry.MatchTypeInferred,
							}
							return true
						}
					}
				}
				return false
			}

			// Standard priority order: resources > actions > data sources
			if !matchFound {
				matchFound = matchKind("resource:")
			}
			if !matchFound {
				matchFound = matchKind("action:")
			}
			if !matchFound {
				matchFound = matchKind("data source:")
			}

			// Fallback: simple name matching (any kind)
			if !matchFound {
				for _, inferredName := range fn.InferredResources {
					if simpleNames[inferredName] {
						bestMatch = &ResourceMatch{
							ResourceName: inferredName,
							Confidence:   0.9,
							MatchType:    registry.MatchTypeInferred,
						}
						matchFound = true
						break
					}
					if idx := strings.Index(inferredName, "_"); idx != -1 {
						shortName := inferredName[idx+1:]
						if simpleNames[shortName] {
							bestMatch = &ResourceMatch{
								ResourceName: shortName,
								Confidence:   0.9,
								MatchType:    registry.MatchTypeInferred,
							}
							matchFound = true
							break
						}
					}
				}
			}
		}

		// Strategy 3: File proximity (medium confidence)
		// File names like widget_resource_test.go indicate the target resource
		if !matchFound {
			if resourceName := l.MatchByFileProximity(fn.FilePath, simpleNames); resourceName != "" {
				bestMatch = &ResourceMatch{
					ResourceName: resourceName,
					Confidence:   0.9,
					MatchType:    registry.MatchTypeFileProximity,
				}
				matchFound = true
			}
		}

		// Strategy 4: Fuzzy matching (low confidence, optional)
		if !matchFound && l.isFuzzyMatchingEnabled() {
			matches := l.findFuzzyMatches(fn.Name, simpleNames)
			if len(matches) > 0 {
				bestMatch = &matches[0]
				matchFound = true
			}
		}

		// Link the test to its matched resource
		if matchFound && bestMatch != nil {
			fn.MatchType = bestMatch.MatchType
			fn.MatchConfidence = bestMatch.Confidence
			l.LinkTestToResource(bestMatch.ResourceName, fn)
		}
	}
}

// isFuzzyMatchingEnabled checks if fuzzy matching is enabled in settings
func (l *Linker) isFuzzyMatchingEnabled() bool {
	// Try to cast settings to *config.Settings
	// We use interface{} to avoid circular imports during migration
	type settingsWithFuzzy interface {
		GetEnableFuzzyMatching() bool
	}

	// First try the interface method if available
	if s, ok := l.settings.(settingsWithFuzzy); ok {
		return s.GetEnableFuzzyMatching()
	}

	// Fallback for direct struct access using reflection
	if l.settings != nil {
		// Check if it has EnableFuzzyMatching field
		switch s := l.settings.(type) {
		case *struct{ EnableFuzzyMatching bool }:
			return s.EnableFuzzyMatching
		default:
			// Try via reflection if the type has EnableFuzzyMatching field
			val := reflect.ValueOf(l.settings)
			if val.Kind() == reflect.Ptr {
				val = val.Elem()
			}
			if val.Kind() == reflect.Struct {
				field := val.FieldByName("EnableFuzzyMatching")
				if field.IsValid() && field.Kind() == reflect.Bool {
					return field.Bool()
				}
			}
		}
	}

	return false
}

// GetAllDefinitions retrieves all definitions from the registry
func (l *Linker) GetAllDefinitions() map[string]*registry.ResourceInfo {
	return l.registry.GetAllDefinitions()
}

// GetAllTestFunctions retrieves all test functions from the registry
func (l *Linker) GetAllTestFunctions() []*registry.TestFunctionInfo {
	return l.registry.GetAllTestFunctions()
}

// LinkTestToResource links a test to a resource in the registry
func (l *Linker) LinkTestToResource(key string, fn *registry.TestFunctionInfo) {
	l.registry.LinkTestToResource(key, fn)
}

// findResourceMatches finds all matching resources for a test function.
// It tries strategies in order of confidence and returns early on high-confidence matches.
func (l *Linker) findResourceMatches(fn interface{}, resourceNames map[string]bool) []ResourceMatch {
	var matches []ResourceMatch

	// Strategy 0: Inferred Content Matching (highest priority)
	// Check if the test explicitly configures known resources in its Config strings.
	// This is the most reliable strategy as it comes from parsing the actual HCL configuration
	// that the test uses. If a test's Config string contains resource blocks, we know
	// definitively which resources it's testing.
	// We collect ALL matching inferred resources (not just the first one), because a test
	// may legitimately test multiple resources (e.g., an action test that also creates inventory).
	// We try both the full name (e.g., "aap_eda_eventstream_post") and the name without
	// provider prefix (e.g., "eda_eventstream_post").
	// TODO: Implement inferred resource matching after fixing registry imports

	// Strategy 1: Function name extraction (high confidence)
	// Always enabled as it's fast and accurate
	// Try all possible resource names from the function name
	// TODO: Implement function name matching after fixing registry imports

	// Strategy 2: File proximity (medium confidence)
	// Always enabled as it's fast and accurate
	// TODO: Implement file proximity matching after fixing registry imports

	// Strategy 3: Fuzzy matching (low confidence, optional)
	// Only runs if enabled (disabled by default due to performance cost and false positives)
	// TODO: Implement fuzzy matching after fixing registry imports

	_ = fn
	_ = resourceNames

	return matches
}

// MatchByFileProximity tries to match based on file naming convention.
// It uses ExtractResourceNameFromPath to handle all standard patterns:
// - resource_widget_test.go -> resource:widget
// - data_source_widget_test.go -> data:widget
// - ephemeral_widget_test.go -> resource:widget
// - widget_resource_test.go -> resource:widget
// - widget_data_source_test.go -> data:widget
// - widget_datasource_test.go -> data:widget
// - widget_action_test.go -> action:widget
// Returns the full key (kind:name) for proper linking when there are naming conflicts.
func (l *Linker) MatchByFileProximity(testFilePath string, resourceNames map[string]bool) string {
	// Use the centralized utility function to extract resource name and kind
	resourceName, isDataSource := ExtractResourceNameFromPath(testFilePath)

	// Check if the extracted name matches a known resource
	if resourceName != "" && resourceNames[resourceName] {
		// Return with kind prefix to ensure correct linking when both
		// resource and data source have the same name (e.g., "inventory")
		// Note: ResourceKind.String() returns "data source" with a space
		baseName := filepath.Base(testFilePath)
		if isDataSource {
			return "data source:" + resourceName
		}
		// Check if file indicates an action
		if strings.Contains(baseName, "_action") {
			return "action:" + resourceName
		}
		return "resource:" + resourceName
	}

	// Also try the raw name without prefix/suffix as fallback (returns simple name)
	baseName := filepath.Base(testFilePath)
	if strings.HasSuffix(baseName, "_test.go") {
		nameWithoutTest := strings.TrimSuffix(baseName, "_test.go")
		if resourceNames[nameWithoutTest] {
			return nameWithoutTest
		}
	}

	return ""
}

// findFuzzyMatches finds resources with similar names using Levenshtein distance.
func (l *Linker) findFuzzyMatches(funcName string, resourceNames map[string]bool) []ResourceMatch {
	var matches []ResourceMatch

	// Extract potential resource name from function
	resourceFromFunc, _ := ExtractResourceFromFuncName(funcName)
	if resourceFromFunc == "" {
		return matches
	}

	for resourceName := range resourceNames {
		confidence := CalculateSimilarity(resourceFromFunc, resourceName)
		// TODO: Use settings.FuzzyMatchThreshold after fixing imports
		if confidence >= 0.75 {
			matches = append(matches, ResourceMatch{
				ResourceName: resourceName,
				Confidence:   confidence,
				MatchType:    registry.MatchTypeFuzzy,
			})
		}
	}

	return matches
}

// CalculateSimilarity calculates string similarity using normalized Levenshtein distance.
// Returns a value between 0.0 (completely different) and 1.0 (identical).
func CalculateSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	distance := LevenshteinDistance(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// LevenshteinDistance calculates the Levenshtein distance between two strings.
// This is the minimum number of single-character edits (insertions, deletions, or
// substitutions) required to change one string into the other.
func LevenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = MinInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// MinInt returns the minimum of the given integers.
func MinInt(nums ...int) int {
	if len(nums) == 0 {
		return 0
	}
	m := nums[0]
	for _, n := range nums[1:] {
		if n < m {
			m = n
		}
	}
	return m
}

// DefaultFunctionNameKeywordsToStrip returns the default CamelCase keywords to strip
// from test function names before matching. This handles generated patterns.
func DefaultFunctionNameKeywordsToStrip() []string {
	return []string{
		"Generated", // Generated test suffix
	}
}

// matchResourceByName attempts to match a test function name to a resource name
// by stripping known prefixes and suffixes and converting to snake_case.
// Uses default keywords; for custom keywords use matchResourceByNameWithKeywords.
//
// For example:
//   - TestAccAwsS3Bucket_basic -> aws_s3_bucket
//   - TestAccResourceWidget_update -> widget
//   - TestDataSourceHTTP_complete -> http
//   - TestAccComputeDiskIamBinding -> compute_disk (with IAM keyword stripping)
//
// Returns the matched resource name and whether a match was found.
func matchResourceByName(funcName string, resourceNames map[string]bool) (string, bool) {
	return matchResourceByNameWithKeywords(funcName, resourceNames, DefaultFunctionNameKeywordsToStrip())
}

// matchResourceByNameWithKeywords attempts to match a test function name to a resource name
// using configurable keywords to strip from the function name.
func matchResourceByNameWithKeywords(funcName string, resourceNames map[string]bool, keywordsToStrip []string) (string, bool) {
	// Strip prefix
	name := funcName
	for _, prefix := range TestFunctionPrefixes {
		if strings.HasPrefix(name, prefix) {
			name = strings.TrimPrefix(name, prefix)
			break
		}
	}

	// If the name starts with an underscore after stripping prefix, skip it
	name = strings.TrimPrefix(name, "_")

	// Extract the resource part before any underscore suffix
	// e.g., "AwsS3Bucket_basic" -> "AwsS3Bucket"
	parts := strings.SplitN(name, "_", 2)
	resourcePart := parts[0]

	// Also try stripping known suffixes from the full name
	for _, suffix := range TestFunctionSuffixes {
		if strings.HasSuffix(name, suffix) {
			// Get the part before the suffix
			withoutSuffix := strings.TrimSuffix(name, suffix)
			// If this creates a valid snake_case, use it
			if withoutSuffix != "" && !strings.HasSuffix(withoutSuffix, "_") {
				parts = strings.SplitN(withoutSuffix, "_", 2)
				resourcePart = parts[0]
				break
			}
		}
	}

	// Convert to snake_case
	snakeName := toSnakeCase(resourcePart)

	// Check if it matches a registered resource
	if resourceNames[snakeName] {
		return snakeName, true
	}

	// Try without provider prefix (e.g., AwsS3Bucket -> s3_bucket)
	// This handles cases like "TestAccAWSInstance_basic" where AWS is the provider
	if len(snakeName) > 0 {
		// Split on first underscore and try the rest
		parts := strings.SplitN(snakeName, "_", 2)
		if len(parts) == 2 && parts[1] != "" {
			if resourceNames[parts[1]] {
				return parts[1], true
			}
		}
	}

	// Try stripping configurable CamelCase keywords (e.g., Iam, IamBinding, Generated)
	// This handles patterns like TestAccComputeDiskIamBinding -> compute_disk
	if len(keywordsToStrip) > 0 {
		modifiedPart := resourcePart
		for _, keyword := range keywordsToStrip {
			// Strip keyword from anywhere in the CamelCase name
			if strings.Contains(modifiedPart, keyword) {
				modifiedPart = strings.Replace(modifiedPart, keyword, "", 1)
			}
		}

		// If we modified the name, try matching again
		if modifiedPart != resourcePart && modifiedPart != "" {
			modifiedSnake := toSnakeCase(modifiedPart)

			// Check direct match
			if resourceNames[modifiedSnake] {
				return modifiedSnake, true
			}

			// Try without provider prefix
			parts := strings.SplitN(modifiedSnake, "_", 2)
			if len(parts) == 2 && parts[1] != "" {
				if resourceNames[parts[1]] {
					return parts[1], true
				}
			}
		}
	}

	// Strategy: Substring matching - try removing middle segments from snake_case
	// This handles patterns like TestAccBigQueryExternalDataTable -> big_query_table
	// by trying combinations that skip interior words
	if match, found := trySubstringMatches(snakeName, resourceNames); found {
		return match, true
	}

	// Also try substring matching without provider prefix
	parts = strings.SplitN(snakeName, "_", 2)
	if len(parts) == 2 && parts[1] != "" {
		if match, found := trySubstringMatches(parts[1], resourceNames); found {
			return match, true
		}
	}

	return "", false
}

// trySubstringMatches tries to match a snake_case name by removing middle segments.
// For "big_query_external_data_table", it will try:
// - big_query_table (skip external, data)
// - big_query_data_table (skip external)
// - big_query_external_table (skip data)
// etc.
func trySubstringMatches(snakeName string, resourceNames map[string]bool) (string, bool) {
	parts := strings.Split(snakeName, "_")
	if len(parts) <= 2 {
		return "", false // Need at least 3 parts for this to be useful
	}

	// Try keeping first part and last part, progressively adding more parts
	// This prioritizes shorter names (more likely to be actual resource names)
	first := parts[0]
	last := parts[len(parts)-1]

	// Try: first_last
	candidate := first + "_" + last
	if resourceNames[candidate] {
		return candidate, true
	}

	// Try: first_second_last (keeping first two and last)
	if len(parts) > 2 {
		candidate = first + "_" + parts[1] + "_" + last
		if resourceNames[candidate] {
			return candidate, true
		}
	}

	// Try: first_last_two (keeping first and last two)
	if len(parts) > 2 {
		candidate = first + "_" + parts[len(parts)-2] + "_" + last
		if resourceNames[candidate] {
			return candidate, true
		}
	}

	// Try keeping first two parts and last part
	if len(parts) > 3 {
		candidate = first + "_" + parts[1] + "_" + last
		if resourceNames[candidate] {
			return candidate, true
		}
	}

	// Try more combinations for longer names
	if len(parts) >= 4 {
		// first_second_last
		candidate = strings.Join([]string{parts[0], parts[1], parts[len(parts)-1]}, "_")
		if resourceNames[candidate] {
			return candidate, true
		}

		// first_second_third_last (skip middle)
		if len(parts) >= 5 {
			candidate = strings.Join([]string{parts[0], parts[1], parts[2], parts[len(parts)-1]}, "_")
			if resourceNames[candidate] {
				return candidate, true
			}
		}
	}

	return "", false
}

// MatchResourceByName is the public API for matching test function names to resources.
func MatchResourceByName(funcName string, resourceNames map[string]bool) (string, bool) {
	return matchResourceByName(funcName, resourceNames)
}

// ClassifyTest determines the category of a test function based on its characteristics.
// This helps separate provider tests, function tests, and resource tests.
func ClassifyTest(fn *registry.TestFunctionInfo) registry.TestCategory {
	// Check if test has resources - if so, it's a resource test
	if len(fn.InferredResources) > 0 {
		return registry.TestCategoryResource
	}

	name := fn.Name
	filePath := fn.FilePath

	// Check function name patterns for provider tests
	if strings.Contains(name, "ProviderConfig") ||
		strings.Contains(name, "ProviderMeta") ||
		strings.Contains(name, "ProviderBasePath") ||
		strings.Contains(name, "ProviderCredentials") ||
		strings.Contains(name, "ProviderEmptyStrings") ||
		strings.Contains(name, "UniverseDomain") ||
		strings.HasPrefix(name, "TestAccProvider") ||
		strings.HasPrefix(name, "TestAccFrameworkProvider") {
		return registry.TestCategoryProvider
	}

	// Check function name patterns for provider functions (Terraform 1.6+)
	if strings.Contains(name, "ProviderFunction") ||
		strings.Contains(name, "Parse_") ||
		strings.Contains(name, "Function_") ||
		strings.HasSuffix(name, "ParseFunction") {
		return registry.TestCategoryFunction
	}

	// Check file path patterns for functions
	if strings.Contains(filePath, "/functions/") ||
		strings.HasSuffix(filePath, "_function_test.go") ||
		strings.Contains(filePath, "/testing/") {
		return registry.TestCategoryFunction
	}

	// Check file path patterns for provider tests
	if strings.Contains(filePath, "provider_test.go") ||
		strings.Contains(filePath, "provider_config") ||
		strings.Contains(filePath, "framework_provider_test.go") ||
		strings.Contains(filePath, "universe_domain") {
		return registry.TestCategoryProvider
	}

	// Default to integration if we can't classify
	// This includes tests that don't configure any resources
	return registry.TestCategoryIntegration
}

// ClassifyAllTests classifies all test functions in the registry.
func (l *Linker) ClassifyAllTests() {
	allTests := l.GetAllTestFunctions()
	for _, fn := range allTests {
		fn.Category = ClassifyTest(fn)
	}
}

// GetUnmatchedResourceTests returns test functions that are resource tests but couldn't be matched.
// This filters out provider tests, function tests, etc. from the orphan count.
func (l *Linker) GetUnmatchedResourceTests() []*registry.TestFunctionInfo {
	var unmatched []*registry.TestFunctionInfo
	for _, fn := range l.GetAllTestFunctions() {
		if fn.MatchType == registry.MatchTypeNone && fn.Category == registry.TestCategoryResource {
			unmatched = append(unmatched, fn)
		}
	}
	return unmatched
}
