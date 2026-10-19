#!/usr/bin/env bash
# Summarise a Go cover profile (as produced by `go test -coverprofile=...`) as a
# Markdown table: one row per package plus a total, weakest package first.
#
# The table sums the statement column of the profile, which is only exact on a
# profile that has one line per block: `go test -coverpkg=...` writes one line
# per block per instrumented package and relies on `go tool cover` to
# deduplicate them, so a raw `-coverpkg` profile holds each block many times.
# Such a profile is rejected here (exit 2) rather than summarised into a number
# that counts those blocks repeatedly; pass it through
# `scripts/merge-coverage.sh` first, which keys blocks by position.
#
# Usage: scripts/coverage-summary.sh [--tsv] [coverage.out]
#
# --tsv prints `scope<TAB>percent<TAB>covered<TAB>statements` rows, weakest
# scope first, for scripts/coverage-gate.sh to compare against its floors.
set -euo pipefail

tsv=0
profile=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tsv) tsv=1 ;;
    -h | --help)
      echo "usage: scripts/coverage-summary.sh [--tsv] [coverage.out]"
      exit 0
      ;;
    -*)
      echo "coverage-summary: unknown flag '$1'" >&2
      exit 2
      ;;
    *)
      if [[ -n "$profile" ]]; then
        echo "coverage-summary: too many profiles ('$profile' and '$1')" >&2
        exit 2
      fi
      profile="$1"
      ;;
  esac
  shift
done
profile="${profile:-coverage.out}"

if [[ ! -f "$profile" ]]; then
  echo "coverage-summary: profile '$profile' not found" >&2
  exit 1
fi

duplicate="$(awk 'NF >= 3 { if (seen[$1]++) { print $1; exit } }' "$profile")"
if [[ -n "$duplicate" ]]; then
  echo "coverage-summary: '$profile' lists the block '$duplicate' more than once" >&2
  echo "coverage-summary: summing the statement column is only exact on a profile with one line per block" >&2
  echo "coverage-summary: merge it first (scripts/merge-coverage.sh <output> '$profile'), or read it with 'go tool cover -func'" >&2
  exit 2
fi

# Drop the module prefix so the table shows `internal/resources/catalog`, `internal`, `.`.
module=$(go list -m 2>/dev/null || true)

# A profile line looks like:
#   github.com/x/y/internal/resources/catalog/resource.go:12.34,56.78 3 1
# $1 file:start,end   $2 statements   $3 hit count
rows="$(awk -v mod="$module" '
  /^mode:/ { next }
  NF < 3 { next }
  {
    file = $1
    sub(/:[0-9]+\.[0-9]+,[0-9]+\.[0-9]+$/, "", file)
    pkg = file
    if (sub(/\/[^\/]+$/, "", pkg) == 0) pkg = "."
    if (mod != "" && index(pkg, mod) == 1) pkg = substr(pkg, length(mod) + 1)
    sub(/^\//, "", pkg)
    if (pkg == "") pkg = "."
    statements[pkg] += $2
    if ($3 + 0 > 0) covered[pkg] += $2
    total_statements += $2
    if ($3 + 0 > 0) total_covered += $2
  }
  END {
    for (pkg in statements) {
      pct = (statements[pkg] > 0) ? 100 * covered[pkg] / statements[pkg] : 0
      printf "%07.2f\t%s\t%d\t%d\n", pct, pkg, covered[pkg], statements[pkg]
    }
    pct = (total_statements > 0) ? 100 * total_covered / total_statements : 0
    printf "%07.2f\tTOTAL\t%d\t%d\n", pct, total_covered, total_statements
  }
' "$profile" | sort -k1,1n -k2,2)"

if [[ "$tsv" -eq 1 ]]; then
  printf '%s\n' "$rows" | awk -F'\t' '
    NF >= 4 {
      scope = ($2 == "TOTAL") ? "total" : $2
      printf "%s\t%.2f\t%s\t%s\n", scope, $1, $3, $4
    }
  '
  exit 0
fi

printf '%s\n' "$rows" | awk -F'\t' '
  BEGIN {
    print "| Package | Coverage | Statements |"
    print "|---------|----------|------------|"
  }
  $2 == "TOTAL" { total = $0; next }
  { printf "| `%s` | %.1f%% | %d/%d |\n", $2, $1, $3, $4 }
  END {
    if (total != "") {
      split(total, row, "\t")
      printf "\n| **Total** | **%.1f%%** | **%d/%d** |\n", row[1], row[3], row[4]
    }
  }
'
