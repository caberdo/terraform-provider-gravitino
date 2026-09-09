#!/usr/bin/env bash
set -euo pipefail

URI="${GRAVITINO_URI:-http://gravitino:8090}"
FILTER="${GO_TEST_FILTER:-TestLiveAcc}"

echo "Waiting for Gravitino at ${URI}/api/health ..."
ready=""
for i in $(seq 1 60); do
  if curl -fsS -m 3 "${URI}/api/health" >/dev/null 2>&1; then
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

curl -fsS -m 5 "${URI}/api/version" >/dev/null || {
  echo "version endpoint unreachable at ${URI}" >&2
  exit 1
}

go mod download && go mod verify

go test -v -timeout 30m -run "${FILTER}" \
  ./internal/resources/metalake/ \
  ./internal/resources/catalog/ \
  ./internal/resources/tag/ \
  ./internal/datasources/health/ \
  ./internal/datasources/metalake/ \
  ./internal/datasources/authentication/
