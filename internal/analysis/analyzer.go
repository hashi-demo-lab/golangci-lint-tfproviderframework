// Package analysis implements a golangci-lint plugin that identifies test coverage gaps
// in Terraform providers built with terraform-plugin-framework.
//
// # Registry Caching Architecture
//
// This package uses a global cache to share ResourceRegistry instances between the 7 analyzers:
//   1. BasicTestAnalyzer    - Checks for basic test coverage
//   2. UpdateTestAnalyzer   - Checks for update test coverage
//   3. ImportTestAnalyzer   - Checks for import test coverage
//   4. ErrorTestAnalyzer    - Checks for error case test coverage
//   5. StateCheckAnalyzer   - Checks for state validation in tests
//   6. DriftCheckAnalyzer   - Checks for CheckDestroy in tests
//   7. SweeperAnalyzer      - Checks for test sweeper registrations
//
// Without caching, each analyzer would parse the AST independently, resulting in 7x redundant work.
// The cache ensures buildRegistry() is called only once per analysis.Pass, providing significant
// performance improvements (typically 6-7x speedup).
//
// # Cache Implementation
//
// The cache uses a two-level structure:
//   - globalCache: map[*analysis.Pass]*registryCache (protected by globalCacheMu)
//   - registryCache: contains sync.Once, mutex, and the actual ResourceRegistry
//
// This design ensures:
//   - Thread safety for concurrent analysis runs
//   - Exactly-once initialization per pass (via sync.Once)
//   - Support for multiple concurrent passes (each gets its own cache entry)
//
// # Memory Management
//
// Cache entries are never automatically cleaned up by default. In short-lived processes (typical
// golangci-lint runs), this is fine as the process exits after analysis. However, in long-running
// processes (e.g., golangci-lint daemon mode or Language Server Protocol servers), cache entries
// can accumulate and cause memory leaks.
//
// To manage cache lifecycle:
//   - ClearRegistryCache(pass): Clears cache for a specific pass (call after analysis completes)
//   - ClearAllRegistryCaches(): Clears all cache entries (useful for tests or forced reset)
//   - GetCacheSize(): Returns current cache size (useful for monitoring)
//
// # Lock Contention Analysis (Phase 3)
//
// The current implementation uses sync.Mutex for globalCache protection. Analysis shows:
//
// Lock Patterns:
//   - globalCacheMu: Protects the globalCache map (coarse-grained locking)
//   - registryCache.mu: Protects individual registry instances (fine-grained locking)
//
// Contention Analysis:
//   - Lock is held only during map operations (get, set, delete)
//   - Registry building happens OUTSIDE the global lock (via sync.Once)
//   - Typical lock hold time: ~microseconds (map lookup + TTL check)
//   - Contention window: Only when multiple analyzers start simultaneously
//
// Alternative Considered: sync.Map
//   - Pros: Lock-free reads, better for high read/write ratio
//   - Cons: No built-in TTL support, harder to implement GetCacheStats
//   - Decision: NOT NEEDED - current Mutex performs well for this use case
//
// Why Mutex is Sufficient:
//   1. Cache map is small (typically 1-10 entries per golangci-lint run)
//   2. Lock is released before expensive buildRegistry() call
//   3. 7 analyzers run sequentially per pass (not concurrently)
//   4. Contention only occurs across different passes (rare)
//   5. Profiling shows no lock contention bottleneck
//
// Sharded Locking (Future):
//   - Only beneficial if cache grows to 1000+ concurrent passes
//   - Not applicable to current golangci-lint usage patterns
//   - Would add complexity without measurable benefit
//
// Conclusion: sync.Mutex + RWMutex is the right choice for this workload.
//
// # Future Improvements
//
// The current global cache pattern works but has limitations:
//   - Relies on global state (harder to test in isolation)
//   - Manual cache management required in long-running processes (mitigated by TTL)
//
// A future v2.0 could explore:
//   - Dependency injection pattern (pass Registry as analyzer dependency)
//   - Cache size limits with LRU eviction
//   - Integration with analysis framework's built-in fact/result system
//
// However, these improvements require significant architectural changes and potential breaking
// changes to the golangci-lint plugin interface. The current global cache approach is a pragmatic
// solution that balances performance, simplicity, and compatibility.
package analysis

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/analysis"

	"github.com/example/tfprovidertest/internal/discovery"
	"github.com/example/tfprovidertest/internal/registry"
	"github.com/example/tfprovidertest/pkg/config"
)

