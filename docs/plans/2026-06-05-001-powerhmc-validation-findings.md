# powerhmc Validation Findings (U1)

Companion to `2026-06-05-001-refactor-linter-correctness-powerhmc-validation-plan.md`.
Baseline report: `specs/reports/powerhmc-report.txt`. Provider: `validation/terraform-provider-powerhmc` (pure `terraform-plugin-framework`, 3 resources, 3 data-source files, 2 actions).

## Method
Generated the report via `go run ./cmd/validate -provider validation/terraform-provider-powerhmc -recursive -report`, then hand-verified every row against the provider source. The report is **substantially correct** — discovery, action handling, and config-inference matching all work well on a pure-framework provider. The defects below are real but narrower than the plan predicted; two were mis-predicted and one is new.

## Confirmed correct (lock with regression tests — U3/U8)
- **Discovery**: 3 resources (`lpar`, `sys_config`, `vios`), 3 data sources, 2 actions (`sys_on_off`, `vios_install`) — all found on a pure-framework provider with lowercase unexported receiver types.
- **Metadata TypeName override works**: `systemConfigResource`/`systemConfigDataSource` are reported as `sys_config` (from `Metadata()` `resp.TypeName = ..._sys_config`), correctly overriding the type-name guess `system_config`. `MetadataMethodStrategy` is doing its job.
- **Matching is robust**: all 17 tests matched via `inferred_from_config` (HCL block inference), **zero orphans** — name-based matching wasn't even needed. The plan's worry about name-extraction fragility did not manifest here.

## Defects (each mapped to a fix unit)

### F1 → U6 + U4: `hasImportStateMethod` receiver reconstruction is broken (plugin path only)
`internal/discovery/utils.go:107` computes `expectedType := toTitleCase(resourceName) + "Resource"`. For canonical name `sys_config` this yields `SysConfigResource`, but the actual receiver type is `systemConfigResource` — **never matches**, so `HasImportState=false` even though `systemConfigResource.ImportState` exists (`system_config_resource.go:386`).
- **Why the report still showed ImportState ✓**: the CLI table column uses `HasImportTest` (test-step derived, `cmd/validate/main.go:588,820`), not `HasImportState`. So the CLI masks the bug.
- **Where it bites**: the **analyzer/plugin path** uses the broken field (`internal/analysis/analyzer.go:435: if !resource.HasImportState`). Running as a golangci-lint plugin would emit a **false "missing ImportState"** diagnostic for `sys_config` (and any provider whose receiver name ≠ `TitleCase(canonical)+"Resource"`).
- **This is also a KTD-4 manifestation**: CLI (`HasImportTest`) and plugin (`HasImportState`) read *different* ImportState signals → divergent results for the same provider. Note there are **two** copies of `hasImportStateMethod` (`internal/discovery/utils.go:107` and `internal/matching/utils.go:425`) — both share the bug.
- **Fix (U6)**: resolve the receiver from the actual declaring type discovered for the resource, not a reconstructed name. Drive the U3 framework fixture from this exact shape (lowercase receiver + Metadata-renamed canonical name).

### F2 → U6 (new — not in original plan): `CheckDestroy: nil` counted as present
Report shows `sys_config` CheckDestroy ✓, but **every** `CheckDestroy` in `system_config_resource_test.go` is literally `CheckDestroy: nil` (lines 25, 89, 220, 262). Detection records `HasCheckDestroy` from the presence of the field key regardless of a `nil` value, so the summary "0 without CheckDestroy" is misleading — `sys_config` has no real destroy check. (`vios` ✓ is legitimate — it has `CheckDestroy: testAccCheckViosDestroy` at `vios_resource_test.go:24`.)
- **Fix (U6)**: treat `CheckDestroy: nil` as absent, mirroring the planned `t.Skip()` skip-detection. Same class of "field present but inert" false positive.

### F3 → U6 + U5/KTD-5: `lpar` data source reported but never registered
`lpar_data_source.go` is discovered and reported as a data source, but `DataSources()` (`provider.go:196`) registers only `NewSystemConfigDataSource` + `NewViosDataSource`. The linter reports **3** data sources; the provider **ships 2**.
- **Fix (U6)**: add the aggregator-reading strategy (`Resources()`/`DataSources()`/`Actions()`) and flag discovered-but-unregistered types rather than silently counting them as shipped.

## Plan corrections (feed forward into U6)
- The plan framed F1 as a report-visible false-negative; it is actually a **plugin-path-only** false-*positive* diagnostic, and is entangled with the U4 divergence. U6's ImportState test must assert the **analyzer** path, not just the CLI column.
- **F2 (CheckDestroy: nil) is net-new** — add it to U6 scope alongside skip-detection.
- The plan's pessimism about name-matching fragility was not borne out on this provider; keep U7 (FIX-010/011) but treat it as lower-confidence-of-impact here (no powerhmc evidence; prior-review evidence stands).

## Expected post-fix numbers (regression lock — U8)
After U6: `sys_config` CheckDestroy → ✗ (all nil); `lpar` data source → flagged discovered-but-unregistered; plugin-path ImportState for `sys_config`/`vios` → ✓ (method correctly detected). Resources still 3, actions 2, orphans 0.
