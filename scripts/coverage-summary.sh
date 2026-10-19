#!/usr/bin/env bash
# Summarise a Go cover profile (as produced by `go test -coverprofile=...`) as a
# Markdown table: one row per package plus a total, weakest package first.
#
# Usage: scripts/coverage-summary.sh [coverage.out]
set -euo pipefail

profile="${1:-coverage.out}"

if [[ ! -f "$profile" ]]; then
  echo "coverage-summary: profile '$profile' not found" >&2
  exit 1
fi

# Drop the module prefix so the table shows `internal/resources/catalog`, `internal`, `.`.
module=$(go list -m 2>/dev/null || true)

# A profile line looks like:
#   github.com/x/y/internal/resources/catalog/resource.go:12.34,56.78 3 1
# $1 file:start,end   $2 statements   $3 hit count
awk -v mod="$module" '
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
' "$profile" | sort -k1,1n -k2,2 | awk -F'\t' '
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
