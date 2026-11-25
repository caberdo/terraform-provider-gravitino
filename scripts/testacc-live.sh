#!/usr/bin/env bash
set -euo pipefail

URI="${GRAVITINO_URI:-http://gravitino:8090}"
FILTER="${GO_TEST_FILTER:-TestLiveAcc}"

# /api/health only exists from Gravitino 1.3.0; /api/version is served by every
# supported release, so it is the readiness signal the whole matrix shares.
echo "Waiting for Gravitino at ${URI}/api/version ..."
ready=""
for i in $(seq 1 60); do
  if curl -fsS -m 3 "${URI}/api/version" >/dev/null 2>&1; then
    ready=1
    break
  fi
  echo "  not ready (attempt ${i}/60)"
  sleep 2
done
if [ -z "${ready}" ]; then
  echo "Gravitino did not become ready at ${URI}" >&2
  exit 1
fi
echo "Gravitino is ready."

go mod download && go mod verify

# Discover the packages that actually contain live tests, so a new TestLiveAcc* test is
# picked up without touching this list.
packages=$(grep -rl --include='*_test.go' -E '^func (TestLiveAcc|TestLiveAcc_)' internal/ \
  | xargs -n1 dirname | sort -u | sed 's|^|./|' | tr '\n' ' ')

if [ -z "${packages}" ]; then
  echo "no live test packages found" >&2
  exit 1
fi

echo "live test packages: ${packages}"
# shellcheck disable=SC2086
go test -v -count=1 -timeout 30m -run "${FILTER}" ${packages}
