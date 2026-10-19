#!/usr/bin/env bash
# Regenerate the coverage badge and the coverage block in README.md from
# .github/coverage-baseline.txt.
#
# The README regions are generated, not hand-written, so the published number
# cannot drift from the number the coverage gate enforces: both read the same
# baseline file.
#
# Usage:
#   scripts/coverage-readme.sh [README.md] [baseline]            # rewrite README.md
#   scripts/coverage-readme.sh --check [README.md] [baseline]    # exit 1 when stale
#
# Exit codes:
#   0  README.md is in sync (--check) or was rewritten
#   1  README.md is out of date (--check only)
#   2  configuration or marker error; nothing was written
#
# Baseline format. Every `measured_<scope>` key names a scope and must have a
# matching `floor_<scope>` key. `_` in the key is the path separator in the
# rendered label, so `measured_internal_client` renders `internal/client`;
# `measured_total` renders `total` and is required (it drives the badge). The
# keys `measured_commit`, `measured_date`, `target` and `gold` are required too.
set -euo pipefail

readme="README.md"
baseline=".github/coverage-baseline.txt"
check=0
positional=0

usage() {
  echo "usage: scripts/coverage-readme.sh [--check] [README.md] [baseline]" >&2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) check=1 ;;
    -h | --help) usage; exit 0 ;;
    -*)
      echo "coverage-readme: unknown flag '$1'" >&2
      usage
      exit 2
      ;;
    *)
      case "$positional" in
        0) readme="$1" ;;
        1) baseline="$1" ;;
        *)
          echo "coverage-readme: too many arguments" >&2
          usage
          exit 2
          ;;
      esac
      positional=$((positional + 1))
      ;;
  esac
  shift
done

[[ -f "$readme" ]] || { echo "coverage-readme: '$readme' not found" >&2; exit 2; }
[[ -f "$baseline" ]] || { echo "coverage-readme: '$baseline' not found" >&2; exit 2; }

trim() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

scopes=()
floor_scopes=()
seen_keys=$'\n'
measured_commit=""
measured_date=""
target=""
gold=""

note_key() {
  local key="$1"
  if [[ "$seen_keys" == *$'\n'"$key"$'\n'* ]]; then
    echo "coverage-readme: '$baseline' defines '$key' more than once" >&2
    exit 2
  fi
  seen_keys+="$key"$'\n'
}

lineno=0
while IFS= read -r raw || [[ -n "$raw" ]]; do
  lineno=$((lineno + 1))
  line="$(trim "${raw%$'\r'}")"
  [[ -z "$line" ]] && continue
  [[ "$line" == \#* ]] && continue

  if [[ "$line" != *=* ]]; then
    echo "coverage-readme: $baseline:$lineno is not 'key=value'" >&2
    exit 2
  fi

  key="$(trim "${line%%=*}")"
  value="$(trim "${line#*=}")"
  if [[ -z "$key" || -z "$value" ]]; then
    echo "coverage-readme: $baseline:$lineno has an empty key or value" >&2
    exit 2
  fi

  case "$key" in
    measured_commit)
      note_key "$key"
      measured_commit="$value"
      ;;
    measured_date)
      note_key "$key"
      measured_date="$value"
      ;;
    target)
      note_key "$key"
      target="$value"
      ;;
    gold)
      note_key "$key"
      gold="$value"
      ;;
    measured_*)
      scope="${key#measured_}"
      if [[ ! "$scope" =~ ^[A-Za-z0-9_]+$ ]]; then
        echo "coverage-readme: $baseline:$lineno has an invalid scope '$scope'" >&2
        exit 2
      fi
      note_key "$key"
      scopes+=("$scope")
      printf -v "value_measured_$scope" '%s' "$value"
      ;;
    floor_*)
      scope="${key#floor_}"
      if [[ ! "$scope" =~ ^[A-Za-z0-9_]+$ ]]; then
        echo "coverage-readme: $baseline:$lineno has an invalid scope '$scope'" >&2
        exit 2
      fi
      note_key "$key"
      floor_scopes+=("$scope")
      printf -v "value_floor_$scope" '%s' "$value"
      ;;
    *)
      echo "coverage-readme: $baseline:$lineno has an unknown key '$key'" >&2
      exit 2
      ;;
  esac
done < "$baseline"

