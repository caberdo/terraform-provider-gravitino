# Contributing to terraform-provider-gravitino

Thank you for considering contributing!

## Development Setup

**Prerequisites:** Go 1.27.1+, Docker, Git

```bash
git clone https://github.com/gravitino/terraform-provider-gravitino.git
cd terraform-provider-gravitino
go mod download
```

**Build:** `go build ./...`

**Tests:** `go test -v -cover ./internal/...`

**Acceptance tests (Docker):** `make testacc-docker`

## Code Conventions

See [AGENTS.md](https://github.com/gravitino/terraform-provider-gravitino/blob/main/AGENTS.md) for:
- Error handling: `client.NewResourceError`
- 404 checks: `client.IsNotFoundError`
- Logging: `tflog.Debug` at CRUD boundaries
- Enum validators: `stringvalidator.OneOf` for all enum fields
- Resource checklist

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(catalog): add support for property updates
fix(schema): handle 404 on read
docs(metalake): add import example
```

## Pull Request Process

1. Create a feature branch
2. Make changes + add tests
3. Run `go build ./...` and `go test -v -cover ./internal/...`
4. Run `golangci-lint run --config .github/golangci.yml ./...`
5. Generate docs with `make generate`
6. Submit PR with description + linked issue

## Questions?

Use [GitHub Discussions](https://github.com/gravitino/terraform-provider-gravitino/discussions) or open an issue.