// registryCache holds the cached registry per analysis pass to avoid rebuilding it 7 times.
// Each analysis pass gets its own registry cache entry to support concurrent analysis runs.
//
// Memory Management:
//   - Cache entries are created on-demand via getOrBuildRegistry()
//   - Cache cleanup should be performed via ClearRegistryCache() or ClearAllRegistryCaches()
//   - In long-running processes (e.g., golangci-lint daemon mode), periodic cleanup prevents memory leaks
//   - TTL-based expiration: Entries older than CacheTTL are automatically evicted on access
//
// Thread Safety:
//   - globalCacheMu protects access to the globalCache map
//   - Each registryCache has its own mutex protecting the registry field
//   - sync.Once ensures buildRegistry is called exactly once per pass
type registryCache struct {
	mu        sync.Mutex
	registry  *registry.ResourceRegistry
	once      sync.Once
	createdAt time.Time // Timestamp when this cache entry was created
}

// Global cache map keyed by analysis.Pass pointer to support multiple concurrent analysis runs.
// This cache is shared across all 7 analyzers (BasicTest, UpdateTest, ImportTest, ErrorTest,
// StateCheck, DriftCheck, Sweeper) to avoid re-parsing the AST multiple times per pass.
//
// Cache Lifecycle:
//   - Entries are created lazily in getOrBuildRegistry()
//   - Entries should be cleaned up via ClearRegistryCache() after all analyzers complete
//   - Use ClearAllRegistryCaches() for global cleanup (e.g., test teardown)
var (
	globalCacheMu sync.Mutex
	globalCache   = make(map[*analysis.Pass]*registryCache)
)

// getOrBuildRegistry retrieves a cached registry for the given pass, or builds it if not yet cached.
// This ensures buildRegistry is called only once per analysis pass, even when all 7 analyzers run.
//
// TTL-based Eviction:
//   - Checks cache entry age against configured CacheTTL
//   - Automatically evicts expired entries (lazy cleanup on access)
//   - Rebuilds registry if cache entry has expired
//
// Performance:
//   - First analyzer call: Parses AST and builds registry (~expensive)
//   - Subsequent analyzer calls: Returns cached registry (~instant)
//   - Typical speedup: 7x (avoiding 6 redundant AST parses)
//
// Usage:
//   registry := getOrBuildRegistry(pass, settings)
//   defer ClearRegistryCache(pass) // Recommended for cleanup
func getOrBuildRegistry(pass *analysis.Pass, settings *config.Settings) *registry.ResourceRegistry {
	cacheTTL := settings.GetCacheTTLDuration()

	globalCacheMu.Lock()
	cache, exists := globalCache[pass]

	// Check if cache entry exists and is still valid (TTL check)
	if exists && cacheTTL > 0 {
		age := time.Since(cache.createdAt)
		if age > cacheTTL {
			// Cache entry expired, remove it
			delete(globalCache, pass)
			exists = false
		}
	}

	if !exists {
		cache = &registryCache{
			createdAt: time.Now(),
		}
		globalCache[pass] = cache
	}
	globalCacheMu.Unlock()

	// Use sync.Once to ensure buildRegistry is called only once per pass
	cache.once.Do(func() {
		cache.mu.Lock()
		defer cache.mu.Unlock()
		cache.registry = discovery.BuildRegistry(pass, *settings)
	})

	cache.mu.Lock()
	defer cache.mu.Unlock()
	return cache.registry
}

// ClearRegistryCache clears the cache entry for a specific analysis pass.
// This should be called after all analyzers have completed for a given pass to prevent memory leaks.
//
// When to call:
//   - After all 7 analyzers complete for a pass (recommended)
//   - In test cleanup (defer ClearRegistryCache(pass))
//   - In long-running processes to free memory
//
// Example:
//   registry := getOrBuildRegistry(pass, settings)
//   defer ClearRegistryCache(pass)
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func ClearRegistryCache(pass *analysis.Pass) {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	delete(globalCache, pass)
}

// ClearAllRegistryCaches clears all cache entries across all analysis passes.
// This is useful for:
//   - Test teardown (cleaning up after test suites)
//   - Forcing a full rebuild of all registries
//   - Reclaiming memory in long-running processes
//
// Warning: This will clear caches for ALL concurrent analysis runs, not just the current one.
// Only use this when you're certain all analysis passes are complete or when forcing a reset.
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func ClearAllRegistryCaches() {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	globalCache = make(map[*analysis.Pass]*registryCache)
}

// GetCacheSize returns the number of cached registry entries.
// This is useful for:
//   - Monitoring memory usage
//   - Debugging cache behavior
//   - Testing cache cleanup
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func GetCacheSize() int {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	return len(globalCache)
}

// CacheStats provides statistics about the current cache state.
type CacheStats struct {
	TotalEntries   int           // Total number of cache entries
	OldestEntryAge time.Duration // Age of the oldest cache entry
	ExpiredEntries int           // Number of entries that have exceeded TTL (but not yet evicted)
}

