## Description

<!-- What changes and why. -->

## Linked Issues

<!-- Fixes #123 -->

## Testing

- [ ] `make build` succeeds
- [ ] `make vet` is clean
- [ ] `make test` passes (unit tests)
- [ ] `make lint` is clean
- [ ] `make generate` run and `docs/` committed, if a schema changed
- [ ] `make validate-examples` passes, if an example changed
- [ ] New or changed resources/data sources have unit tests for schema, create, update, delete and import (valid + invalid)
- [ ] A regression test covers this change, if it is a bug fix
- [ ] Mock payloads are copied from the OpenAPI `examples:` blocks, not written from the Go structs
- [ ] `make testacc` passes, if CRUD behaviour through Terraform Core changed
- [ ] `make coverage-merge && make coverage-gate` passes, and the floors in `.github/coverage-baseline.txt` are raised (with `make coverage-readme` re-run) if the change improved coverage

<!-- If this change needs a Gravitino server newer than the default, say so: -->

- [ ] Requires Gravitino >= 1.3.1 (leave unchecked if the default 1.3.0 image is enough)

## Documentation

- [ ] `make generate` run (if a schema changed; never edit `docs/` by hand)
- [ ] Examples added or updated under `examples/` (if user-facing behaviour changed)
- [ ] README/CHANGELOG updated, if the change affects usage or releases

## Checklist

- [ ] API errors go through `client.NewResourceError`
- [ ] 404 handling uses `client.IsNotFoundError` and `resp.State.RemoveResource(ctx)`
- [ ] `tflog.Debug` at the start and end of Create/Read/Update/Delete
- [ ] Import parses and validates the dot-separated hierarchy ID
- [ ] Attributes the API cannot update in place carry `RequiresReplace()`
- [ ] Computed attributes are set on every path or carry `UseStateForUnknown()`
- [ ] Enum attributes have `stringvalidator.OneOf` with the values from the API docs
- [ ] Resource or data source registered in `internal/provider/provider.go`

See [.github/CONTRIBUTING.md](./CONTRIBUTING.md) for the development setup, commands and the
full pull request process, and [ARCHITECTURE.md](../ARCHITECTURE.md) for how the provider is
put together.
