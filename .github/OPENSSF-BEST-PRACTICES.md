# OpenSSF Best Practices badge — evidence pack

Everything needed to register `caberdo/terraform-provider-gravitino` at
[bestpractices.dev](https://www.bestpractices.dev), answer the questionnaire, publish the badge and
give OpenSSF Scorecard's `CII-Best-Practices` check something to read.

Prepared 2026-10-19 against repository revision `c2fc783` plus this change, with the criterion
texts from
[`criteria/criteria.yml`](https://github.com/coreinfrastructure/best-practices-badge/blob/main/criteria/criteria.yml)
and
[`config/locales/en.yml`](https://github.com/coreinfrastructure/best-practices-badge/blob/main/config/locales/en.yml)
of the BadgeApp. Answers are grouped by badge level; only **MUST** criteria decide whether a level is
awarded. Every number in this file was measured in this repository — the commands are in §7.

## 1. How Scorecard scores this check — read this first

| Badge level | Scorecard score |
|---|---|
| In Progress | 2 |
| **Passing** | **5** |
| **Silver** | **7** |
| **Gold** | **10** |

Source: [`checks/evaluation/cii_best_practices.go`](https://github.com/ossf/scorecard/blob/main/checks/evaluation/cii_best_practices.go)
(`passingScore = 5`, `silverScore = 7`, gold = max) and
[`probes/hasOpenSSFBadge/impl.go`](https://github.com/ossf/scorecard/blob/main/probes/hasOpenSSFBadge/impl.go).
Scorecard reads the level from bestpractices.dev, not from the README:

```
GET https://www.bestpractices.dev/projects.json?url=https://github.com/caberdo/terraform-provider-gravitino
→ []                                    # empty today: the project is not registered
```

Source: [`clients/cii_http_client.go`](https://github.com/ossf/scorecard/blob/main/clients/cii_http_client.go).
The README badge is for humans; Scorecard never parses it.

**Where the climb stands.** Passing (5) is reachable now. Silver (7) is short of one item,
recorded in §5: the badge itself cannot be linked from the README before the project is
registered. Merged statement coverage clears the 80% requirement: the `Coverage` job fails a pull
request below the per-scope floor in `.github/coverage-baseline.txt`, whose total floor is 82.5,
and `merge-coverage.sh` keeps one line per block however the inputs were instrumented, so the
merged total does not depend on which profile is read last. The measured total is `measured_total`
in that file rather than a number quoted here, because it moves with every test.
`signed_releases` and `version_tags_signed` are **Met** since the tagged release became GPG-signed
with the public key committed (§4.3). Gold (10) is **not reachable while the project has one
maintainer**: `bus_factor`, `contributors_unassociated` and `two_person_review` are MUST criteria
there, and no amount of code satisfies them.

## 2. Register the project (maintainer, ~10 minutes, cannot be delegated)

1. Sign in at <https://www.bestpractices.dev> with GitHub.
2. **Get started** → *Add a new project*:
   * **Project name:** `Terraform Provider for Apache Gravitino`
   * **Project homepage URL / source repository:** `https://github.com/caberdo/terraform-provider-gravitino`
   * **Project description:** what `README.md` says — a Terraform provider that manages Apache
     Gravitino metadata (metalakes, catalogs, schemas, tables, filesets, topics, views, functions,
     models, partitions, tags, policies, jobs, users/groups/roles, owners) as code: 25 resources and
     55 data sources against the Gravitino REST API.
   * **License:** `Apache-2.0` (the SPDX identifier; `LICENSE` and GitHub agree).
3. Answer from §4. Criteria answered `Met` need a justification and, where the table says so, a URL.
4. Submit at **Passing** first, then re-answer §4.3/§4.4 as §5 items close.
5. Paste the badge (next section) into the README and merge it.

## 3. The README badge (after step 2)

bestpractices.dev assigns a numeric project id. Add this line to `README.md` (next to the other
badges), replacing `PROJECT_ID` with the id from `https://www.bestpractices.dev/projects/PROJECT_ID`:

```markdown
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/PROJECT_ID/badge)](https://www.bestpractices.dev/projects/PROJECT_ID)
```

The id does not exist before step 2, so this line is the one deliberate fill-in in this pack. It
satisfies the Silver criterion `documentation_achievements`, which requires the achievement to be
linked from the project's front page. `.github/github-action-markdown-link-check-config.json`
already ignores such external hosts, so a missing project cannot break `Validate Markdown Files`.

## 4. Answers to paste into the questionnaire

`Evidence` paths are relative to the repository root. `blob/…` in the Evidence column means
`https://github.com/caberdo/terraform-provider-gravitino/blob/main/…`.

### 4.1 Passing (level 0) — MUST criteria

| Criterion | Answer | Justification | Evidence |
|---|---|---|---|
| `description_good` | Met | README and the repository description say what the provider does: manage Apache Gravitino metadata as Terraform resources and data sources, 21 + 50 of them. | `blob/README.md` |
| `interact` | Met | README shows how to obtain the provider, configure it (block or `GRAVITINO_*` environment variables) and apply a configuration; `SUPPORT.md` and the issue forms say how to get help and report bugs; `CONTRIBUTING.md` says how to contribute. | `blob/README.md`, `blob/SUPPORT.md`, `blob/.github/CONTRIBUTING.md` |
| `contribution` | Met | `CONTRIBUTING.md` documents prerequisites, the `make` targets, the four test layers, the coding standards, the testing policy, the documentation rule and the pull request process. | `blob/.github/CONTRIBUTING.md` |
| `contribution_requirements` | Met | Same file states the requirements a change must meet: conventions, tests, regenerated docs, one logical change per pull request. | `blob/.github/CONTRIBUTING.md` |
| `floss_license` | Met | Apache-2.0, an OSI-approved licence; `LICENSE` holds the licence text and the GitHub API reports `Apache-2.0`. | `blob/LICENSE` |
| `floss_license_osi` | Met | Apache-2.0 is OSI-approved. | `blob/LICENSE` |
| `license_location` | Met | The licence is in the standard root location, linked from the README, and `NOTICE` records what the project derives from. | `blob/LICENSE`, `blob/NOTICE` |
| `documentation_basics` | Met | README (requirements, provider configuration, resource/data-source inventory, testing), the generated reference in `docs/`, `examples/` (provider, one per resource and data source, and `complete/`), plus `ARCHITECTURE.md`, `ROADMAP.md`, `GOVERNANCE.md`. | `blob/README.md`, `blob/docs/index.md`, `blob/examples/complete/main.tf` |
| `documentation_interface` | Met | Every resource and data source has a generated page listing every attribute with its type, whether it is required/optional/computed, and its import syntax. | `blob/docs/resources`, `blob/docs/data-sources` |
| `sites_https` | Met | Every project site is HTTPS: GitHub, the Terraform Registry, `gravitino.apache.org` and the links from the README. | `https://github.com/caberdo/terraform-provider-gravitino` |
| `discussion` | Met | GitHub issues and pull-request threads are searchable, URL-addressable, open to any GitHub account and need no proprietary client. GitHub Discussions is disabled on this repository, so issues are the discussion mechanism. | `https://github.com/caberdo/terraform-provider-gravitino/issues` |
| `english` | Met | All documentation, comments and commit messages are in English. | `blob/README.md`, `blob/.github/CONTRIBUTING.md` |
| `maintained` | Met | Maintained: 24 tags with `v0.9.0` released 2026-10-08, commits on `main` today, Dependabot version updates daily, CI on every pull request and push. | `https://github.com/caberdo/terraform-provider-gravitino/releases` |
| `repo_public` | Met | Public Git repository. | `https://github.com/caberdo/terraform-provider-gravitino` |
| `repo_track` | Met | Git records every change with author and timestamp; releases are annotated tags. | `blob/CHANGELOG.md` |
| `repo_interim` | Met | Every change lands as a pull request before release, and `CHANGELOG.md` now carries an `Unreleased` section for work merged but not yet released. | `https://github.com/caberdo/terraform-provider-gravitino/pulls?q=is%3Apr+is%3Aclosed`, `blob/CHANGELOG.md` |
| `repo_distributed` | Met | Git, with the full history published. | `https://github.com/caberdo/terraform-provider-gravitino` |
| `version_unique` | Met | Every release has a unique version: tags `v0.1.0` … `v0.9.0`, the version embedded in the binary by `-X main.version`, and the registry manifest name. | `https://github.com/caberdo/terraform-provider-gravitino/tags` |
| `version_semver` | Met | Releases follow semantic versioning (`v0.9.0`), with breaking changes called out in `CHANGELOG.md`. | `blob/CHANGELOG.md` |
| `version_tags` | Met | Each release is tagged; the tag drives `release.yml`. | `https://github.com/caberdo/terraform-provider-gravitino/releases` |
| `release_notes` | Met | `CHANGELOG.md` groups each release into features, bug fixes, enhancements and internal/tooling (not a raw commit log), and the GitHub release page links the merged pull requests. | `blob/CHANGELOG.md` |
| `release_notes_vulns` | N/A | No release has ever fixed a publicly known run-time vulnerability with a CVE or equivalent, so there is nothing to list. | — |
| `report_process` | Met | Bug reports go through a GitHub issue form asking for the provider version, Terraform version, Gravitino version, reproduction steps and error output. | `blob/.github/ISSUE_TEMPLATE/bug_report.md` |
| `report_responses` | Met | The repository is four months old and reports filed so far were triaged and closed by the change that resolved them; the open list is four tracked items (#47–#50). | `https://github.com/caberdo/terraform-provider-gravitino/issues?q=is%3Aissue` |
| `report_tracker` | Met | GitHub Issues is the tracker. | `https://github.com/caberdo/terraform-provider-gravitino/issues` |
| `report_archive` | Met | All issues and pull requests, with comments, stay public and searchable. | `https://github.com/caberdo/terraform-provider-gravitino/issues?q=is%3Aissue` |
| `enhancement_responses` | Met | Enhancement requests are answered in the issue and implemented through the linked pull request that closes it. | `https://github.com/caberdo/terraform-provider-gravitino/pulls?q=is%3Apr+is%3Aclosed` |
| `vulnerability_report_process` | Met | `SECURITY.md` publishes the process: report privately, never in a public issue, and what to include. | `blob/.github/SECURITY.md` |
| `vulnerability_report_private` | Met | Private reporting through GitHub's private vulnerability reporting, with the exact link in `SECURITY.md`. | `blob/.github/SECURITY.md` |
| `vulnerability_report_response` | N/A | No vulnerability report has been received in the last six months; `SECURITY.md` commits to acknowledging within 48 hours. | `blob/.github/SECURITY.md` |
| `build` | Met | Anyone can rebuild the provider from source: `go build ./...` or `make build`, with the Go version pinned in `go.mod`. | `blob/GNUmakefile` |
| `build_common_tools` | Met | The build uses the standard Go toolchain; no custom build language. | `blob/GNUmakefile` |
| `build_floss_tools` | Met | `go`, `make`, GoReleaser, `terraform-plugin-docs` and the Terraform CLI are all FLOSS. | `blob/.github/goreleaser.yml` |
| `test` | Met | An automated test suite ships with the project under the same licence: roughly 870 `Test*` functions across 59 of the 60 packages under `internal/`, run by `make test` / `make testacc`; `README.md` and `CONTRIBUTING.md` document how to run it. | `blob/GNUmakefile`, `blob/README.md` |
| `test_invocation` | Met | The suite is invoked the standard way for Go: `go test ./internal/...`, or `make test`. | `blob/GNUmakefile` |
| `test_most` | Met | Most tests need no server; the acceptance suite needs the shipped Docker/Podman environment (`make testacc-docker`, `make testacc-live`). | `blob/README.md#testing` |
| `test_continuous_integration` | Met | `test.yml` runs build, vet, dependency verification, example validation, the docs-drift gate and `go test -race` on every pull request and push to `main`, and the acceptance matrix on `main`; each job's result is visible on the pull request. | `blob/.github/workflows/test.yml` |
| `test_policy` | Met | `CONTRIBUTING.md` states the policy: major new functionality must come with tests, and every bug fix with a regression test. | `blob/.github/CONTRIBUTING.md` |
| `tests_are_added` | Met | Recent changes carry tests: the regression tests added with the 0.9.0 bug fixes (properties drift, inconsistent results after apply, statistics serialisation, OAuth single-flight refresh) and the version-gate tests in `internal/client` and `internal/models`. | `blob/CHANGELOG.md`, `blob/internal/client/version_test.go` |
| `tests_documented_added` | Met | The pull request template and `CONTRIBUTING.md` require tests with the exact commands to run them. | `blob/.github/PULL_REQUEST_TEMPLATE.md` |
| `warnings` | Met | CI runs `go vet` and golangci-lint (errcheck, govet with nearly all checks, ineffassign, staticcheck, unused, bodyclose, noctx, unconvert) with a pinned linter version on every change. | `blob/.github/golangci.yml`, `blob/.github/workflows/test.yml` |
| `warnings_fixed` | Met | Findings block the run: golangci-lint and gosec both fail the `Lint & Security` job (gosec after uploading SARIF). The `main` ruleset does not yet require the check, so enforcement at merge time is the maintainer's commitment — recorded as an open item in `GOVERNANCE.md` and §5. | `blob/.github/workflows/test.yml`, `blob/GOVERNANCE.md` |
| `warnings_strict` | Met | `govet` runs with `enable-all` minus `fieldalignment` and `shadow` (the two deliberate exclusions are named in the config), staticcheck's default set and gofmt are enforced. | `blob/.github/golangci.yml` |
| `know_secure_design` | Met | The design is documented with its security argument in `SECURITY.md` (trust boundaries, secure-design principles mapped to code) and `ARCHITECTURE.md` (single mediated request path, fail-safe defaults, no code execution). | `blob/.github/SECURITY.md`, `blob/ARCHITECTURE.md` |
| `know_common_errors` | Met | `SECURITY.md` lists the weakness classes relevant to a REST client in Go (injection, sensitive data exposure, MITM, insecure deserialization, XXE, unbounded input, dependency risk, supply chain) and the countermeasure or accepted limitation for each. | `blob/.github/SECURITY.md` |
| `crypto_published` | Met | The only cryptography is published, reviewed and standard: TLS (the Go standard library) for transport, and the Kerberos SPNEGO cryptography in the Apache-2.0 `gokrb5/v8` module for `auth = "kerberos"`. | `blob/go.mod`, `blob/internal/client/auth/kerberos.go` |
| `crypto_call` | Met | Cryptography is used only through the Go standard library and `gokrb5/v8`; the provider implements no primitive of its own. | `blob/internal/client/auth` |
| `crypto_floss` | Met | Every cryptographic component is FLOSS under a permissive licence (BSD-3-Clause Go standard library, Apache-2.0 `gokrb5/v8`). | `blob/NOTICE` |
| `crypto_keylength` | Met | Key lengths are the library defaults and meet NIST's through-2030 minimums: TLS 1.2+ with ≥128-bit ciphers (Go default), and `gokrb5`'s AES/RC4-HMAC Kerberos enctypes. The provider configures nothing weaker and exposes no option to lower a key length. | `blob/internal/client/client.go` |
| `crypto_working` | Met | No broken algorithm or mode is selected by the provider: no MD4/MD5, DES/3DES or RC4 of its own, and no TLS configuration at all — the Go defaults apply, which exclude SSLv3 and TLS 1.0/1.1. | `blob/internal/client/client.go` |
| `crypto_weaknesses` | Met | Same as above: the provider adds no weak primitive, and the deprecated Kerberos enctypes are the server's choice, not the client's. | `blob/internal/client/auth/kerberos.go` |
| `crypto_pfs` | Met | The Go TLS stack negotiates ephemeral (EC)DHE key exchange by default and the provider pins no cipher suite, so forward secrecy is the default. | `blob/internal/client/client.go` |
| `crypto_password_storage` | N/A | The provider authenticates *to* Gravitino with credentials the user supplies; it stores no password for authenticating external users. Password storage for users of Gravitino is the server's job, and passwords the user puts in a provider block are marked `Sensitive` and never logged. | `blob/internal/provider/provider.go` |
| `crypto_random` | N/A | The provider generates no cryptographic keys and no nonces; it signs nothing and encrypts nothing itself. The one random value it produces is a test-name suffix in `internal/acceptance`. | `blob/internal/acceptance/live.go` |
| `delivery_mitm` | Met | Releases are delivered over HTTPS/SSH from GitHub and the Terraform Registry; Terraform records the release checksums in `.terraform.lock.hcl`. | `blob/.github/workflows/release.yml` |
| `delivery_unsigned` | Met | Nothing is verified by an unsigned hash: the release's `SHA256SUMS` file is GPG-signed (`…_SHA256SUMS.sig`) and published next to the artifacts, the build carries SLSA v1.0 provenance verifiable with `gh attestation verify`, and Terraform itself re-checks the recorded checksums in `.terraform.lock.hcl`. | `blob/README.md#verifying-a-release`, `https://github.com/caberdo/terraform-provider-gravitino/releases/tag/v0.9.0` |
| `vulnerabilities_fixed_60_days` | Met (judgement call, §6.1) | `querybatch` against OSV for all 154 non-main modules reports advisories in six of them. Every advisory older than 60 days is confined to build/test tooling that is not in the released binary (`go-git` v5 and `go-billy` v5 via the docs/registry tooling, `goldmark` v1.7.7 via `tfplugindocs`, `x/crypto/openpgp`, a package this provider does not import). The advisories that *are* in the binary's dependency graph (`golang.org/x/net` v0.59.0, five HTTP/2 issues published 2026-10-08) are ten days old and have a published fix; they are the top open item in §5. | `blob/go.mod`, `blob/.github/workflows/release.yml` |
| `vulnerabilities_critical_fixed` | Met | No open critical vulnerability; the OSV query above is the check, and §5 tracks the fix for the one advisory group that is in the binary graph. | — |
| `no_leaked_credentials` | Met | No valid private credential is in the repository or its history: the credentials in `examples/` and the test fixtures are `admin`/`admin` against a local container, `.gitignore` excludes key material, provider credentials come from the block or `GRAVITINO_*` at runtime, and the four credential-bearing attributes are `Sensitive`. | `blob/.gitignore`, `blob/internal/provider/provider.go` |
| `static_analysis` | Met | Applied before every release and on every pull request: gosec (security rules, SARIF uploaded to code scanning) and golangci-lint (staticcheck, govet, errcheck, bodyclose). | `blob/.github/workflows/test.yml` |
| `static_analysis_common_vulnerabilities` | Met | gosec's rule set targets the common Go/CWE classes (G101 secrets, G107 URL injection, G304 file inclusion, G4xx TLS), on top of staticcheck's correctness rules. | `blob/.github/workflows/test.yml` |
| `static_analysis_fixed` | Met | The gosec step uploads its SARIF to code scanning and then fails the job, so a finding is visible in two places rather than merged past unnoticed; the maintainer fixes findings before merging. The ruleset does not yet require the check (§5 item 11). | `blob/.github/workflows/test.yml` |
| `static_analysis_often` | Met | Both tools run on every pull request and every push to `main`, far more often than once per six months. | `blob/.github/workflows/test.yml` |
| `dynamic_analysis` | Met | Every `main` build runs the acceptance suite against real Gravitino **1.3.0 and 1.3.1** containers and the unit suite under `-race`. | `blob/.github/workflows/test.yml` |
| `dynamic_analysis_unsafe` | N/A | The provider is Go, a memory-safe language, and `CGO_ENABLED=0` keeps C out of the released binary. | `blob/.github/goreleaser.yml` |
| `dynamic_analysis_enable_assertions` | Met | CI runs the suite with `-race` and no build tags that disable checks, so a data race or panic fails the job. | `blob/.github/workflows/test.yml` |
| `dynamic_analysis_fixed` | Met | No exploitable vulnerability has been reported by the acceptance suite or the race detector; if one were, it would be treated as a release blocker by the same policy as static findings (the `Test` job fails, and a red run is not merged — §5 item 11 covers making that mechanical). | `blob/.github/workflows/test.yml` |

### 4.2 Passing — SHOULD / SUGGESTED criteria (no level requires them; answer them anyway)

| Criterion | Answer | Justification |
|---|---|---|
| `contribution_requirements` | Met | `CONTRIBUTING.md` states the standards, the tests, the docs regeneration and the one-change-per-pull-request rule. |
| `dco` | N/A | A Developer Certificate of Origin is not required; contributions are licensed under Apache-2.0 by the terms in `CONTRIBUTING.md`. |
| `bus_factor` (SHOULD at this level) | **Unmet** | One maintainer — see §5. |
| `accessibility_best_practices` | N/A | The project produces no graphical user interface; the documentation is plain Markdown. |
| `internationalization` | N/A | A provider/CLI context: error strings and documentation are English-only by convention, which the `english` criterion expects. |
| `build_preserve_debug` | Met | Release flags are `-s -w` (symbols stripped) but `-trimpath` and `panic` line information remain, and a debug build is one `go build` away; `-debug` runs the provider under a debugger. |
| `build_repeatable` | Met (see §6.5) | Release inputs are pinned (`go.mod`/`go.sum`), the build is `CGO_ENABLED=0` with `-trimpath` and `mod_timestamp` from the commit, and the release is built from the tagged revision. CI does not yet compare two rebuilds byte for byte. |
| `test_most` | Met | `make test` needs no server; the acceptance layers are one `make` target away. |
| `warnings_strict` | Met | See §4.1. |
| `crypto_algorithm_agility` | Met | TLS cipher suites and the Kerberos enctype negotiation come from the libraries, so a broken algorithm is replaced by a dependency update rather than a code change. |
| `interfaces_current` | Met | The provider targets the current Gravitino REST API (1.3.0/1.3.1) and documents the version gates for both; the Terraform plugin protocol is 6.0, the current version. |

### 4.3 Silver (level 1) — MUST criteria

| Criterion | Answer | Justification | Evidence |
|---|---|---|---|
| `achieve_passing` | Met | §4.1 answers every Passing MUST criterion. | `https://www.bestpractices.dev/projects/PROJECT_ID` |
| `contribution_requirements` | Met | `CONTRIBUTING.md`: standards, tests, docs, review expectations. | `blob/.github/CONTRIBUTING.md` |
| `governance` | Met | `GOVERNANCE.md` documents the model: roles, how decisions are made, the CI gate, continuity and how maintainers are added. | `blob/GOVERNANCE.md` |
| `code_of_conduct` | Met | Contributor Covenant **2.0** in the standard location. | `blob/CODE_OF_CONDUCT.md` |
| `roles_responsibilities` | Met | `GOVERNANCE.md` names the roles (maintainer, organization owner, contributor, security reporter) and their responsibilities, including who holds the release key and the registry namespace. | `blob/GOVERNANCE.md#roles` |
| `access_continuity` | **Unmet — see §5** | The repository, registry namespace and release identity belong to the `caberdo` organization and the release procedure is written down, but there is one maintainer: a second person with organization access and access to the signing secrets is required before the project could release within a week if that person were lost. | `blob/GOVERNANCE.md#access-and-continuity` |
| `documentation_roadmap` | Met | `ROADMAP.md` states what the project intends to do next, in order, and what it explicitly will not do. | `blob/ROADMAP.md` |
| `documentation_architecture` | Met | `ARCHITECTURE.md` documents the entry point, provider wiring, the client and its auth providers, the request flow, the error model, version detection and the gates, the repository layout, the test layers and the release pipeline. | `blob/ARCHITECTURE.md` |
| `documentation_security` | Met | `SECURITY.md` states what users can and cannot expect: TLS verified with no switch to disable it, credentials never logged, signed checksums, and the limits (state contents, plaintext URIs, the Gravitino server's own security). | `blob/.github/SECURITY.md` |
| `documentation_quick_start` | Met | README shows a working provider block plus resource configuration, `examples/complete/main.tf` is a full configuration, and the acceptance suites run against the shipped container environment. | `blob/README.md`, `blob/examples/complete/main.tf` |
| `assurance_case` | Met | `SECURITY.md` carries the assurance case: requirements, six trust boundaries, secure-design principles mapped to code, the weakness-class table, hardening and the accepted limitations. | `blob/.github/SECURITY.md#assurance-case` |
| `documentation_current` | Met | `docs/` is regenerated by `make generate` and CI fails on drift; examples are validated against the built provider; `ARCHITECTURE.md`, `GOVERNANCE.md`, `ROADMAP.md` and `AGENTS.md` were corrected in this change. | `blob/.github/workflows/test.yml`, `blob/.github/workflows/docs.yml` |
| `documentation_achievements` | Met once §3 lands | The badge is linked from the README as soon as it exists; the front page also links the Terraform Registry. | `blob/README.md` |
| `sites_password_security` | N/A | No project site stores passwords for authenticating users: GitHub, the Terraform Registry and the repository itself. | — |
| `maintenance_or_update` | Met | Users follow the Registry's latest version and `terraform init -upgrade`; `CHANGELOG.md` documents every release including breaking changes; attributes that cannot change in place carry `RequiresReplace()` (71 uses) instead of silently doing nothing. | `blob/CHANGELOG.md`, `blob/README.md` |
| `report_tracker` | Met | GitHub Issues. | `https://github.com/caberdo/terraform-provider-gravitino/issues` |
| `vulnerability_report_credit` | N/A | No vulnerability report has been resolved in the last 12 months; `SECURITY.md` states the credit policy for when one is. | `blob/.github/SECURITY.md` |
| `vulnerability_response_process` | Met | `SECURITY.md` documents the process and timelines: acknowledge within 48 hours, assess within 7 days, fix within 14 days. | `blob/.github/SECURITY.md` |
| `coding_standards` | Met | Identified and required: `gofmt`, `go vet`, the repository's golangci-lint configuration, and the provider-specific rules (error handling, 404 handling, logging, plan modifiers, enum validators) in `CONTRIBUTING.md` and `AGENTS.md`. | `blob/.github/CONTRIBUTING.md` |
| `coding_standards_enforced` | Met | Enforced by CI with pinned versions: golangci-lint v2.13.2 with `-race` tests, `go vet`, and the docs-drift gate. | `blob/.github/golangci.yml`, `blob/.github/workflows/test.yml` |
| `build_standard_variables` | N/A | No C toolchain: pure Go with `CGO_ENABLED=0`, so `CC`/`CFLAGS`/`LDFLAGS` do not apply. | `blob/.github/goreleaser.yml` |
| `build_non_recursive` | N/A | One Go module and a flat makefile with no recursion; `go build ./...` walks the packages. | `blob/GNUmakefile` |
| `build_repeatable` | Met | See §4.2 and §6.5. | `blob/.github/goreleaser.yml` |
| `installation_common` | Met | Users install the provider the standard way for Terraform (`terraform init` from the Registry); developers use `make install` into `~/.terraform.d/plugins` for `dev_overrides`. | `blob/README.md`, `blob/GNUmakefile` |
| `installation_standard_variables` | N/A | There is no installer: the artifact is a provider plugin published through the Registry, which owns the install location. | — |
| `installation_development_quick` | Met | A developer is productive with `git clone`, `go mod download` and `make test`; the acceptance environment is `make testacc-docker` or `make testacc-live` and needs no manual configuration. | `blob/.github/CONTRIBUTING.md` |
| `external_dependencies` | Met | All external dependencies are declared machine-readably in `go.mod`/`go.sum` (7 direct, 155 modules total), and each release publishes an SPDX-JSON SBOM. | `blob/go.mod` |
| `dependency_monitoring` | Met (with a caveat in §5) | Dependabot opens version-update pull requests daily for `gomod`, `docker-compose` and `github-actions`; the maintainer also queries OSV for the module graph. Dependabot *security* alerts and secret scanning are currently disabled in the repository settings — an owner task, not a code change. | `blob/.github/dependabot.yml` |
| `updateable_reused_components` | Met | Every reused component is a versioned Go module declared in `go.mod` with hashes in `go.sum`; nothing is vendored or copied in, so an update is `go get` plus a pull request. | `blob/go.mod` |
| `automated_integration_testing` | Met | Every pull request runs build, vet, example validation, docs-drift check and `go test -race` with a coverage artifact; every push to `main` runs the acceptance matrix against real Gravitino servers. Both publish their result as a job summary. | `blob/.github/workflows/test.yml` |
| `regression_tests_added50` | Met | The 0.9.0 bug fixes came with tests (property drift, inconsistent results after apply, statistics conversion, OAuth refresh), and the pull request template makes a regression test a checklist item. | `blob/CHANGELOG.md`, `blob/.github/PULL_REQUEST_TEMPLATE.md` |
| `test_statement_coverage80` | Met | The unit + acceptance profile clears the 80% requirement, and `scripts/coverage-gate.sh` (`make coverage-gate`) enforces a per-scope floor in `.github/coverage-baseline.txt` (total 82.5, including the previously weak data source, provider and Kerberos areas) on every pull request after the acceptance container. The measured total is `measured_total` in that file. | `blob/.github/workflows/test.yml`, `blob/.github/coverage-baseline.txt`, `blob/scripts/coverage-gate.sh` |
| `test_policy_mandated` | Met | Written policy in `CONTRIBUTING.md`: tests for major new functionality, a regression test for every bug fix, enforced by the pull request checklist. | `blob/.github/CONTRIBUTING.md` |
| `tests_documented_added` | Met | Same files state that tests are part of the change, with the commands. | `blob/.github/PULL_REQUEST_TEMPLATE.md` |
| `warnings_strict` | Met | Nearly all `go vet` checks enabled (only `fieldalignment` and `shadow` excluded, with the reason in the config), staticcheck and gofmt enforced. | `blob/.github/golangci.yml` |
| `implement_secure_design` | Met | The assurance case maps the principles to the implementation: one mediated request path, fail-safe defaults, allow-listed inputs, no code execution, and the limitations written down instead of hidden. | `blob/.github/SECURITY.md` |
| `crypto_weaknesses` | Met | No weak algorithm selected by the provider; TLS defaults from the standard library, Kerberos enctypes negotiated by `gokrb5`. | `blob/internal/client/client.go` |
| `crypto_credential_agility` | Met | Credentials are runtime inputs (provider block or `GRAVITINO_*`), never compiled in, so they rotate without a rebuild; the token endpoint, scope and Kerberos principal are configured the same way. | `blob/internal/provider/provider.go` |
| `crypto_credential_agility` / `crypto_certificate_verification` | Met | Verification is on by default and cannot be turned off: the provider sets no `tls.Config`, so the standard library verifies against the system roots and fails the handshake before any request is sent. There is no `insecure`/`skip_verify` attribute. | `blob/internal/client/client.go` |
| `crypto_verification_private` | Met | All HTTP happens through one `http.Client`; nothing is sent before the handshake, and HTTP/2 (via the plugin protocol and gRPC) is TLS-protected when the URI is `https`. | `blob/internal/client/client.go` |
| `crypto_used_network` (SHOULD at this level) | Met for the REST API, with the §6.2 caveat | The Gravitino endpoint and the OAuth2 token endpoint use Go's TLS stack; the scheme comes from the user's `uri`/`oauth_server_uri`, so a plaintext `http://` endpoint is possible when the user configures one — the endpoint is mandatory and explicit, and there is no way to make an HTTPS connection skip verification. | `blob/ARCHITECTURE.md` |
| `crypto_tls12` (SHOULD at this level) | Met | Go's TLS client default minimum is TLS 1.2; the provider adds no configuration that could lower it. | `blob/internal/client/client.go` |
| `signed_releases` | Met | Releases are cryptographically signed and the verification path is documented: the tag is GPG-signed by the release key (GitHub shows it **Verified**), every release carries `terraform-provider-gravitino_<version>_SHA256SUMS.sig`, and the public key is committed at `.github/release-signing-key.asc` with the `gpg --import` / `gpg --verify` steps in the README. The private key exists only as an encrypted Actions secret (`GPG_PRIVATE_KEY`, `PASSPHRASE`), used by `create-release-tag.yml` and `release.yml`, never committed and never uploaded with the artifacts. | `blob/README.md#verifying-a-release`, `blob/.github/release-signing-key.asc` |
| `version_tags_signed` (SUGGESTED) | Met | `create-release-tag.yml` signs the tag (`git tag -s`), verifies it (`git tag -v`) before pushing, and uses the signing key's identity so GitHub can map the signature to an account. | `blob/.github/workflows/create-release-tag.yml` |
| `input_validation` | Met | Inputs are typed and allow-listed: the framework decodes plan/state into concrete Go types, 30 attributes validate against explicit `OneOf` allow-lists, the provider `uri` must be a parseable `http`/`https` URL with a host, and the 1.3.1-only values are rejected with a version diagnostic rather than forwarded. Server-side names are validated by Gravitino, whose errors are surfaced verbatim. | `blob/internal/provider/provider.go`, `blob/internal/models/version.go` |
| `hardening` | Met | Documented hardening: no shell/SQL/template evaluation, typed JSON decoding, single mediated request path, request timeout, truncated error bodies, no way to disable TLS verification, `-trimpath`/`CGO_ENABLED=0` release builds, gosec + staticcheck + golangci-lint in CI. | `blob/.github/SECURITY.md#hardening-in-place` |

### 4.4 Gold (level 2) — MUST criteria

Gold is **not reachable today**: three MUST criteria are about a second, unassociated contributor,
which no code can substitute for. The answers are ready for the moment the people-side items are
true.

| Criterion | Answer | Justification | Evidence |
|---|---|---|---|
| `achieve_silver` | Met once Silver is awarded | §4.1 and §4.3 are complete except for the items in §5. | `https://www.bestpractices.dev/projects/PROJECT_ID` |
| `bus_factor` | **Unmet** | One maintainer: `dennismdejong` (137 commits) plus `dependabot[bot]`; the organization has one member. A second person with repository access and the release secrets is required. | `blob/GOVERNANCE.md#access-and-continuity` |
| `contributors_unassociated` | **Unmet** | Every contribution to date comes from one person (or Dependabot). A contributor outside `caberdo` is needed; a bot approving pull requests would make the claim untrue. | `https://github.com/caberdo/terraform-provider-gravitino/graphs/contributors` |
| `copyright_per_file` | **Unmet — out of scope for this change** | No per-file copyright statement. The licence is repository-wide (`LICENSE`, `NOTICE`) and issue #50 explicitly places per-file Apache-2.0 headers out of scope; a separate change can add them. | `blob/LICENSE`, `blob/NOTICE` |
| `license_per_file` | **Unmet — out of scope for this change** | Same as `copyright_per_file`: no `SPDX-License-Identifier` header in each source file. | `blob/LICENSE` |
| `repo_distributed` | Met | Git. | `https://github.com/caberdo/terraform-provider-gravitino` |
| `small_tasks` | **Unmet** | The `good first issue` label exists, but no open issue currently carries it. The four open issues (#47–#50) are maintainer-sized work. Labelling two or three genuinely beginner-sized issues is the fix, and only the maintainer can do it. | `https://github.com/caberdo/terraform-provider-gravitino/labels` |
| `require_2FA` | **Unmet** | The organization setting `two_factor_requirement_enabled` is `false` today. Enforcing 2FA for members with write access is an owner task in the organization's authentication-security settings; record the check here once it is on. | `https://github.com/organizations/caberdo/settings/security` |
| `secure_2FA` (SHOULD) | Partially | GitHub's own account security and the `gh` authentication path are the only entry points; no project-specific second factor exists. Answered after `require_2FA`. | — |
| `code_review_standards` | Met | `CONTRIBUTING.md` documents the review requirements: what is reviewed (correctness and error paths, tests, schema/state compatibility, docs and examples, security, Gravitino compatibility), and what blocks a merge (red CI, unresolved threads). | `blob/.github/CONTRIBUTING.md#code-review` |
| `two_person_review` | **Unmet** | Every pull request is currently merged by its author; the `main` ruleset sets `required_approving_review_count: 0` because a single-maintainer project must be able to merge. The trade-off is recorded in `GOVERNANCE.md` rather than hidden. | `blob/GOVERNANCE.md#continuous-integration-as-the-gate` |
| `build_reproducible` | Met (see §6.5) | The release build is reproducible from the tag: `go.mod`/`go.sum` pin the module graph, `-trimpath` removes checkout paths, `mod_timestamp` is the commit timestamp, `CGO_ENABLED=0` removes toolchain variance. | `blob/.github/goreleaser.yml` |
| `test_invocation` | Met | Standard Go invocation, plus the documented `make` split for the server-backed layers. | `blob/GNUmakefile` |
| `test_continuous_integration` | Met | `test.yml` integrates every pull request after lint, vet, example validation, docs drift and `go test -race`, and runs the acceptance matrix on `main`. | `blob/.github/workflows/test.yml` |
| `test_statement_coverage90` | **Unmet** | The 80% floor is met (`measured_total` in `.github/coverage-baseline.txt`); 90% needs the error and drift paths a stock server never reaches, the live-coverage gaps, and unit coverage of `internal/client` and `internal/models`. | `blob/.github/coverage-baseline.txt`, `blob/scripts/coverage-gate.sh` |
| `test_branch_coverage80` | N/A (see §6.3) | Go's coverage tooling measures statement coverage (`-covermode=set|count|atomic`); no mature FLOSS branch-coverage tool exists for Go, so this cannot be measured with the standard toolchain. | — |
| `crypto_used_network` | Met for the API path (see §6.2) | Go's TLS client defaults; HTTPS when the configured endpoint is HTTPS, and no way to weaken verification. | `blob/internal/client/client.go` |
| `crypto_tls12` | Met | Go's default minimum is TLS 1.2 and the provider configures no lower bound. | `blob/internal/client/client.go` |
| `hardened_site` | Met | GitHub and the Terraform Registry send `strict-transport-security: max-age=31536000; includeSubdomains; preload`, `x-frame-options: deny`, `x-content-type-options: nosniff` and a restrictive `content-security-policy` (measured with `curl -sI` on 2026-10-18). | `https://github.com/caberdo/terraform-provider-gravitino` |
| `security_review` | **Unmet — see §5** | `SECURITY.md` defines the requirements and the boundary, but no dated security review against a released version has been performed yet. This is the one Gold item that a solo maintainer can close without help. | `blob/.github/SECURITY.md#security-review-status` |
| `hardening` | Met | See §4.3. | `blob/.github/SECURITY.md#hardening-in-place` |
| `dynamic_analysis` | Met | The acceptance suite executes the provider against real Gravitino 1.3.0 and 1.3.1 before every release, and the unit suite runs under the race detector. | `blob/.github/workflows/test.yml` |
| `dynamic_analysis_enable_assertions` | Met | `-race` plus the framework's own validation run in CI; a panic fails the job. | `blob/.github/workflows/test.yml` |

## 5. Open items — what is not met yet, and who can fix it

| # | Criterion (level) | What has to happen | Who |
|---|---|---|---|
| 1 | `test_statement_coverage90` (Gold) | The 80% criterion is met (`measured_total` in `.github/coverage-baseline.txt`); the remaining gap to 90% is unit tests for the error and drift paths of the packages `make coverage` ranks lowest — `internal/datasources/schema`, `internal/datasources/secrets`, `internal/datasources/policy`, `internal/models` — measured as merged profiles, since per-package numbers understate them. Raise `floor_*` in `.github/coverage-baseline.txt` as tests land. | Code work |
| 2 | `documentation_achievements` (Silver) | Merge the badge line from §3 after registration. | Maintainer |
| 3 | `access_continuity` + `bus_factor` (Silver/Gold) | A second person with `maintain`/`admin` on the organization, the release secrets and a documented hand-over in `GOVERNANCE.md`. | Maintainer |
| 4 | `contributors_unassociated` (Gold) | A significant contributor outside the `caberdo` organization. | Maintainer + community |
| 5 | `two_person_review` (Gold) | ≥50% of changes reviewed by someone other than the author: set `required_approving_review_count: 1` and make `.github/CODEOWNERS` name a real user or team once item 3 exists (today it names an *organization*, which GitHub cannot resolve as a code owner). | Maintainer |
| 6 | `small_tasks` (Gold) | Label two or three beginner-sized issues `good first issue` so newcomers have an entry point. | Maintainer |
| 7 | `require_2FA` (Gold) | Turn on the organization's 2FA requirement for members with write access, and enable secret scanning with push protection while there. | Organization owner |
| 8 | `security_review` (Gold) | Perform and date a security review against the requirements and boundary already written in `SECURITY.md`; a self-review by the maintainer against the threat model satisfies the criterion. | Maintainer |
| 9 | `copyright_per_file` / `license_per_file` (Gold) | Add a copyright line and `SPDX-License-Identifier: Apache-2.0` to every source file. Deliberately out of scope of issue #50. | Code work |
| 10 | `vulnerabilities_fixed_60_days` (Passing) | Bump `golang.org/x/net` to v0.60.0, the `go` directive to 1.27.2 (the stdlib `net/http` HTTP/2 fixes are in the 1.27.2 toolchain, and CI currently builds with the version from `go.mod`), and `goldmark` to v1.7.17 for the docs tooling, so the OSV answer stops resting on a reachability argument. | Code work |
| 11 | `warnings_fixed`, `test_continuous_integration` hardening (Passing/Silver) | Add required status checks (`Lint & Security`, `Test`, `Coverage`) to the `main` ruleset so a red run cannot be merged, instead of relying on the maintainer's discipline. | Maintainer (repository settings) |

**Realistic target.** Items 1–2 and 10–11 unlock **Silver (7)**; items 3–9 are the price of
**Gold (10)** and are mostly about people, not code.

## 6. Judgement calls a reviewer may push back on

1. **`vulnerabilities_fixed_60_days`.** Six modules in the graph have OSV advisories. Four of them
   (`go-git` v5.18.0, `go-billy` v5.8.0, `goldmark` v1.7.7, `x/crypto/openpgp`) are not in
   `go list -deps .`: they belong to the docs-generation and registry tooling, and the `openpgp`
   package is one this provider never imports. The two that *are* in the released binary are
   `golang.org/x/net` v0.59.0 and `golang.org/x/crypto` v0.57.0 — the `x/net` HTTP/2 advisories
   (published 2026-10-08, fix in v0.60.0) are the ones to take seriously, since the plugin protocol
   is gRPC over HTTP/2. The answer above is Met because no unfixed medium-or-higher advisory is
   older than 60 days; if a reviewer wants the stronger position, land item 10 first and answer
   without the argument.
2. **`crypto_used_network`.** The provider supports HTTPS and cannot be configured to skip
   certificate verification, but it does not *force* TLS: the scheme comes from the mandatory `uri`
   (and `oauth_server_uri`), which is explicit user configuration. The alternative — refusing
   non-HTTPS URIs — would break every local Docker and Podman workflow the project ships. This is
   recorded here rather than decided silently.
3. **`test_branch_coverage80`.** Answered N/A: `go test -cover` measures statements only and there
   is no established FLOSS branch-coverage tool for Go. A reviewer could ask for `gocov`-style
   tooling; there is none that measures branch coverage for Go today.
4. **`crypto_keylength`, `crypto_random`, `crypto_password_storage`.** N/A/Met rests on the provider
   being a *client*: it generates no keys or nonces and stores no passwords for external users. The
   Gravitino server does those things and is a different project.
5. **`build_repeatable`/`build_reproducible`.** The build is configured for byte-identical results
   but CI does not perform a two-run comparison. If a reviewer asks for proof, add that step to
   `release.yml`.
6. **`hardened_site`.** Measured on the GitHub repository and the Registry landing page, which are
   the only sites the project controls; the headers differ between them but both are non-permissive.

## 7. Where the evidence lives, and how the numbers were measured

| Topic | File |
|---|---|
| What the software does, provider configuration, resource/data-source inventory, testing | [`README.md`](../README.md) |
| Runtime structure, version gates, test layers, release pipeline | [`ARCHITECTURE.md`](../ARCHITECTURE.md) |
| Roles, decisions, CI gate, continuity | [`GOVERNANCE.md`](../GOVERNANCE.md) |
| What the project intends to do, and not do | [`ROADMAP.md`](../ROADMAP.md) |
| Security policy, requirements, threat model, assurance case, limitations | [`.github/SECURITY.md`](./SECURITY.md) |
| Contribution process, coding standards, review, testing policy | [`.github/CONTRIBUTING.md`](./CONTRIBUTING.md) |
| Live acceptance suite and the server quirks it documented | [`contributing/live-acceptance-tests.md`](../contributing/live-acceptance-tests.md) |
| Licence and what the project derives from | [`LICENSE`](../LICENSE), [`NOTICE`](../NOTICE) |
| Release signing, provenance, SBOM, checksums | [`.github/goreleaser.yml`](./goreleaser.yml), [`.github/workflows/release.yml`](./workflows/release.yml), [`.github/workflows/create-release-tag.yml`](./workflows/create-release-tag.yml) |
| Release signing key and the user-facing verification steps | [`.github/release-signing-key.asc`](./release-signing-key.asc), [`README.md#verifying-a-release`](../README.md#verifying-a-release) |
| Lint and vet configuration | [`.github/golangci.yml`](./golangci.yml) |
| Coverage baseline, floor, README badge/block and the scripts that read them | [`.github/coverage-baseline.txt`](./coverage-baseline.txt), [`scripts/coverage-gate.sh`](../scripts/coverage-gate.sh), [`scripts/coverage-readme.sh`](../scripts/coverage-readme.sh), [`scripts/coverage-summary.sh`](../scripts/coverage-summary.sh), [`scripts/merge-coverage.sh`](../scripts/merge-coverage.sh) |
| Dependency updates | [`.github/dependabot.yml`](./dependabot.yml) |

Commands behind every figure in this pack (run from the repository root; counts move with every test, so the command is the source of truth):

| Figure | Command |
|---|---|
| 25 resources, 55 data sources | `ls docs/resources \| wc -l`, `ls docs/data-sources \| wc -l` |
| 869 `Test*`, 72 `TestAcc*`, 26 `TestLiveAcc*` | `grep -rh '^func Test' --include='*_test.go' . \| wc -l`, same with `TestAcc`/`TestLiveAcc` |
| 60 packages, 59 with tests | `go list ./internal/... \| wc -l`, `find internal -name '*_test.go' -exec dirname {} + \| sort -u \| wc -l` |
| 312 Go files, 139 test files, 28,312 non-test / 42,254 test lines | `find . -name '*.go' \| wc -l`, `cat` the sets into `wc -l` |
| 12 `Sensitive` attributes, 35 `OneOf`, 84 `RequiresReplace`, 41 `UseStateForUnknown` | `grep -r 'Sensitive: *true' internal/ \| wc -l`, same for the other patterns |
| Merged coverage (the gated number) and its floors | `make coverage`, `make testacc-docker`, `make coverage-merge`, `make coverage-gate`, `make coverage-readme-check` (`scripts/coverage-summary.sh`, `scripts/merge-coverage.sh`, `scripts/coverage-gate.sh`, `scripts/coverage-readme.sh`, `.github/coverage-baseline.txt`) |
| Unit coverage per package | `make coverage`, or `go test -coverprofile=c.out ./internal/...` then `go tool cover -func=c.out \| tail -1` |
| 7 direct dependencies, 155 modules | `awk` over `go.mod`, `go list -m all \| wc -l` |
| OSV advisories per module | `go list -m all` → `POST https://api.osv.dev/v1/querybatch` |
| Ruleset, 2FA, secret scanning, contributors, 24 tags | `gh api repos/caberdo/terraform-provider-gravitino/…` |
| Site hardening headers | `curl -sI https://github.com/caberdo/terraform-provider-gravitino` |

When a criterion's answer changes, update this table, the answer row, and the corresponding open
item in §5 in the same pull request.
