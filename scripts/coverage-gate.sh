#!/usr/bin/env bash
# Fail when the merged (unit + acceptance) statement coverage drops below the
# floor recorded in .github/coverage-baseline.txt, so that a pull request cannot
# quietly make the project less tested than it was.
#
# The baseline is the same file `scripts/coverage-readme.sh` renders README.md
# from, parsed with the same rules: `measured_<scope>` names a scope and needs a
# matching `floor_<scope>`, `_` is the path separator in the rendered label, and
# an unknown, duplicated, empty or non-numeric key is a configuration error.
#
# The gate fails closed. It exits 2 — never 0 — for an absent profile or
# baseline, for a baseline it cannot parse, and for a scope the baseline declares
# that the profile does not report (a scope must not be able to drop out of the
# table while coverage falls). It exits 1, naming every scope, when a scope is
# below its floor.
#
# Construction it enforces: the profile `make coverage-merge` writes
# (`coverage-union.out`) — the unit profile from `make coverage` (`coverage.out`)
# merged by `scripts/merge-coverage.sh` with the profile the acceptance container
# writes (`coverage-acceptance.out`, `-covermode=atomic -coverpkg=./internal/...`,
# `TF_ACC=1` against a real Gravitino). The merge keys blocks by position and
# `coverage-summary.sh` refuses a profile that repeats a block, so the statement
# column is summed exactly once per block.
#
# Usage: scripts/coverage-gate.sh [profile] [baseline]
#        scripts/coverage-gate.sh coverage-union.out
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
profile="${1:-coverage-union.out}"
baseline="${2:-.github/coverage-baseline.txt}"

die() {
  echo "coverage-gate: $*" >&2
  exit 2
}

trim() {
  local s="$1"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

[[ -f "$profile" ]] || die "profile '$profile' not found; run 'make coverage' and 'make coverage-merge' first"
[[ -f "$baseline" ]] || die "baseline '$baseline' not found"

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
    die "'$baseline' defines '$key' more than once"
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
    die "$baseline:$lineno is not 'key=value'"
  fi

  key="$(trim "${line%%=*}")"
  value="$(trim "${line#*=}")"
  if [[ -z "$key" || -z "$value" ]]; then
    die "$baseline:$lineno has an empty key or value"
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
        die "$baseline:$lineno has an invalid scope '$scope'"
      fi
      note_key "$key"
      scopes+=("$scope")
      printf -v "value_measured_$scope" '%s' "$value"
      ;;
    floor_*)
      scope="${key#floor_}"
      if [[ ! "$scope" =~ ^[A-Za-z0-9_]+$ ]]; then
        die "$baseline:$lineno has an invalid scope '$scope'"
      fi
      note_key "$key"
      floor_scopes+=("$scope")
      printf -v "value_floor_$scope" '%s' "$value"
      ;;
    *)
      die "$baseline:$lineno has an unknown key '$key'"
      ;;
  esac
done < "$baseline"

