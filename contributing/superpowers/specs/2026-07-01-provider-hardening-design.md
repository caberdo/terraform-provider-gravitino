# Provider Hardening — Lessons from terraform-provider-minio

**Date:** 2026-07-01
**Status:** Design approved

## Motivation

The terraform-provider-gravitino codebase has complete CRUD + import for all 13 resources, but lags behind production-grade Terraform providers in error handling, observability, testing infrastructure, and developer guides. The terraform-provider-minio codebase demonstrates proven patterns that can be borrowed.

## Approach: 3-Phase Incremental

| Phase | Name | Areas |
|-------|------|-------|
| 1 | Foundation | Error utility, tflog logging, AGENTS.md expansion |
| 2 | DevOps | Docker Compose acceptance tests, CI/CD additions, Makefile targets |
| 3 | Polish | Random test names, diagnostics audit, final AGENTS.md polish |

Phases are independently testable. Phase 2 depends on Phase 1 (Docker tests benefit from structured logs). Phase 3 is refinements with no downstream dependencies.

---

## Phase 1: Foundation

### 1.1 Centralized Error Utility

**New file:** `internal/client/errors.go`

```go
func NewResourceError(operation, resource string, err error) diag.Diagnostics
func IsNotFoundError(err error) bool
```

**Behavior:**
- Extracts `models.ErrorResponse` details (code, type, message, stack) when present
- Provides human-friendly hints for common failures:
  - 404 → "Resource not found. It may have been deleted outside Terraform."
  - Connection refused → "The Gravitino server is unreachable. Verify the URI."
  - TLS/cert errors → "TLS error. If using self-signed certificates, verify your CA."
  - Auth failures → "Authentication failed. Check your credentials."
- Falls back to raw error message for unknown errors
- `IsNotFoundError`: wraps the `strings.Contains(err.Error(), "404")` pattern (used 11 times across resources)

**Current state:** 82 calls to `resp.Diagnostics.AddError("Failed to ...", err.Error())` across 13 resource files, all identical plain-string patterns. Zero error detail extraction.

### 1.2 Update All 13 Resources

Every `resp.Diagnostics.AddError("Failed to ...", err.Error())` replaced with:

```go
resp.Diagnostics.Append(client.NewResourceError("creating catalog", plan.Name.ValueString(), err)...)
```

Every 404 check replaced with:

```go
if client.IsNotFoundError(err) {
    resp.State.RemoveResource(ctx)
    return
}
```

**Files touched:** All 13 resource files + client files that return errors.

### 1.3 Structured tflog Logging

Add `tflog.Debug()` at CRUD boundaries:

```go
tflog.Debug(ctx, "Creating catalog", map[string]interface{}{
    "metalake": plan.Metalake.ValueString(),
    "name":     plan.Name.ValueString(),
})
```

**Coverage:** Start/end of Create, start of Read, start/end of Update, start/end of Delete — all 13 resources.

**Dependency:** `terraform-plugin-log` is already in `go.mod` as indirect (`v0.10.0`). Only needs explicit import + `go mod tidy`.

**Current state:** Zero logging anywhere in resource code, client code, or data sources.

### 1.4 AGENTS.md Expansion

Add to existing AGENTS.md (created 2026-07-01):

**Quick reference section:**
- Error handling: always use `client.NewResourceError`
- 404 check: always use `client.IsNotFoundError`
- Logging: always `tflog.Debug` at CRUD boundaries

**Hard guardrails:**
- Never use `resp.Diagnostics.AddError` directly
- Never use `fmt.Println` / `println` in resources
- Never ignore diagnostics from state/type conversion calls

**Checklist for new resources:**
- [ ] Error handling via `client.NewResourceError`
- [ ] 404 via `client.IsNotFoundError`
- [ ] tflog.Debug at CRUD boundaries
- [ ] Import with dot-separated ID parsing (valid + invalid test)
- [ ] Unit tests: schema, create, delete, import

