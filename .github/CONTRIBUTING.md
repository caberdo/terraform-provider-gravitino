# Contributing to terraform-provider-gravitino

The Terraform provider for Apache Gravitino, published as
`registry.terraform.io/caberdo/gravitino`. Plugin protocol 6.0, built on
`terraform-plugin-framework` v1.19.0. Go module
`github.com/gravitino/terraform-provider-gravitino`, Apache-2.0, default branch `main`.
The provider currently exposes 21 resources and 50 data sources.

| Document | Contents |
|---|---|
| [ARCHITECTURE.md](../ARCHITECTURE.md) | Package layout, request flow, authentication, version gating, testing layers. |
| [GOVERNANCE.md](../GOVERNANCE.md) | Roles, decision making, review requirements and release authority. |
| [ROADMAP.md](../ROADMAP.md) | Supported Gravitino versions, planned surface and open work. |
| [AGENTS.md](../AGENTS.md) | Full convention reference for new resources and data sources. |
| [contributing/live-acceptance-tests.md](../contributing/live-acceptance-tests.md) | Human-readable specification of the live acceptance suite. |
| [.github/SECURITY.md](./SECURITY.md) | Supported versions, vulnerability reporting. |

## Development setup

Prerequisites:

- **Go 1.27.1 or newer.** The `go` directive in `go.mod` is `1.27.1`.
- **Docker with Compose**, for the container-based acceptance suite
  (`make testacc-docker`, `make testacc-matrix`).
- **Podman with `podman compose`**, for the live acceptance suite
  (`make testacc-live`, `make testacc-live-filter`).
- **Terraform CLI >= 1.9**, only to run `make validate-examples`. It is not needed to
  build or test the provider.

```bash
git clone https://github.com/caberdo/terraform-provider-gravitino.git
cd terraform-provider-gravitino
go mod download
go build ./...
```

`go build ./...` is a compile check that also warms the module cache; `make build` writes the
provider binary to `bin/terraform-provider-gravitino` with the version stamped in.

## Commands

Every command below is a target in [`GNUmakefile`](../GNUmakefile):

| Command | Runs |
|---|---|
| `make build` | `go build -o bin/terraform-provider-gravitino -ldflags "-X main.version=$(VERSION)" .` |
| `make install` | `make build`, then copies the binary into `~/.terraform.d/plugins/registry.terraform.io/caberdo/gravitino/$(VERSION)/$(GOOS)_$(GOARCH)/` |
| `make test` | `go test -v -cover ./internal/...` (unit tests; no server needed) |
| `make testacc` | `TF_ACC=1 go test -v -cover ./internal/...` (acceptance tests through Terraform Core) |
| `make testacc-docker` | `docker compose run --rm test` — Go 1.27 test container plus Gravitino 1.3.0 (`docker-compose.yml`) |
| `make testacc-matrix` | `make testacc-docker` once per version in [`.github/gravitino-versions.txt`](./gravitino-versions.txt), tearing each compose project down between versions |
| `make testacc-live` | `podman compose up -d gravitino` then `podman compose run --rm acc` in `docker-compose.yml` |
| `make testacc-live-filter F=<TestName>` | Same as above with `GO_TEST_FILTER` set; the target fails fast when `F` is empty |
| `make lint` | `golangci-lint run --config .github/golangci.yml ./...` |
| `make lint-fix` | Same with `--fix` |
| `make fmt` | `go fmt ./...` |
| `make vet` | `go vet ./...` |
| `make generate` | `go generate ./...` (regenerates `docs/`) |
| `make validate-examples` | `./scripts/validate-examples.sh` |
| `make coverage` | Unit profile (`coverage.out`) plus the per-package table |
| `make coverage-html` | `make coverage`, then `go tool cover -html=coverage.out` |
| `make coverage-merge` | Merges `coverage.out` and `coverage-acceptance.out` into `coverage-union.out` and prints the table |
| `make coverage-gate` | Fails when a scope of `coverage-union.out` is below its floor in [`.github/coverage-baseline.txt`](./coverage-baseline.txt) |
| `make coverage-readme` / `make coverage-readme-check` | Renders README.md's coverage badge and block from that baseline / fails when they have drifted from it |

`GRAVITINO_VERSION` defaults to `1.3.0` and selects the `apache/gravitino` image tag; pass
`GRAVITINO_VERSION=1.3.1` on the `make` command line to run against the other supported
server. CI pins golangci-lint v2.13.2 with the config committed at
[`.github/golangci.yml`](./golangci.yml) (errcheck, govet with `enable-all` minus
`fieldalignment` and `shadow`, ineffassign, staticcheck, unused, bodyclose, noctx, unconvert)
and runs gosec v2.29.0 as a separate job that fails on findings.

## Test layers

The suite is deliberately layered; each layer catches what the previous one cannot.

| Layer | Prefix | Count | Needs | Command |
|---|---|---|---|---|
| Unit and mock | `Test*` | 615 `Test*` functions across 116 `*_test.go` files | nothing | `make test` |
| Acceptance through Terraform Core | `TestAcc*` | 66 | nothing beyond Go (mock server) | `make testacc` |
| Container acceptance matrix | `TestAcc*` | same 66 | Docker | `make testacc-matrix` |
| Live acceptance against a real server | `TestLiveAcc*` | 26 | Podman | `make testacc-live` |