if (( ${#scopes[@]} == 0 )); then
  die "'$baseline' defines no measured_<scope> keys"
fi
if [[ -z "$measured_commit" || -z "$measured_date" ]]; then
  die "'$baseline' needs measured_commit and measured_date"
fi
if [[ -z "$target" || -z "$gold" ]]; then
  die "'$baseline' needs target and gold"
fi

# Same shape as scripts/coverage-readme.sh: a scope key names a scope, and every
# path may not contain a slash, so `_` is the separator in the rendered label.
value_of() {
  local prefix="$1" scope="$2" name value
  name="value_${prefix}_${scope}"
  value="${!name-}"
  printf '%s' "$value"
}

require_pct() {
  local label="$1" value="$2"
  if [[ ! "$value" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
    die "'$baseline' has a non-numeric $label '$value'"
  fi
  if ! awk -v v="$value" 'BEGIN { exit !(v >= 0 && v <= 100) }'; then
    die "'$baseline' has $label '$value' outside 0..100"
  fi
}

has_total=0
for scope in ${scopes[@]+"${scopes[@]}"}; do
  [[ "$scope" == total ]] && has_total=1
done
if [[ "$has_total" -eq 0 ]]; then
  die "'$baseline' needs a measured_total scope"
fi

# Every measured scope must have a floor and every floor must name a measured
# scope: a missing, misspelled or stray floor is a configuration error (exit 2),
# never a scope that silently stops being enforced.
for scope in ${scopes[@]+"${scopes[@]}"}; do
  if [[ -z "$(value_of floor "$scope")" ]]; then
    die "'$baseline' has measured_$scope but no floor_$scope"
  fi
done
for scope in ${floor_scopes[@]+"${floor_scopes[@]}"}; do
  found=0
  for measured_scope in ${scopes[@]+"${scopes[@]}"}; do
    [[ "$scope" == "$measured_scope" ]] && found=1
  done
  if [[ "$found" -eq 0 ]]; then
    die "'$baseline' has floor_$scope but no measured_$scope"
  fi
done

require_pct target "$target"
require_pct gold "$gold"
if ! awk -v t="$target" -v g="$gold" 'BEGIN { exit !(t < g) }'; then
  die "'$baseline' needs target ($target) below gold ($gold)"
fi
for scope in ${scopes[@]+"${scopes[@]}"}; do
  require_pct "measured_$scope" "$(value_of measured "$scope")"
  require_pct "floor_$scope" "$(value_of floor "$scope")"
done

# The summary rejects a profile with duplicate block lines (exit 2), which is the
# only shape in which its statement column is exact.
set +e
rows="$("$script_dir/coverage-summary.sh" --tsv "$profile")"
status=$?
set -e
[[ "$status" -eq 0 ]] || exit "$status"

# Profile rows report the package path (or `total`), so a scope key is matched by
# rendering the path the same way the baseline key is written.
row_pct() {
  local want="$1" line scope pct
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    scope="${line%%$'\t'*}"
    [[ "${scope//\//_}" == "$want" ]] || continue
    pct="${line#*$'\t'}"
    printf '%s' "${pct%%$'\t'*}"
    return 0
  done <<< "$rows"
  return 1
}

display_name() {
  case "$1" in
    total) printf 'total' ;;
    *) printf '%s' "${1//_//}" ;;
  esac
}

# Baseline order, with total last so the table reads as a progression.
ordered=()
for scope in ${scopes[@]+"${scopes[@]}"}; do
  [[ "$scope" == total ]] && continue
  ordered+=("$scope")
done
ordered+=(total)

at_floor=0
below=0
detail=""

report() {
  local scope="$1" label floor actual delta
  label="$(display_name "$scope")"
  floor="$(value_of floor "$scope")"
  actual="$(row_pct "$scope")" ||
    die "profile '$profile' reports no scope '$label'; a scope named in $baseline cannot disappear while coverage falls"
  if awk -v a="$actual" -v b="$floor" 'BEGIN { exit !(a + 0 >= b + 0) }'; then
    at_floor=$((at_floor + 1))
  else
    below=$((below + 1))
    detail="${detail}  - ${label}: ${actual}% is below the ${floor}% floor"$'\n'
  fi
  delta="$(awk -v a="$actual" -v b="$floor" 'BEGIN { printf "%+.1f", a - b }')"
  # shellcheck disable=SC2016  # the backticks are literal Markdown
  printf '| `%s` | %.1f%% | %s%% | %s |\n' "$label" "$actual" "$floor" "$delta"
}

echo "Merged statement coverage (unit + acceptance)"
echo "Profile:  $profile"
echo "Baseline: $baseline"
echo
echo "| Scope | Coverage | Floor | Delta |"
echo "|-------|----------|-------|-------|"
for scope in "${ordered[@]}"; do
  report "$scope"
done

echo
if [[ "$below" -gt 0 ]]; then
  echo "coverage-gate: FAIL — merged coverage in $profile is below the floor in $baseline"
  printf '%b' "$detail"
  echo
  echo "Add tests for what you changed, or — only if the drop is deliberate and justified —"
  echo "lower the floor in $baseline in the same pull request and run 'make coverage-readme'."
  exit 1
fi

total_actual="$(row_pct total)"
echo "coverage-gate: PASS — $at_floor scope(s) at or above their floor."
if awk -v a="$total_actual" -v t="$target" 'BEGIN { exit !(a + 0 >= t + 0) }'; then
  echo "The ${target}% milestone is met."
else
  echo "Milestones tracked for this project: ${target}% first, then ${gold}%."
fi