**Updated conventions section:**
- Update pattern reference with exact code snippet
- Configure pattern reference

---

## Phase 2: DevOps

### 2.1 Docker Compose for Acceptance Tests

**New file:** `docker-compose.yml` at repo root

```yaml
services:
  gravitino:
    image: apache/gravitino:0.8.0-incubating
    ports: ["8090:8090", "9001:9001"]
    environment:
      GRAVITINO_SERVER_HTTP_PORT: 8090
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8090/api/health"]
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 30s

  test:
    image: golang:1.26.4
    volumes:
      - .:/app
      - go-modules:/go/pkg/mod
    working_dir: /app
    command:
      - bash
      - -c
      - |
        go mod download && go mod verify
        go test -v -cover -timeout 30m ./internal/...
    environment:
      TF_ACC: "1"
      GRAVITINO_URI: "http://gravitino:8090"
      GRAVITINO_AUTH: "none"
    depends_on:
      gravitino:
        condition: service_healthy

volumes:
  go-modules:
```

**New file:** `.dockerignore` — exclude `.git`, vendor, build artifacts.

### 2.2 CI Acceptance Test Job

**Update:** `.github/workflows/test.yml`

Add a job `acceptance-tests` that runs `docker compose run --rm test`. Runs on push/PR to `main` only.

### 2.3 Makefile Enhancements

**Update:** `GNUmakefile`

Add targets:
- `testacc-docker`: `docker compose run --rm test`
- `lint-fix`: `golangci-lint run --fix ./...`

Existing targets (build, test, testacc, vet, fmt, lint, install, generate, clean) already exist.

---

## Phase 3: Polish

### 3.1 Random Test Names (Acceptance Only)

Unit tests with `httptest.NewServer` keep hardcoded names (no collision risk).

New acceptance tests in Phase 2 use `acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)` for resource names.

### 3.2 Diagnostics Consistency Audit

Manual audit pass over all 13 resource files:

- [ ] Every `types.MapValueFrom` / `types.ObjectValue` call correctly checks returned diagnostics
- [ ] `resp.State.Set()` errors always guarded by `resp.Diagnostics.HasError()` before return
- [ ] `Configure` / `SetClient` pattern consistent across all resources
- [ ] No silent error discards

This is a verification pass, not a rewrite. Expected findings: minor inconsistencies in property/aidit conversion helpers.

### 3.3 Final AGENTS.md Polish

After Phases 1-3 complete, update AGENTS.md with:
- Final error handling reference (exact import + usage)
- Final logging reference (exact import + usage)
- Final test patterns (unit vs acceptance)
- Docker Compose commands for local testing
- Makefile target reference

---

## Files Summary

| # | Area | New Files | Modified Files |
|---|------|-----------|----------------|
| 1.1 | Error utility | `internal/client/errors.go` | — |
| 1.2 | Error refactor | — | 13 resources, 20+ client files |
| 1.3 | tflog logging | — | 13 resources |
| 1.4 | AGENTS.md v1 | — | `AGENTS.md` |
| 2.1 | Docker Compose | `docker-compose.yml`, `.dockerignore` | — |
| 2.2 | CI acceptance | — | `.github/workflows/test.yml` |
| 2.3 | Makefile | — | `GNUmakefile` |
| 3.1 | Random names | — | Acceptance test files (new) |
| 3.2 | Audit | — | 13 resources (minor fixes) |
| 3.3 | AGENTS.md v2 | — | `AGENTS.md` |

---

## Constraints

1. **go build ./...** must pass after each phase
2. **go test -v -cover ./internal/...** must pass after each phase (unit tests continue to use httptest.NewServer)
3. **No API changes** to any Terraform resource schema — all changes are internal refactoring
4. **No changes to user-facing behavior** — error messages may improve but same errors still returned
5. **Existing lint rules** (.github/golangci.yml) must pass
