Based on a review of the provided source code, specifically parser.go, registry.go, analyzer.go, and the project structure, here are recommendations for architectural improvements to enhance maintainability, testability, and scalability.

1. Modularize Package Structure
   Currently, the core logic resides in a flat structure within the root package tfprovidertest. As the project has grown to ~6,000 LOC, this creates tight coupling and makes the API surface area unclear.

Recommendation: Refactor the root package into distinct sub-packages under an internal/ directory (to hide implementation details) or pkg/ (if reusable).

internal/discovery: Move parser.go and AST analysis logic here. This separates finding code from analyzing it.

internal/registry: Move registry.go here. This package would define the data model (ResourceInfo, TestFunctionInfo) and storage.

internal/matching: Move linker.go and utils.go (related to naming conventions) here.

internal/analysis: Move the actual check logic (analyzer.go) here.

Benefit: This enforces separation of concerns. For example, the parser shouldn't necessarily know about linker logic, and the registry should just be a dumb store.

2. Refactor parser.go using the Strategy Pattern
   The parseResources function in parser.go is becoming a complex monolith. It currently implements four distinct strategies (Schema method, Factory functions, Metadata method, Action factories) inside one function body.

Recommendation: Define a DiscoveryStrategy interface.

Go

type DiscoveryStrategy interface {
Discover(file *ast.File, fset *token.FileSet) []\*ResourceInfo
}
Implement separate strategies:

SchemaMethodStrategy

FrameworkFactoryStrategy (for New... functions)

MetadataMethodStrategy

Benefit: This makes parser.go easier to read and test. Adding a new way to detect resources (e.g., scanning for struct tags in the future) becomes adding a new struct rather than modifying a massive loop.

3. Decompose the "God Object" Registry
   The ResourceRegistry struct in registry.go handles storage, linking, concurrency locking, and coverage calculation. It knows too much about the relationships between objects.

Recommendation: Split responsibilities.

Storage: Keep Registry strictly for thread-safe storage of definitions.

Calculator: Create a CoverageCalculator service that takes a read-only view of the registry and computes the ResourceCoverage structs.

Linker: The Linker struct already exists, but it should be the only place where relationships between Tests and Resources are mutated, rather than the Registry having mixed mutation methods.

4. Standardize Configuration Loading
   The Settings struct handles configuration, but the logic for applying these settings is scattered. For instance, parser.go takes a ParserConfig struct which is manually constructed from Settings in analyzer.go.

Recommendation: Use a "Configuration Object" pattern that is passed down through the context or constructor chain.

Create a config package.

Ensure that ParserConfig and Settings are aligned so you don't need manual mapping code like:

Go

config := ParserConfig{
CustomHelpers: settings.CustomTestHelpers,
LocalHelpers: localHelpers,
TestNamePatterns: settings.TestNamePatterns,
}
Benefit: Reduces the risk of a setting added to Settings being ignored because it wasn't manually mapped to ParserConfig.

5. Improve State Management in Analyzers
   In analyzer.go, there is a globalCache map protected by a mutex. This is used to share the Registry between the five different analyzers (BasicTest, UpdateTest, etc.) to avoid re-parsing the AST 5 times.

Recommendation: While this works for golangci-lint plugins, it relies on global state. A cleaner approach for the architectural design (if the linter framework allows) is to create a "Runner" or "Coordinator" analyzer that:

Runs once.

Builds the Registry.

Outputs the Registry as its "Fact" (result).

Have the other 5 analyzers depend on the Coordinator analyzer and consume its output.

Benefit: This removes the need for globalCache, sync.Once, and manual cache clearing logic, leveraging the analysis framework's built-in dependency management instead.

6. Remove Shell Script Dependencies
   The root directory contains scripts like fix_settings.sh which contains embedded Go code in a heredoc. This indicates a complex build or generation process that is brittle.

Recommendation: Move any logic required to generate or fix code into a Go //go:generate directive or a strictly typed Makefile. Avoid embedding Go source code inside shell scripts strings.

7. Clean Up Deprecated Fields
   The ResourceInfo struct in registry.go contains fields marked as "Deprecated" or "New" alongside each other:

Go

Kind ResourceKind // New: replaces IsDataSource
IsDataSource bool // Deprecated
IsAction bool // Deprecated
Recommendation: Since this is a specialized linter likely used internally or in a specific ecosystem, aggressively refactor to remove the deprecated boolean flags and rely solely on the Kind enum. This prevents logic bugs where Kind says one thing but IsDataSource says another.
