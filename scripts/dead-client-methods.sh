#!/usr/bin/env bash
# Fail when an exported *client.Client method has no call site.
#
# `deadcode -test ./...` cannot police this surface: with the test binaries as
# roots it treats every exported method of the client as reachable, and without
# them it still does not report them. The methods that matter are only reachable
# through the provider, so this check looks for an actual `.Method(` call site
# in the module (production and test code alike).
#
# The lint job in .github/workflows/test.yml runs this before the test job.
set -euo pipefail

cd "$(dirname "$0")/.."

status=0
while read -r method; do
	if grep -rqE "\.${method}\(" --include='*.go' --exclude-dir=.git .; then
		continue
	fi
	echo "unreferenced client method: Client.${method}" >&2
	status=1
done < <(
	grep -rhoE '^func \(c \*Client\) [A-Z][A-Za-z0-9_]*' internal/client --include='*.go' \
		| sed -E 's/^func \(c \*Client\) //' \
		| sort -u
)

if [ "$status" -ne 0 ]; then
	echo "Delete the method or call it from the provider surface (see issue #72)." >&2
fi
exit "$status"
