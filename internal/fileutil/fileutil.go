// Package fileutil holds small, dependency-free string- and path-classification
// helpers shared by the discovery and matching packages. These were previously
// duplicated byte-for-byte in internal/discovery/utils.go and
// internal/matching/utils.go; this package is the single source of truth.
package fileutil

import (
	"path/filepath"
	"strings"
	"unicode"
)

// SnakeCase converts a CamelCase identifier to snake_case. It inserts an
// underscore before an uppercase rune when the previous rune is lowercase or
// the next rune is lowercase, so acronyms are handled (e.g. "HTTPServer" ->
// "http_server").
func SnakeCase(s string) string {
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			if unicode.IsLower(prev) || (i+1 < len(runes) && unicode.IsLower(runes[i+1])) {
				result.WriteRune('_')
			}
		}
		result.WriteRune(unicode.ToLower(r))
	}
	return result.String()
}

// TitleCase converts a snake_case identifier to CamelCase.
func TitleCase(s string) string {
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

// ExtractResourceName strips a Resource/DataSource/Action suffix from a Go type
// name and returns the snake_case resource name (e.g. "WidgetResource" ->
// "widget", "HttpDataSource" -> "http").
func ExtractResourceName(typeName string) string {
	name := strings.TrimSuffix(typeName, "Resource")
	name = strings.TrimSuffix(name, "DataSource")
	name = strings.TrimSuffix(name, "Action")
	return SnakeCase(name)
}

// IsBaseClassFile reports whether a file is a base-class file (base_*.go),
// typically an abstract type that should not be treated as a resource.
func IsBaseClassFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasPrefix(base, "base_") || strings.HasPrefix(base, "base.")
}

// IsSweeperFile reports whether a file is a sweeper file (*_sweeper.go), test
// infrastructure for cleaning up resources.
func IsSweeperFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_sweeper.go")
}

// IsMigrationFile reports whether a file is a state-migration file.
func IsMigrationFile(filePath string) bool {
	base := filepath.Base(filePath)
	return strings.HasSuffix(base, "_migrate.go") ||
		strings.Contains(base, "_migration") ||
		strings.HasSuffix(base, "_state_upgrader.go")
}

// ShouldExcludeFile reports whether a file path matches any of the given glob/
// substring exclude patterns (matched against the full path, the base name, or
// as a trailing-slash-trimmed substring).
func ShouldExcludeFile(filePath string, excludePaths []string) bool {
	for _, pattern := range excludePaths {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(filePath)); matched {
			return true
		}
		if strings.Contains(filePath, strings.TrimSuffix(pattern, "/")) {
			return true
		}
	}
	return false
}
