#!/usr/bin/env bash
# Merge several Go cover profiles into one, so the unit suite and the acceptance
# suite can be reported as a single statement-coverage number.
#
# All inputs must use the same cover mode (`-covermode=atomic`) and the same
# source revision: blocks are identified by their position range, and the hit
# counts are added.
#
# Usage: scripts/merge-coverage.sh <output.out> <input.out> [input.out ...]
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: scripts/merge-coverage.sh <output.out> <input.out> [input.out ...]" >&2
  exit 2
fi

output="$1"
shift

for profile in "$@"; do
  if [[ ! -f "$profile" ]]; then
    echo "merge-coverage: profile '$profile' not found" >&2
    exit 1
  fi
done

# Profile line: `github.com/x/y/internal/foo.go:12.34,56.78 3 1` (range, statements, hits).
awk '
  FNR == 1 && /^mode:/ { mode = $0; next }
  NF < 3 { next }
  { statements[$1] = $2; hits[$1] += $3 }
  END {
    print (mode == "" ? "mode: atomic" : mode)
    for (block in hits) printf "%s %d %d\n", block, statements[block], hits[block]
  }
' "$@" > "$output"

echo "merged $# profiles into $output"