- **Unit tests** run the resource and client code directly. Provider HTTP interaction is
  served by `httptest.NewServer`; 75 files across the suite start one. There is no fuzzing.
- **`TestAcc*` tests** exercise the provider the way Terraform does: they call
  `resource.Test` with providers built by `providerserver.NewProtocol6WithError`, so the
  requests really travel through Terraform Core in-process and the mock Gravitino server is
  an `httptest` instance. This is the layer that catches unknown or inconsistent values that
  direct framework calls miss. They are skipped unless `TF_ACC=1` is set, which is what
  `make testacc` and the Docker targets do.
- **`TestLiveAcc*` tests** run against a real Gravitino server started by Podman. They are
  gated by `acceptance.LivePreCheck` (`internal/acceptance`), which skips the test when
  `GRAVITINO_URI` is unset and fails when `GET /api/version` does not return the expected
  Gravitino version. The individual cases are documented in
  [contributing/live-acceptance-tests.md](../contributing/live-acceptance-tests.md).
- **The version matrix** runs the acceptance suite once per entry in
  [`.github/gravitino-versions.txt`](./gravitino-versions.txt) — currently Gravitino `1.3.0`
  and `1.3.1`. Adding a supported server version is a one-line change there. Tests for
  features that only exist in a newer server (for example
  `TestLiveAccCatalogConnectionTestExisting`, which needs 1.3.1) skip themselves on older
  versions instead of failing.

## Coverage

`make coverage` writes the unit profile (`coverage.out`, `TestAcc*`/`TestLiveAcc*` skipped) and
prints a per-package table, weakest first; `make coverage-html` opens it as HTML.
`make coverage-merge` merges it with the profile the acceptance container writes
(`coverage-acceptance.out`, `-coverpkg=./internal/...`) into `coverage-union.out` and prints the
same table. `merge-coverage.sh` keys blocks by position, so the union has one line per block
however the inputs were instrumented, and `coverage-summary.sh` refuses a profile that repeats a
block (exit 2) rather than printing a wrong number. CI publishes both tables in its job summaries:
the `test` job for the unit profile, the `Coverage` job on a pull request and the `acceptance` job
on `main` for the merged one.

Merged (unit + acceptance) statement coverage is enforced. `make coverage-gate`
(`scripts/coverage-gate.sh`) compares `coverage-union.out` against
[`.github/coverage-baseline.txt`](./coverage-baseline.txt) — the file `scripts/coverage-readme.sh`
renders README.md's badge and block from — and fails, naming each scope below its `floor_*`. The
`Coverage` job runs both scripts on every pull request, after running the acceptance container, so
the floor is never compared against a unit-only number and the published block cannot drift. The
gate fails closed: an absent profile, a scope named in the baseline that the profile does not
report, and a missing, misspelled or stray `floor_*` are configuration errors (exit 2), never a
pass.

```bash
make coverage            # unit profile + per-package table
make testacc-docker      # writes coverage-acceptance.out
make coverage-merge      # merge both into coverage-union.out
make coverage-gate       # fail when below the floor
```

To raise a floor: update the `measured_*` and `floor_*` values in
[`.github/coverage-baseline.txt`](./coverage-baseline.txt), run `make coverage-readme` to refresh
the README block, and run `make coverage-gate` in the same pull request that adds the tests.

Measured at revision `9c94a46` on the committed tooling (`go 1.27.2`, Gravitino 1.3.0):

