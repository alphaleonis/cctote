#!/usr/bin/env bash
# check-coverage.sh — Enforce per-package coverage thresholds.
# Fails if any package drops below its minimum. Run via: mise run coverage
set -euo pipefail

# Per-package minimum coverage (statement %). Adjust as coverage improves.
declare -A THRESHOLDS=(
  [github.com/alphaleonis/cctote/cmd]=80
  [github.com/alphaleonis/cctote/internal/buildinfo]=60
  [github.com/alphaleonis/cctote/internal/claude]=85
  [github.com/alphaleonis/cctote/internal/cliutil]=90
  [github.com/alphaleonis/cctote/internal/config]=80
  [github.com/alphaleonis/cctote/internal/engine]=85
  [github.com/alphaleonis/cctote/internal/fileutil]=70
  [github.com/alphaleonis/cctote/internal/manifest]=80
  [github.com/alphaleonis/cctote/internal/mcp]=80
  [github.com/alphaleonis/cctote/internal/tui]=30
  [github.com/alphaleonis/cctote/internal/ui]=90
)

failures=0

# Run tests with coverage and parse each package's result.
while IFS= read -r line; do
  # Match lines like: ok  github.com/alphaleonis/cctote/cmd  0.2s  coverage: 83.8% of statements
  if [[ $line =~ ^ok[[:space:]]+([^[:space:]]+)[[:space:]].*coverage:[[:space:]]+([0-9]+(\.[0-9]+)?)% ]]; then
    pkg="${BASH_REMATCH[1]}"
    pct="${BASH_REMATCH[2]}"

    threshold="${THRESHOLDS[$pkg]:-}"
    if [[ -z $threshold ]]; then
      continue
    fi

    # Compare using awk for floating-point.
    if awk "BEGIN { exit !($pct < $threshold) }"; then
      printf "FAIL  %-50s %5.1f%% < %d%% minimum\n" "$pkg" "$pct" "$threshold"
      ((failures++))
    else
      printf "ok    %-50s %5.1f%% >= %d%%\n" "$pkg" "$pct" "$threshold"
    fi
  fi
done < <(go test -count=1 ./... -coverprofile=coverage.out 2>&1)

echo ""
if ((failures > 0)); then
  echo "FAIL: $failures package(s) below coverage threshold"
  exit 1
else
  echo "All packages meet coverage thresholds."
fi