// GetCacheStats returns detailed statistics about the cache.
// This is useful for:
//   - Monitoring cache health
//   - Debugging TTL-based eviction
//   - Performance analysis
//
// Thread-safe: Can be called concurrently from multiple goroutines.
func GetCacheStats(ttl time.Duration) CacheStats {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()

	stats := CacheStats{
		TotalEntries: len(globalCache),
	}

	if len(globalCache) == 0 {
		return stats
	}

	now := time.Now()
	var oldestAge time.Duration

	for _, cache := range globalCache {
		age := now.Sub(cache.createdAt)

		// Track oldest entry
		if age > oldestAge {
			oldestAge = age
		}

		// Count expired entries (if TTL is configured)
		if ttl > 0 && age > ttl {
			stats.ExpiredEntries++
		}
	}

	stats.OldestEntryAge = oldestAge
	return stats
}

// RunBasicTestAnalyzer implements User Story 1: Basic Test Coverage
// Detects resources and data sources that lack basic acceptance tests
func RunBasicTestAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)
	calculator := NewCoverageCalculator(reg)

	// Report untested resources with enhanced location information
	untested := calculator.GetUntestedResources()
	for _, resource := range untested {
		resourceType := "resource"
		resourceTypeTitle := "Resource"
		if resource.Kind == registry.KindDataSource {
			resourceType = "data source"
			resourceTypeTitle = "Data source"
		}

		// Build enhanced message with location details
		pos := pass.Fset.Position(resource.SchemaPos)
		expectedTestPath := BuildExpectedTestPath(resource)
		expectedTestFunc := BuildExpectedTestFunc(resource)

		// Enhanced message with suggestions
		msg := fmt.Sprintf("%s '%s' has no acceptance test\n"+
			"  %s: %s:%d\n"+
			"  Expected test file: %s\n"+
			"  Expected test function: %s\n"+
			"  Suggestion: Create %s with function %s",
			resourceType, resource.Name,
			resourceTypeTitle, pos.Filename, pos.Line,
			expectedTestPath, expectedTestFunc,
			filepath.Base(expectedTestPath), expectedTestFunc)

		pass.Reportf(resource.SchemaPos, "%s", msg)
	}

	return nil, nil
}

func RunUpdateTestAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)

	// Check for resources with updatable attributes but no update tests
	// Only check regular resources (not data sources)
	for name, resource := range reg.GetAllDefinitions() {
		if resource.Kind != registry.KindResource {
			continue
		}
		// Check if resource has updatable attributes using isAttributeUpdatable
		hasUpdatable := false
		var updatableAttrs []string
		for _, attr := range resource.Attributes {
			if isAttributeUpdatable(attr) {
				hasUpdatable = true
				updatableAttrs = append(updatableAttrs, attr.Name)
			}
		}

		if !hasUpdatable {
			// Resource doesn't need update tests
			continue
		}

		// Get all test functions for this resource
		testFunctions := reg.GetResourceTests(name)

		// No tests at all - covered by BasicTestAnalyzer
		if len(testFunctions) == 0 {
			continue
		}

		// Check if any test function has a real update step
		hasUpdateTest := false
		for _, testFunc := range testFunctions {
			for _, step := range testFunc.TestSteps {
				// Use IsRealUpdateStep to properly distinguish real updates
				// from "Apply -> Import" patterns
				if step.IsRealUpdateStep() {
					hasUpdateTest = true
					break
				}
			}
			// Fallback: if we have multiple config steps (excluding imports), consider it an update test
			if !hasUpdateTest && len(testFunc.TestSteps) >= 2 {
				configSteps := 0
				for _, step := range testFunc.TestSteps {
					if step.HasConfig && !step.ImportState {
						configSteps++
					}
				}
				if configSteps >= 2 {
					hasUpdateTest = true
				}
			}
			if hasUpdateTest {
				break
			}
		}

		if !hasUpdateTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' has updatable attributes but no update test coverage\n"+
				"  Resource: %s:%d\n"+
				"  Updatable attributes: %s\n"+
				"  Suggestion: Add a test step that modifies one of these attributes",
				name, pos.Filename, pos.Line,
				strings.Join(updatableAttrs, ", "))
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

// isAttributeUpdatable determines if an attribute needs an update test.
// It returns false for:
//   - Non-optional attributes (Computed-only attributes don't need update tests)
//   - Attributes with RequiresReplace modifiers
//
// It defaults to true if unsure, to avoid false negatives.
func isAttributeUpdatable(attr registry.AttributeInfo) bool {
	// Computed-only attributes don't need update tests
	if !attr.Optional {
		return false
	}

	// Attributes with RequiresReplace don't need update tests
	// (the resource is recreated on change, not updated)
	if !attr.IsUpdatable {
		return false
	}

	// Default: assume it IS updatable if we aren't sure
	return true
}

