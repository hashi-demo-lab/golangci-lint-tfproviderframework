# Validation harness — guidance

The `validation/` tree holds vendored Terraform provider sources used to
exercise the linter end-to-end. Each `terraform-provider-<name>/` is recorded
as a git **gitlink** (mode 160000) — the provider's own checkout embedded in
this repo — and its committed coverage report lives at
`specs/reports/<name>-report.txt`. The reports are the project's
**regression-detection artifact**: refresh them and review the diff whenever
discovery, matching, or coverage behavior changes.

## Onboarding a new validation provider

1. Copy the provider source into `validation/terraform-provider-<name>/`
   (mirrors the existing providers; git records it as a gitlink).
2. Generate its report:
   `scripts/regenerate-reports.sh <name>`
   (writes `specs/reports/<name>-report.txt` via `go run ./cmd/validate`).
3. **Hand-verify** the report against the source — do not just trust that the
   tool ran. Check resource/data-source/action counts, that `Metadata()`
   TypeName overrides are honored, and that CheckDestroy/ImportState columns
   reflect reality (see the powerhmc findings doc for the kinds of defects this
   catches: `docs/plans/2026-06-05-001-powerhmc-validation-findings.md`).
4. Add a row to `validation/VALIDATION_REPORT.md`.
5. For a deterministic provider, consider a Go regression-lock test asserting
   `analysis.CoverageCalculator.Summarize()` counts (see
   `powerhmc_regression_test.go`).

## Regenerating reports

- `scripts/regenerate-reports.sh` — all vendored providers.
- `scripts/regenerate-reports.sh tls powerhmc` — only the named providers.
- Reports use repo-relative `validation/...` header paths.

## Invariants worth protecting (where bugs cluster)

- **Discovery duplicate-prevention.** The six discovery strategies share a
  `DiscoveryState` (`Seen`, `ProcessedFactoryFuncs`, `RecvTypeToIndex`). Any new
  strategy that appends to `state.Resources` MUST consult these, or it
  reintroduces duplicate resources. `MetadataMethodStrategy` is authoritative
  for the canonical name and overrides the type-derived guess.
- **One registry build path.** Both the golangci-lint plugin and the CLI go
  through `discovery.BuildRegistryFromFiles`. Do not reintroduce a second,
  divergent construction path — that previously caused the plugin to miss
  central-map resources and skip test classification.
- **One coverage computation.** Summary counts come from
  `analysis.CoverageCalculator.Summarize()`. The CLI table and JSON renderers
  both consume it; do not recompute inline (they previously disagreed on
  action plan-checks).
- **Inert values are not coverage.** A literal `CheckDestroy: nil` is not a
  destroy check (`isNilIdent`). Watch for the same "field present but inert"
  trap with other fields and with `t.Skip()`-gated tests.
- **ImportState receiver.** Detect ImportState against the *actual* receiver
  type (from `RecvTypeToIndex`), never a reconstructed `Title+"Resource"` name
  — unexported and Metadata-renamed receivers will not match the reconstruction.

## Known issues

- **Nondeterministic reports** for providers with a same-named resource across
  multiple files (e.g. `tls` `private_key`). The FILE column varies between runs
  via map iteration order. Do not refresh affected reports until fixed. See the
  follow-up task and `VALIDATION_REPORT.md` addendum.
- `terraform-provider-random` is absent; its `specs/reports/random-report.txt`
  is an error stub. Add the source or remove the stub.