| Command | Statement coverage |
|---|---|
| `make coverage` (unit only, per package — `make test`'s shape) | 59.8% (4584/7661) |
| `make coverage-merge` (unit + acceptance; the profile the gate enforces) | 79.2% (6069/7661) |

`make test` produces the per-package number because it passes `-cover`, not `-coverpkg`. CI's test
job runs `go test -v -race -covermode=atomic -coverprofile=coverage.out ./internal/...`, publishes
`coverage.out` as a build artifact and in the job summary; the `Coverage` job (pull requests) and
the acceptance job (main) merge it with the container's profile. Statement counts depend on the Go
patch release that produced the profile (`go 1.27.1`, the `go.mod` toolchain, sums this tree to
9766 statements against 7661 for `go 1.27.2`), so compare numbers only within one toolchain, and
note that a merged profile takes its statement counts from the profile `merge-coverage.sh` reads
last — the acceptance container's, in CI.

## Code conventions

These are enforced in review; see [AGENTS.md](../AGENTS.md) for the complete list.

| Convention | Rule |
|---|---|
| API errors | Every API error is returned through `client.NewResourceError` (153 call sites). |
| Not found | A 404 is detected with `client.IsNotFoundError` and handled by `resp.State.RemoveResource(ctx)` (69 call sites). Delete treats a missing object as success. |
| Logging | `tflog.Debug` at the start and end of every Create/Read/Update/Delete (192 call sites). The provider `uri` and credentials are never logged. |
| IDs | Hierarchy IDs are dot-separated (`metalake.catalog.schema`). Import parses and validates them. |
| Immutable attributes | An attribute the API cannot update in place carries `RequiresReplace()` (71 call sites); a change is never accepted and silently dropped. |
| Computed attributes | Set on every code path, or carry `UseStateForUnknown()` (41 call sites), so refresh cannot produce an inconsistent result. |
| Enum attributes | `stringvalidator.OneOf` on every enum field, with the values taken from the Gravitino API docs (30 call sites). |

## Testing policy

- Every resource and data source must have tests for its major functionality: schema, create,
  read, update, delete and import (both valid and invalid IDs).
- Every bug fix must come with a regression test that fails before the fix.
- Mock payloads MUST be copied from the `examples:` block of the matching OpenAPI
  specification rather than written from the Go structs. A payload invented from the structs
  only proves that the struct marshals, not that the API agrees with it — note that the
  provider's string literals come from those `examples:` blocks.
- CRUD behaviour that goes through Terraform Core belongs in a `TF_ACC=1` test rather than a
  direct framework call.

## Generated documentation

`docs/` is generated by `terraform-plugin-docs` from the `templates/` and `examples/`
directories via `make generate` (`go generate ./...`). **`docs/` is never hand-edited**: an
edit is reverted by the next run. CI regenerates it and fails on any diff under `docs/`, so a
schema change that is not accompanied by `make generate` cannot merge. Examples added under
`examples/` must pass `make validate-examples`.

## Commit messages

[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with a scope naming
the affected resource, data source or subsystem:

```
feat(catalog): add support for property updates
fix(schema): handle 404 on read
docs(metalake): add import example
chore(ci): bump golangci-lint
```

## Pull request process

1. Fork the repository (or create a branch if you have write access) and branch off `main`.
2. Implement the change and add the tests the policy above requires.
3. Run `make build`, `make vet`, `make test` and `make lint` before pushing. `make fmt` fixes
   formatting.
4. Run `make generate` if a schema changed, and `make validate-examples` if an example
   changed; commit the regenerated files.
5. Run `make coverage-merge && make coverage-gate`, and raise the floor in
   [`.github/coverage-baseline.txt`](./coverage-baseline.txt) — followed by `make coverage-readme`
   — in the same pull request if the change improved coverage.
6. Push and open a pull request against `main`, filling in
   [PULL_REQUEST_TEMPLATE.md](./PULL_REQUEST_TEMPLATE.md).
7. CI runs on every push and pull request: lint (golangci-lint and gosec), build and vet,
   `go mod tidy` and `go mod verify`, cross-builds for windows/amd64 and darwin/arm64, the
   example validator, the docs-drift check, and the unit tests with `-race`. A pull request
   also runs the acceptance container, the merged-coverage gate and the README/baseline drift
   check (the `Coverage` check); the acceptance matrix runs only on pushes to `main`.
8. The maintainer reviews the change; address the comments and push follow-ups to the
   same branch. `.github/CODEOWNERS` currently reads `* @caberdo`, naming the
   organization rather than a user or team, so GitHub's code-owner rule has no effective
   owner today — see [GOVERNANCE.md](../GOVERNANCE.md#continuous-integration-as-the-gate).
9. Once CI is green and the threads are resolved, the pull request is merged (merge, squash
   or rebase — all three are allowed). Release tags are created separately by maintainers;
   see [GOVERNANCE.md](../GOVERNANCE.md).

## Code review

Reviewers check the change against the conventions table above, the testing policy, and the
resource checklist in [AGENTS.md](../AGENTS.md): error wrapping, 404 handling, logging
boundaries, id parsing, replacement vs in-place update, validators and generated docs.

What blocks a merge:

- A red CI run on the pull request. The `main` ruleset does **not** currently require status
  checks, so this is a convention enforced by reviewers rather than by GitHub.
- Unresolved review threads.

What the `main` ruleset does enforce: a pull request (no direct pushes), a code-owner review
rule from [CODEOWNERS](./CODEOWNERS), and no force-pushes or branch deletion. That rule has
no effective owner while CODEOWNERS names the `caberdo` organization, and the ruleset requires
**zero approving reviews**, does not dismiss approvals on push, and does not require review
thread resolution — reviewers must therefore block on red CI and open threads by hand.

## Licensing

All contributions are accepted under the Apache License 2.0 and must be compatible with it;
third-party code or assets you add must carry a licence that allows redistribution under
Apache-2.0. See [LICENSE](../LICENSE) and [NOTICE](../NOTICE) for the project licence and the
upstream works it derives from. By opening a pull request you confirm you have the right to
submit the change under those terms. Contributions are subject to
[CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md) (Contributor Covenant 2.0).

## Questions

Ask in a [GitHub Issue](https://github.com/caberdo/terraform-provider-gravitino/issues); bug
reports and feature requests have templates under
[`.github/ISSUE_TEMPLATE/`](./ISSUE_TEMPLATE). GitHub Discussions are disabled on this
repository, so questions are handled in issues. Report suspected vulnerabilities privately
as described in [.github/SECURITY.md](./SECURITY.md), never in a public issue.
