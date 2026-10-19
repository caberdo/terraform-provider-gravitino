# Roadmap

What this project intends to do — and deliberately not do — over the next twelve months. The
living version of the near-term work is the
[issue list](https://github.com/caberdo/terraform-provider-gravitino/issues); this file records the
direction those issues add up to. It is reviewed once per release; `CHANGELOG.md` records what
actually shipped.

## What the project is

A Terraform provider for [Apache Gravitino](https://gravitino.apache.org): it manages metalakes,
catalogs, schemas, tables, filesets, topics, views, functions, models, partitions, tags, policies,
jobs, users/groups/roles and owners as Terraform state, through the Gravitino REST API. Today:
**21 resources and 50 data sources** (`ls docs/resources | wc -l`, `ls docs/data-sources | wc -l`),
one build supporting Gravitino **1.3.0 and 1.3.1** (`.github/gravitino-versions.txt`), 24 tags with
`v0.9.0` as the latest release (2026-10-08).

## Now — supply chain, CI and the documentation spine

The current cycle makes the project's own guarantees as checkable as the ones it configures in
Gravitino. Open work, in the order it unblocks itself:

| Work | Issue |
|---|---|
| Publish the governance, roadmap and architecture documents plus an OpenSSF Best Practices evidence pack | [#50](https://github.com/caberdo/terraform-provider-gravitino/issues/50) |

Landed in this cycle already: every GitHub Action pinned to a commit SHA, a `dependency-review` job
on pull requests, the docs-drift gate, example validation against the provider built from the tree,
a pinned `golangci-lint` and `goreleaser`, a cross-platform build, `go mod tidy`/`verify` as a
check, the live acceptance suite against a real server, the README and Registry landing page
rebuilt as the front door
([#47](https://github.com/caberdo/terraform-provider-gravitino/issues/47)), and the release chain
hardened — GPG-signed tags, GPG-signed checksums, the public signing key committed in the
repository, and SLSA v1.0 build provenance attached to every release
([#48](https://github.com/caberdo/terraform-provider-gravitino/issues/48)). OpenSSF Scorecard now
runs weekly on `main`, on branch-protection changes and publishes its result for the README badge,
and CI reports the merged unit + acceptance statement coverage in its job summaries
([#49](https://github.com/caberdo/terraform-provider-gravitino/issues/49)).

Still open, with no issue filed yet: six modules in the dependency graph carry OSV advisories
(measured with `go list -m all` against `api.osv.dev/v1/querybatch`). The ones that matter are in
the released binary's graph, not the docs tooling — `golang.org/x/net` v0.59.0 (five HTTP/2
advisories published 2026-10-08, fixed in v0.60.0) and the stdlib `net/http` HTTP/2 fixes that ship
in the Go **1.27.2** toolchain while `go.mod` still says `1.27.1`. Bumping both, plus `goldmark`
v1.7.17 for the docs tooling, is the next supply-chain step; it is also open item 10 of
[`.github/OPENSSF-BEST-PRACTICES.md`](./.github/OPENSSF-BEST-PRACTICES.md#5-open-items--what-is-not-met-yet-and-who-can-fix-it).

Two configuration gaps are recorded in [`GOVERNANCE.md`](./GOVERNANCE.md#continuous-integration-as-the-gate)
rather than tracked as issues, because they are repository settings and not code: the `main` ruleset
does not require the CI checks, and its code-owner requirement has no effective owner. The
organization also has 2FA enforcement and secret scanning turned off.

## Next — correctness, coverage and honest compatibility

* **Raise the merged statement coverage above 85%.** `make coverage-merge` reports the number CI
  publishes: **82.1%** (8021/9766) for the merged unit + acceptance profile, 65.8% for the unit
  profile alone — above the 80% the OpenSSF Best Practices programme requires. The gap is
  concentrated where the tests are acceptance-shaped rather than unit-shaped, so the work is unit
  tests for the error and drift paths of `internal/datasources/schema`,
  `internal/datasources/secrets`, `internal/provider`, `internal/client` and `internal/models`;
  `make coverage` ranks the packages, weakest first. Per-package `go test -cover` numbers understate
  this badly (the client reads 14.3% there because its callers live in other packages); always
  quote the merged profile.
* **Keep the version matrix honest.** `.github/gravitino-versions.txt` is the single source of truth
  for the acceptance matrix; a new Gravitino release is added there once its REST API is checked
  against the schema, and every version-restricted field keeps its union-schema-plus-runtime-gate
  treatment.
* **Close the live-coverage gaps** that a stock server cannot reach today: table, view, function,
  partition and statistics need a lakehouse catalog (hive/iceberg), topic needs a Kafka catalog, and
  `idp_user`/`idp_group` need the `idp-basic` extension. The live suite documents the exact
  configuration each one needs (`contributing/live-acceptance-tests.md`), so the work is a CI
  compose profile, not guesswork.
* **Extend `gravitino_secrets` when Gravitino 1.4 ships.** The endpoint exists only on `main` today
  and answers `404` on every supported release, which the data source surfaces explicitly.

## Later — structure and reach

* **Discoverability:** keep the provider listed in the Terraform Registry and the OpenTofu
  registry, and link it from the Gravitino documentation's client list.
* **Release cadence:** patch releases for Gravitino compatibility and bug fixes, minor releases for
  new resources and data sources, deprecations announced one minor release ahead of removal.

## Explicit non-goals

* **Not a Gravitino server, installer or deployment tool.** The provider configures a *running*
  Gravitino instance; it does not provision the server, its catalogs' backing storage (Hive
  metastores, object stores, Kafka) or its containers. Use the Gravitino distribution, Ansible,
  Kubernetes or your cloud tooling for that.
* **Not a generic Gravitino REST client.** Only the endpoints the Terraform surface needs are
  wrapped; arbitrary API calls are out of scope, and a new endpoint arrives with a resource or data
  source that uses it.
* **Not a data-plane tool.** The provider manages metadata; it never reads or writes table data.
* **No breaking schema changes.** Renaming or removing a Terraform type or attribute breaks user
  state, so it only happens with a documented upgrade path and a major version.
* **No hosted service, paid tier or telemetry.** This repository is the whole product; the provider
  sends nothing anywhere except the Gravitino server you configure.
* **No roadmap dates.** Work is ordered by value, not by calendar; the issues and this file are the
  commitment.

## How to influence this

Open an issue using the templates in `.github/ISSUE_TEMPLATE/`, or comment on an existing one.
Decisions and their reasoning are recorded in the issue before the code lands — see
[`GOVERNANCE.md`](./GOVERNANCE.md).
