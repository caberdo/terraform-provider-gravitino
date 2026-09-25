# Gravitino Terraform Provider Examples

This directory contains example Terraform configurations for the Gravitino provider, demonstrating how to manage Gravitino resources using infrastructure as code.

- **provider/** — Provider configuration
- **resources/** — One directory per resource, containing `resource.tf` (embedded verbatim in
  the generated `docs/resources/*.md`, so it must be valid HCL)
- **data-sources/** — Data source queries (listing and reading resources)
- **complete/** — A larger, end-to-end configuration that combines resources

## Validating the examples

Example files are published to the Terraform Registry as provider documentation, so an example
that references a removed or renamed attribute ships a broken snippet to users. Validate every
example against the provider built from this working tree:

```bash
make validate-examples     # or: ./scripts/validate-examples.sh
```

The script builds the provider, points Terraform at it through `dev_overrides`, and runs
`terraform validate` in every module under `examples/`. CI runs it on every pull request.
