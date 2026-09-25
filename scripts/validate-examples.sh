#!/usr/bin/env bash
# Validates every Terraform example under examples/ against the provider built from
# this working tree. See scripts/validate-examples.py for what is checked and why.
#
# Usage: scripts/validate-examples.sh
#   GRAVITINO_PLUGIN_DIR=<dir>  override the plugin directory (for parallel runs)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform is not installed; skipping example validation" >&2
  exit 0
fi

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
OS="$(go env GOOS)"
ARCH="$(go env GOARCH)"
PLUGIN_DIR="${GRAVITINO_PLUGIN_DIR:-${TMPDIR:-/tmp}/gravitino-provider-dev}"
BIN_DIR="${PLUGIN_DIR}/registry.terraform.io/gravitino/gravitino/${VERSION}/${OS}_${ARCH}"
CLI_CONFIG="${PLUGIN_DIR}/dev.tfrc"

mkdir -p "$BIN_DIR"
echo "building provider ${VERSION}..."
go build -o "${BIN_DIR}/terraform-provider-gravitino" \
  -ldflags "-X main.version=${VERSION}" .

# The resource snippets in examples/ carry no terraform block, so Terraform infers
# hashicorp/gravitino as the source; the provider block in examples/complete uses
# gravitino/gravitino. Override both, so no example needs network access.
printf 'provider_installation {\n  dev_overrides {\n    "gravitino/gravitino" = "%s"\n    "hashicorp/gravitino" = "%s"\n  }\n  direct {}\n}\n' \
  "$BIN_DIR" "$BIN_DIR" >"$CLI_CONFIG"

export TF_CLI_CONFIG_FILE="$CLI_CONFIG"
export TF_IN_AUTOMATION=1

exec python3 "${REPO_ROOT}/scripts/validate-examples.py"
