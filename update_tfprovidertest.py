#!/usr/bin/env python3
"""
Script to add missing functionality to tfprovidertest.go
"""

import re

# Read the file
with open('tfprovidertest.go', 'r') as f:
    content = f.read()

# 1. Add shouldExcludeFile function after isBaseClassFile
should_exclude_func = '''
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
'''

# Find the end of isBaseClassFile function and add shouldExcludeFile after it
pattern = r'(// isBaseClassFile checks if a file is a base class file that should be excluded\nfunc isBaseClassFile\(filePath string\) bool \{\n\tbase := filepath\.Base\(filePath\)\n\treturn strings\.HasPrefix\(base, "base_"\) \|\| strings\.HasPrefix\(base, "base\."\)\n})'
replacement = r'\1' + should_exclude_func
content = re.sub(pattern, replacement, content)

# 2. Update parseTestFile to accept customPatterns parameter
content = re.sub(
    r'// T016: Test file parser - now supports multiple naming conventions\nfunc parseTestFile\(file \*ast\.File, fset \*token\.FileSet, filePath string\) \*TestFileInfo \{',
    '// T016: Test file parser - now supports multiple naming conventions\nfunc parseTestFile(file *ast.File, fset *token.FileSet, filePath string, customPatterns []string) *TestFileInfo {',
    content
)

# 3. Update the call to isTestFunction within parseTestFile to use customPatterns
content = re.sub(
    r'(\t\t// Check if this is a test function using flexible matching\n\t\tif !isTestFunction\(name, )nil(\) \{)',
    r'\1customPatterns\2',
    content
)

# Write the file back
with open('tfprovidertest.go', 'w') as f:
    f.write(content)

print("Successfully updated tfprovidertest.go")
print("Added shouldExcludeFile function")
print("Updated parseTestFile signature to accept customPatterns")
print("Updated isTestFunction call to use customPatterns")
