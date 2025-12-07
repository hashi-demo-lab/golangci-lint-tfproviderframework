#!/bin/bash

# This script applies all the necessary changes to tfprovidertest.go

# First, add the shouldExcludeFile function after isBaseClassFile
sed -i '/^\/\/ isBaseClassFile checks if a file is a base class file that should be excluded$/,/^}$/ {
  /^}$/a\
\
// shouldExcludeFile checks if a file path matches any of the exclude patterns\
func shouldExcludeFile(filePath string, excludePaths []string) bool {\
\tfor _, pattern := range excludePaths {\
\t\t// Try matching the full path\
\t\tif matched, _ := filepath.Match(pattern, filePath); matched {\
\t\t\treturn true\
\t\t}\
\t\t// Try matching just the base name\
\t\tif matched, _ := filepath.Match(pattern, filepath.Base(filePath)); matched {\
\t\t\treturn true\
\t\t}\
\t\t// Try matching with Contains for patterns like "vendor/"\
\t\tif strings.Contains(filePath, strings.TrimSuffix(pattern, "/")) {\
\t\t\treturn true\
\t\t}\
\t}\
\treturn false\
}
}' tfprovidertest.go

echo "Changes applied successfully"
