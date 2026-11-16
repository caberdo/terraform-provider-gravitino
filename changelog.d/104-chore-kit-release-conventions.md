The CI, release and coverage tooling now comes from `caberdo/terraform-provider-kit` at
`v0.13.0`. The released version is the last git tag rather than a `VERSION` file, every pull
request that changes code, workflows, docs or examples adds a fragment under `changelog.d/`, and
the README install constraint and changelog headings were repaired where they disagreed.
