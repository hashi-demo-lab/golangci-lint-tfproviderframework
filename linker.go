// Package tfprovidertest implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
package tfprovidertest

import (
	"path/filepath"
	"strings"
)

// Linker associates test functions with resources using multiple strategies.
// It implements a prioritized matching approach:
// 0. Inferred content matching (highest priority - from Config string parsing)
// 1. Function name extraction (high confidence)
// 2. File proximity matching (medium confidence)
// 3. Fuzzy matching (lowest confidence, optional)
type Linker struct {
	registry *ResourceRegistry
	settings Settings
}

// NewLinker creates a new Linker instance.
func NewLinker(registry *ResourceRegistry, settings Settings) *Linker {
	return &Linker{
		registry: registry,
		settings: settings,
	}
}

// ResourceMatch represents a potential resource match for a test function.
type ResourceMatch struct {
	ResourceName string
	Confidence   float64
	MatchType    MatchType
}

// LinkTestsToResources iterates over all test functions and associates them with resources.
// It uses multiple strategies in order of confidence to find the best match.
func (l *Linker) LinkTestsToResources() {
	// Use unified definitions map instead of merging separate resources/dataSources maps
	allDefinitions := l.registry.GetAllDefinitions()

	// Build resource names set for matching
	resourceNames := make(map[string]bool, len(allDefinitions))
	for name := range allDefinitions {
		resourceNames[name] = true
	}

	for _, fn := range l.registry.GetAllTestFunctions() {
		matches := l.findResourceMatches(fn, resourceNames)

		// Update function with matches
		for _, match := range matches {
			fn.InferredResources = append(fn.InferredResources, match.ResourceName)
			fn.MatchConfidence = match.Confidence
			fn.MatchType = match.MatchType

			// Link to resource
			l.registry.LinkTestToResource(match.ResourceName, fn)
		}
	}
}

// findResourceMatches finds all matching resources for a test function.
// It tries strategies in order of confidence and returns early on high-confidence matches.
func (l *Linker) findResourceMatches(fn *TestFunctionInfo, resourceNames map[string]bool) []ResourceMatch {
	var matches []ResourceMatch

	// Strategy 0: Inferred Content Matching (highest priority)
	// Check if the test explicitly configures a known resource in its Config strings.
	// This is the most reliable strategy as it comes from parsing the actual HCL configuration
	// that the test uses. If a test's Config string contains a resource block, we know
	// definitively which resource it's testing.
	for _, inferredName := range fn.InferredResources {
		if resourceNames[inferredName] {
			matches = append(matches, ResourceMatch{
				ResourceName: inferredName,
				Confidence:   1.0,
				MatchType:    MatchTypeInferred,
			})
			return matches // High confidence match from config parsing
		}
	}

	// Strategy 1: Function name extraction (high confidence)
	// Always enabled as it's fast and accurate
	if resourceName, found := ExtractResourceFromFuncName(fn.Name); found {
		if resourceNames[resourceName] {
			matches = append(matches, ResourceMatch{
				ResourceName: resourceName,
				Confidence:   1.0,
				MatchType:    MatchTypeFunctionName,
			})
			return matches // High confidence match, no need to continue
		}
	}

	// Strategy 2: File proximity (medium confidence)
	// Always enabled as it's fast and accurate
	if resourceName := l.matchByFileProximity(fn.FilePath, resourceNames); resourceName != "" {
		matches = append(matches, ResourceMatch{
			ResourceName: resourceName,
			Confidence:   0.8,
			MatchType:    MatchTypeFileProximity,
		})
		return matches
	}

	// Strategy 3: Fuzzy matching (low confidence, optional)
	// Only runs if enabled (disabled by default due to performance cost and false positives)
	if l.settings.EnableFuzzyMatching {
		fuzzyMatches := l.findFuzzyMatches(fn.Name, resourceNames)
		for _, fm := range fuzzyMatches {
			if fm.Confidence >= l.settings.FuzzyMatchThreshold {
				matches = append(matches, fm)
			}
		}
	}

	return matches
}

// matchByFileProximity tries to match based on file naming convention.
// It uses ExtractResourceNameFromPath to handle all standard patterns:
// - resource_widget_test.go -> widget
// - data_source_widget_test.go -> widget
// - ephemeral_widget_test.go -> widget
// - widget_resource_test.go -> widget
// - widget_data_source_test.go -> widget
// - widget_datasource_test.go -> widget
func (l *Linker) matchByFileProximity(testFilePath string, resourceNames map[string]bool) string {
	// Use the centralized utility function to extract resource name
	resourceName, _ := ExtractResourceNameFromPath(testFilePath)

	// Check if the extracted name matches a known resource
	if resourceName != "" && resourceNames[resourceName] {
		return resourceName
	}

	// Also try the raw name without prefix/suffix as fallback
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
		confidence := calculateSimilarity(resourceFromFunc, resourceName)
		if confidence >= l.settings.FuzzyMatchThreshold {
			matches = append(matches, ResourceMatch{
				ResourceName: resourceName,
				Confidence:   confidence,
				MatchType:    MatchTypeFuzzy,
			})
		}
	}

	return matches
}

// calculateSimilarity calculates string similarity using normalized Levenshtein distance.
// Returns a value between 0.0 (completely different) and 1.0 (identical).
func calculateSimilarity(a, b string) float64 {
	if a == b {
		return 1.0
	}

	distance := levenshteinDistance(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistance calculates the Levenshtein distance between two strings.
// This is the minimum number of single-character edits (insertions, deletions, or
// substitutions) required to change one string into the other.
func levenshteinDistance(a, b string) int {
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
			matrix[i][j] = minInt(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

// minInt returns the minimum of the given integers.
func minInt(nums ...int) int {
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

// matchResourceByName attempts to match a test function name to a resource name
// by stripping known prefixes and suffixes and converting to snake_case.
//
// For example:
//   - TestAccAwsS3Bucket_basic -> aws_s3_bucket
//   - TestAccResourceWidget_update -> widget
//   - TestDataSourceHTTP_complete -> http
//
// Returns the matched resource name and whether a match was found.
func matchResourceByName(funcName string, resourceNames map[string]bool) (string, bool) {
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

	return "", false
}

// MatchResourceByName is the public API for matching test function names to resources.
func MatchResourceByName(funcName string, resourceNames map[string]bool) (string, bool) {
	return matchResourceByName(funcName, resourceNames)
}