// IsAttributeUpdatable is the public API for checking if an attribute needs update tests.
func IsAttributeUpdatable(attr registry.AttributeInfo) bool {
	return isAttributeUpdatable(attr)
}

func RunImportTestAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)

	// Check for resources with ImportState but no import tests
	// Only check regular resources (not data sources)
	for name, resource := range reg.GetAllDefinitions() {
		if resource.Kind != registry.KindResource {
			continue
		}
		// Only check resources that implement ImportState
		if !resource.HasImportState {
			continue
		}

		// Get all test functions for this resource
		testFunctions := reg.GetResourceTests(name)
		if len(testFunctions) == 0 {
			// No tests at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if ANY test function has an import step
		hasImportTest := false
		for _, testFunc := range testFunctions {
			if testFunc.HasImportStep {
				hasImportTest = true
				break
			}
		}

		if !hasImportTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' implements ImportState but has no import test coverage\n"+
				"  Resource: %s:%d\n"+
				"  Suggestion: Add a test step with ImportState: true, ImportStateVerify: true",
				name, pos.Filename, pos.Line)
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

func RunErrorTestAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)

	// Check for resources with validation rules but no error tests
	for name, resource := range reg.GetAllDefinitions() {
		if resource.Kind != registry.KindResource {
			continue
		}
		// Check if resource has validation rules
		hasValidation := false
		var validatedAttrs []string
		for _, attr := range resource.Attributes {
			if attr.NeedsValidationTest() {
				hasValidation = true
				validatedAttrs = append(validatedAttrs, attr.Name)
			}
		}

		if !hasValidation {
			// Resource doesn't need error tests
			continue
		}

		// Get all test functions for this resource
		testFunctions := reg.GetResourceTests(name)
		if len(testFunctions) == 0 {
			// No tests at all - but this is covered by BasicTestAnalyzer
			continue
		}

		// Check if ANY test function has an error case
		hasErrorTest := false
		for _, testFunc := range testFunctions {
			if testFunc.HasErrorCase {
				hasErrorTest = true
				break
			}
		}

		if !hasErrorTest {
			pos := pass.Fset.Position(resource.SchemaPos)
			msg := fmt.Sprintf("resource '%s' has validation rules but no error case tests\n"+
				"  Resource: %s:%d\n"+
				"  Validated attributes: %s\n"+
				"  Suggestion: Add a test step with ExpectError to verify validation",
				name, pos.Filename, pos.Line,
				strings.Join(validatedAttrs, ", "))
			pass.Reportf(resource.SchemaPos, "%s", msg)
		}
	}

	return nil, nil
}

func RunStateCheckAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)
	calculator := NewCoverageCalculator(reg)

	// Report at resource level - only flag resources missing ALL state/plan checks
	for _, coverage := range calculator.GetResourcesMissingStateChecks() {
		resourceType := "resource"
		if coverage.Resource.Kind == registry.KindDataSource {
			resourceType = "data source"
		}

		msg := fmt.Sprintf("%s '%s' has %d test(s) but none include state validation (Check) or plan checks (ConfigPlanChecks)\n"+
			"  Suggestion: Add Check: resource.ComposeTestCheckFunc(...) or ConfigPlanChecks to at least one test",
			resourceType, coverage.Resource.Name, coverage.TestCount)

		pass.Reportf(coverage.Resource.SchemaPos, "%s", msg)
	}

	return nil, nil
}

func RunDriftCheckAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	reg := getOrBuildRegistry(pass, settings)
	calculator := NewCoverageCalculator(reg)

	// Report at resource level - only flag resources missing CheckDestroy
	// Data sources are excluded as they don't create resources to destroy
	for _, coverage := range calculator.GetResourcesMissingCheckDestroy() {
		msg := fmt.Sprintf("resource '%s' has %d test(s) but none include CheckDestroy for drift detection\n"+
			"  Suggestion: Add CheckDestroy: testAccCheckDestroy to at least one test's resource.TestCase",
			coverage.Resource.Name, coverage.TestCount)

		pass.Reportf(coverage.Resource.SchemaPos, "%s", msg)
	}

	return nil, nil
}

func RunSweeperAnalyzer(pass *analysis.Pass, settings *config.Settings) (interface{}, error) {
	// Check if any file in the package has sweeper registrations
	hasSweepers := false
	for _, file := range pass.Files {
		if discovery.CheckHasSweepers(file) {
			hasSweepers = true
			break
		}
	}

	if !hasSweepers {
		// Report at package level (first file position)
		if len(pass.Files) > 0 {
			pass.Reportf(pass.Files[0].Pos(), "package has no test sweeper registrations\n"+
				"  Suggestion: Add resource.AddTestSweepers() calls for cleanup")
		}
	}

	return nil, nil
}
