#!/usr/bin/env bash
#
# Regenerate the committed coverage reports under specs/reports/ for every
# vendored validation provider under validation/terraform-provider-*.
#
# Reports are the project's regression-detection artifact: commit the refreshed
# reports and review the diff whenever discovery/matching/coverage behavior
# changes. Run from anywhere; paths are resolved relative to the repo root.
#
# Usage:
#   scripts/regenerate-reports.sh                 # regenerate all providers
#   scripts/regenerate-reports.sh tls powerhmc    # regenerate only the named providers
#
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

mkdir -p specs/reports

# Determine the provider list: explicit args, or every vendored provider.
if [ "$#" -gt 0 ]; then
  providers=("$@")
else
  providers=()
  for dir in validation/terraform-provider-*/; do
    [ -d "$dir" ] || continue
    name="$(basename "$dir")"
    providers+=("${name#terraform-provider-}")
  done
fi

for name in "${providers[@]}"; do
  provider_dir="validation/terraform-provider-${name}"
  report_file="specs/reports/${name}-report.txt"

  if [ ! -d "$provider_dir" ]; then
    echo "skip: ${name} (no ${provider_dir})" >&2
    continue
  fi

  echo "regenerating ${report_file} ..."
  # Use `go run` so no prebuilt binary is required or committed.
  if ! go run ./cmd/validate -provider "$provider_dir" -recursive -report > "$report_file" 2>/dev/null; then
    echo "warn: ${name} report generation exited non-zero (see ${report_file})" >&2
  fi
done

echo "done."
