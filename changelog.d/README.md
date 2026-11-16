# Changelog fragments

Every user-visible change lands with one fragment in this directory instead of
an edit to `CHANGELOG.md`. Feature pull requests therefore never touch a shared
release file, and two pull requests cannot conflict in the changelog.

## Naming

```
<issue>-<kind>-<slug>.md
```

- `<issue>` is the GitHub issue or pull request number, used to order entries.
- `<kind>` is one of `breaking`, `feature`, `improvement`, `bugfix`, `security`
  or `chore`. It is also the release-note category, so no YAML front matter is
  needed.
- `<slug>` is a short lowercase description with words separated by hyphens.

Examples:

```
94-bugfix-tls-destroy-warning.md
117-feature-generated-resource-tables.md
118-security-lock-down-docs-workflow.md
```

## Content

The body is the release-note entry, written as Markdown without the leading
`- `. The release assembly adds the bullet and indents continuation lines.

```markdown
`gravitino_catalog` manages a catalog end to end. Destroying it removes only
the remote catalog; nothing local is kept.
```

Keep the first sentence self-contained: it is what most readers read. Mention
the Terraform type names and the server versions that matter, and describe the
effect on existing state when behaviour changes.

## Releasing

The release pull request is the only place that edits `CHANGELOG.md`. Before
tagging, it groups every fragment by kind, sorts each group by issue number,
inserts a dated section below the always-empty `## Unreleased` heading and
deletes the processed fragments. The newest `CHANGELOG.md` section names the
version, and the tag is cut from the merge commit after this pull request
merges; the repository has no `VERSION` file, and the release pull request does
not edit one. A change that no user can observe needs no fragment; label such a
pull request `skip-changelog` instead.