if (( ${#scopes[@]} == 0 )); then
  echo "coverage-readme: '$baseline' defines no measured_<scope> keys" >&2
  exit 2
fi
if [[ -z "$measured_commit" || -z "$measured_date" ]]; then
  echo "coverage-readme: '$baseline' needs measured_commit and measured_date" >&2
  exit 2
fi
if [[ -z "$target" || -z "$gold" ]]; then
  echo "coverage-readme: '$baseline' needs target and gold" >&2
  exit 2
fi

value_of() {
  local prefix="$1" scope="$2" name value
  name="value_${prefix}_${scope}"
  value="${!name-}"
  printf '%s' "$value"
}

has_total=0
for scope in ${scopes[@]+"${scopes[@]}"}; do
  [[ "$scope" == total ]] && has_total=1
done
if [[ "$has_total" -eq 0 ]]; then
  echo "coverage-readme: '$baseline' needs a measured_total scope" >&2
  exit 2
fi

# Every measured scope is required to have a floor, and every floor must name a
# measured scope. A typo must be a configuration error, never a silently
# unrendered (or silently unenforced) scope.
for scope in ${scopes[@]+"${scopes[@]}"}; do
  if [[ -z "$(value_of floor "$scope")" ]]; then
    echo "coverage-readme: '$baseline' has measured_$scope but no floor_$scope" >&2
    exit 2
  fi
done
for scope in ${floor_scopes[@]+"${floor_scopes[@]}"}; do
  found=0
  for measured_scope in ${scopes[@]+"${scopes[@]}"}; do
    [[ "$scope" == "$measured_scope" ]] && found=1
  done
  if [[ "$found" -eq 0 ]]; then
    echo "coverage-readme: '$baseline' has floor_$scope but no measured_$scope" >&2
    exit 2
  fi
done

require_pct() {
  local label="$1" value="$2"
  if [[ ! "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    echo "coverage-readme: '$baseline' has a non-numeric $label '$value'" >&2
    exit 2
  fi
  if ! awk -v v="$value" 'BEGIN { exit !(v >= 0 && v <= 100) }'; then
    echo "coverage-readme: '$baseline' has $label '$value' outside 0..100" >&2
    exit 2
  fi
}

require_pct target "$target"
require_pct gold "$gold"
if ! awk -v t="$target" -v g="$gold" 'BEGIN { exit !(t < g) }'; then
  echo "coverage-readme: '$baseline' needs target ($target) below gold ($gold)" >&2
  exit 2
fi
for scope in ${scopes[@]+"${scopes[@]}"}; do
  require_pct "measured_$scope" "$(value_of measured "$scope")"
  require_pct "floor_$scope" "$(value_of floor "$scope")"
done

# Baseline order, with total last so the chart reads as a progression.
ordered=()
for scope in ${scopes[@]+"${scopes[@]}"}; do
  [[ "$scope" == total ]] && continue
  ordered+=("$scope")
done
ordered+=(total)

display_name() {
  case "$1" in
    total) printf 'total' ;;
    *) printf '%s' "${1//_//}" ;;
  esac
}

names=()
width=0
for scope in "${ordered[@]}"; do
  name="$(display_name "$scope")"
  names+=("$name")
  if (( ${#name} > width )); then
    width=${#name}
  fi
done

bar() {
  awk -v pct="$1" -v width="${2:-20}" 'BEGIN {
    filled = int(pct * width / 100 + 0.5)
    if (filled > width) filled = width
    s = ""
    for (i = 0; i < filled; i++) s = s "\342\226\210"
    for (i = filled; i < width; i++) s = s "\342\226\221"
    print s
  }'
}

color() {
  awk -v pct="$1" 'BEGIN {
    if (pct >= 90) print "brightgreen"
    else if (pct >= 80) print "green"
    else if (pct >= 70) print "yellow"
    else if (pct >= 60) print "orange"
    else print "red"
  }'
}

tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/coverage-readme.XXXXXX")"
trap 'rm -rf "$tmpdir"' EXIT
badge_file="$tmpdir/badge"
block_file="$tmpdir/block"
step1="$tmpdir/step1"
step2="$tmpdir/step2"

printf '[![Coverage](https://img.shields.io/badge/coverage-%s%%25-%s)](#coverage)\n' \
  "$(value_of measured total)" "$(color "$(value_of measured total)")" > "$badge_file"

{
  echo '```'
  index=0
  for scope in "${ordered[@]}"; do
    name="${names[$index]}"
    measured="$(value_of measured "$scope")"
    floor="$(value_of floor "$scope")"
    if [[ "$scope" == total ]]; then
      printf '%-*s  %s  %s%%   floor %s%%   -> %s%%, then %s%%\n' \
        "$width" "$name" "$(bar "$measured")" "$measured" "$floor" "$target" "$gold"
    else
      printf '%-*s  %s  %s%%   floor %s%%\n' \
        "$width" "$name" "$(bar "$measured")" "$measured" "$floor"
    fi
    index=$((index + 1))
  done
  echo '```'
  echo
  # shellcheck disable=SC2016  # the backticks are literal Markdown
  echo 'Merged statement coverage from `coverage.out` (unit) and `coverage-acceptance.out`'
  # shellcheck disable=SC2016  # the backticks are literal Markdown
  echo '(acceptance, `-covermode=atomic -coverpkg=./internal/...`) via `scripts/merge-coverage.sh`'
  # shellcheck disable=SC2016  # the backticks are literal Markdown
  echo '(`make coverage-merge`).'
  echo
  # shellcheck disable=SC2016  # the backticks are literal Markdown
  printf 'Measured on `%s` (unit + acceptance) on %s.\n' "$measured_commit" "$measured_date"
} > "$block_file"

marker_count() {
  awk -v marker="$1" '$0 == marker { count++ } END { print count + 0 }' "$2"
}

marker_line() {
  awk -v marker="$1" '$0 == marker { print NR; exit }' "$2"
}

require_marker() {
  local marker="$1" file="$2" count
  count="$(marker_count "$marker" "$file")"
  case "$count" in
    0)
      echo "coverage-readme: '$file' has no '$marker' marker" >&2
      return 2
      ;;
    1) return 0 ;;
    *)
      echo "coverage-readme: '$file' has $count '$marker' markers (expected exactly one)" >&2
      return 2
      ;;
  esac
}

require_region() {
  local file="$1" region="$2" start end start_line end_line
  start="<!-- $region:start -->"
  end="<!-- $region:end -->"
  require_marker "$start" "$file" || return 2
  require_marker "$end" "$file" || return 2
  start_line="$(marker_line "$start" "$file")"
  end_line="$(marker_line "$end" "$file")"
  if (( start_line >= end_line )); then
    echo "coverage-readme: '$file' has the '$end' marker before the '$start' marker" >&2
    return 2
  fi
}

splice() {
  local start="$1" end="$2" blockfile="$3" infile="$4" outfile="$5"
  awk -v start="$start" -v end="$end" -v blockfile="$blockfile" '
    $0 == start {
      print
      while ((getline line < blockfile) > 0) print line
      close(blockfile)
      inblock = 1
      next
    }
    $0 == end { inblock = 0; print; next }
    inblock { next }
    { print }
  ' "$infile" > "$outfile"
}

# Marker matching is exact in both the guard and the splicer: `$0 == marker`.
# A marker with trailing whitespace or a CRLF terminator is therefore rejected
# here instead of passing the guard and then failing to splice.
require_region "$readme" coverage-badge
require_region "$readme" coverage

splice '<!-- coverage-badge:start -->' '<!-- coverage-badge:end -->' "$badge_file" "$readme" "$step1"
splice '<!-- coverage:start -->' '<!-- coverage:end -->' "$block_file" "$step1" "$step2"

# The splicer must not have swallowed a region or the file tail.
require_region "$step1" coverage-badge
require_region "$step2" coverage
if [[ ! -s "$step2" ]]; then
  echo "coverage-readme: refusing to write an empty '$readme'" >&2
  exit 2
fi

if [[ "$check" -eq 1 ]]; then
  if ! diff -u "$readme" "$step2" >/dev/null 2>&1; then
    echo "coverage-readme: '$readme' is out of date with '$baseline'" >&2
    diff -u "$readme" "$step2" >&2 || true
    echo >&2
    echo "Run 'make coverage-readme' and commit the result." >&2
    exit 1
  fi
  echo "coverage-readme: '$readme' is in sync with '$baseline'"
  exit 0
fi

# `cat` rather than `mv` so the existing file mode and ownership are kept.
cat "$step2" > "$readme"
echo "coverage-readme: updated '$readme' from '$baseline'"
