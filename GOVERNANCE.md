# Governance

How decisions are made in `caberdo/terraform-provider-gravitino`, who is responsible for what, and
what happens if a maintainer steps away. Kept deliberately short and factual: this is a
single-repository, effectively single-maintainer project today, and the document says so instead of
describing a committee that does not exist. Settings quoted here were read from the GitHub API on
2026-10-18; where a setting is missing, that is stated as an open item rather than implied.

## Roles

| Role | Who | Responsibilities |
|---|---|---|
| **Maintainer** | [@dennismdejong](https://github.com/dennismdejong) | Final say on scope and design; review and merge pull requests; cut releases (tag + GoReleaser run); triage issues; handle private vulnerability reports; hold the release signing key and the repository's Actions secrets |
| **Organization owner** | the [`caberdo`](https://github.com/caberdo) GitHub organization (one member) | Owns the repository, the Terraform Registry namespace `caberdo/gravitino` and the release identity, so ownership does not disappear with a personal account |
| **Contributor** | anyone | Propose changes through pull requests; file reproducible bug reports; take part in issue discussion. Requirements are in [`CONTRIBUTING.md`](./.github/CONTRIBUTING.md) |
| **Security reporter** | anyone | Report vulnerabilities privately as described in [`SECURITY.md`](./.github/SECURITY.md); reporters are credited in the release notes unless they ask otherwise |

There is no separate release manager, documentation owner or triage team: the maintainer holds those
responsibilities and the workflows in `.github/workflows/` do the mechanical parts.

## How decisions are made

1. **Anything non-trivial starts as an issue.** Bugs and features arrive through the issue forms in
   `.github/ISSUE_TEMPLATE/`. Design questions are decided *in* the issue, with the alternatives and
   the reasoning written down, so a later reader can see why the code looks the way it does.
2. **Every change lands through a pull request against `main`.** The `main` ruleset is active on the
   default branch and forbids deletion and force-pushes; all three merge methods (merge, squash,
   rebase) are allowed.
3. **The maintainer decides.** Where an issue reaches no consensus, the maintainer records the
   decision and its rationale in the issue and moves on. Disagreement is expressed in the issue,
   not by re-litigating a merged change.
4. **Releases are cut, not continuous.** `Create Release Tag` computes the next version from `main`,
   signs and pushes the tag, and invokes the `Release` workflow, which builds with GoReleaser,
   signs the checksums, attests build provenance, attaches the SPDX SBOM and publishes to the
   Terraform Registry; the public verification steps are in
   [`README.md`](./README.md#verifying-a-release). A failed release is republished by dispatching
   `Release` with the existing tag. `CHANGELOG.md` is updated per release; breaking changes are
   called out in it.
5. **Backwards compatibility is a commitment.** Terraform type and attribute names are public API:
   renaming `gravitino_metalake` or an attribute silently breaks user state. An attribute the API
   cannot change in place carries `RequiresReplace()` (71 uses today) instead of silently doing
   nothing, and Go package moves never change a Terraform type name.

## Continuous integration as the gate

| Check | Workflow | Runs on | Blocks what |
|---|---|---|---|
| `Lint & Security` — golangci-lint (pinned v2.13.2) and gosec (SARIF → code scanning, job fails on findings) | `test.yml` | every pull request, every push to `main` | a red run is treated as blocking; see the note below |
| `Test` — build, `go vet`, `go mod tidy`/`verify`, cross-platform build, `make validate-examples`, generated-docs drift, `go test -race` with coverage | `test.yml` | every pull request, every push to `main` | same |
| `Dependency Review` | `test.yml` | pull requests | vulnerable dependency introduced by the pull request |
| `Coverage` — the acceptance container's profile merged with the unit profile, checked against `.github/coverage-baseline.txt` by `scripts/coverage-gate.sh`, plus the README/baseline drift check (`scripts/coverage-readme.sh --check`) | `test.yml` | pull requests | a change that drops a scope below its coverage floor, or a README coverage block that no longer matches the enforced baseline (the gate exits 2, not 0, when the profile or a floor is missing) |
| `Acceptance Tests (Gravitino 1.3.0 / 1.3.1)` | `test.yml` | push to `main` | a release from a `main` whose acceptance matrix is red |
| `Generate Docs` (opens a PR when `docs/` drifts) / `Validate Markdown Files` | `docs.yml` | push to `main` / pull request | merged documentation that no longer matches the schema, and broken markdown links |
| `Release` — GoReleaser from a GPG-signed tag, GPG-signed checksums, SLSA v1.0 build provenance, SPDX SBOM | `release.yml` | tag `v*`, `workflow_call`, or a manual dispatch with a tag to republish | a release whose artifacts are unsigned or un-attested |

**Honest state of enforcement.** The `main` ruleset currently requires a pull request and a
code-owner review but sets `required_approving_review_count: 0` and declares **no required status
checks**, and GitHub's classic branch protection is not enabled on top of it. CI therefore reports
on every change, but a merge is not mechanically blocked by a red run; that is the maintainer's
commitment, not an enforced rule. Two related gaps are recorded here rather than papered over:

* Required status checks should be added to the ruleset (`Lint & Security`, `Test`, `Coverage`) so
  the gate is enforced instead of trusted.
* `.github/CODEOWNERS` reads `* @caberdo` — the *organization*, while GitHub resolves code owners
  to users or teams with write access. The ruleset's `require_code_owner_review` therefore has no
  effective owner today. Naming the maintainer directly is not a free fix: GitHub does not let an
  author satisfy a code-owner review, so `* @dennismdejong` would block the maintainer's own pull
  requests until there is a second person with write access. The decision is deliberately left to
  the maintainer.

## Access and continuity

The project must be able to keep working if one person becomes unavailable. What exists today:

* The repository, the registry namespace and the release identity belong to the `caberdo`
  organization rather than a personal account.
* The release key pair and its passphrase live in the repository's encrypted Actions secrets
  (`GPG_PRIVATE_KEY`, `PASSPHRASE`), used only by the release workflows, and the public half is
  committed at `.github/release-signing-key.asc`, so any user can verify a release without asking
  for a key; every release is reproducible from a tag by anyone with repository and secret access.
* The whole release procedure is the two workflows in `.github/workflows/`, and the architecture
  behind it is written down in [`ARCHITECTURE.md`](./ARCHITECTURE.md#release-pipeline).

**Open gap — bus factor.** There is one maintainer: the contributor list is
`dennismdejong` (137 commits) and `dependabot[bot]` (25), and the organization has one member. To
satisfy the continuity requirement — *the project can still triage issues, accept changes and
release within a week if any one person is lost* — a second person needs (1) `maintain` or `admin`
on the organization, (2) membership of the secret-holding team, and (3) a documented hand-over in
this file. Until then, continuity depends on the organization remaining reachable by its current
owner.

Two organization settings that belong to the same gap and are currently **off** (organisation
settings → Authentication security / Code security): two-factor authentication is not required for
members, and secret scanning with push protection is disabled. Turning both on is an owner task,
and neither needs a code change.

## Adding maintainers and reviewers

1. The candidate contributes at least one reviewed change and shows they can run the suites
   (`make testacc-live` with Podman, `make testacc-docker` with Docker).
2. An existing maintainer invites them as a collaborator and grants organization `maintain`.
3. When they are expected to review: fix `.github/CODEOWNERS` to name them (or a team) and set
   `required_approving_review_count: 1` plus the required status checks on the `main` ruleset. The
   decision and its date are recorded in the issue that proposed it.
4. Their name is added to the roles table above.

## Changing this document

Governance changes are proposed by pull request like any other change. If a second maintainer
joins, the roles table and the continuity section are updated in the same pull request that grants
the access, and the open items in this file are closed or restated.
