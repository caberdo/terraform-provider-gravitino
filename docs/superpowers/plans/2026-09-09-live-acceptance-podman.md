# Live Gravitino Acceptance Tests (podman) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add acceptance tests that run against a real Gravitino server (`apache/gravitino:1.3.0` via podman) and prove the responses are not mocks.

**Architecture:** New `internal/acceptance` package provides a `LivePreCheck` that requires a reachable `GRAVITINO_URI` whose `GET /api/version` returns a valid Gravitino version (optionally pinned via `GRAVITINO_EXPECT_VERSION`), plus shared provider factories and unique-name helpers. New `TestLiveAcc*` tests live alongside the existing mock tests in the metalake/catalog/tag resource packages and the health/metalake/authentication datasource packages. A new `acc` compose service + `scripts/testacc-live.sh` starts Gravitino via `podman compose`, polls `/api/health` (podman-compose 1.6.0 ignores `condition: service_healthy`), and runs the filtered live tests.

**Tech Stack:** Go 1.26.4, terraform-plugin-testing v1.16.0, podman 6.1.1 + podman-compose 1.6.0, apache/gravitino:1.3.0.

## Global Constraints

- Existing mock-based `TestAcc*` tests must NOT be modified. Live tests live in separate files.
- Live tests run only when `TF_ACC=1` AND `GRAVITINO_URI` is set to a reachable Gravitino whose `/api/version` answers (enforced by `acceptance.LivePreCheck`). Without a server they must skip, never fail on a missing server.
- Live tests must never start their own HTTP mock or call `t.Setenv("GRAVITINO_URI", ...)`.
- Resource names created against the shared server must be unique per run (`acceptance.UniqueName`).
- No code comments; follow repo conventions (`resource.Test` + `ProtoV6ProviderFactories`, `client.NewResourceError` for resource errors is not needed in tests).
- `docker-compose.yml` default `test` service and the CI acceptance job must keep working unchanged.
- Gravitino default auth is none; expected real-server behaviours (from spike): `/api/version` → `{"version":{"version":"1.3.0",...}}`, health status lowercase `"up"`, principal + audit.creator `"anonymous"`, metalake/catalog/tag audit creator equals the authenticated principal.

---
### Task 1: Client `GetVersion` + model (unit-tested)

**Files:**
- Create: `internal/models/version.go`
- Create: `internal/client/version.go`
- Test: `internal/client/version_test.go`

**Interfaces:**
- Consumes: `client.Client.Get` (`internal/client/client.go:114`), `models` package.
- Produces:
  - `models.VersionResponse{ Code int; Version Version }` with `Version Version` json tag `version`.
  - `models.Version{ Version string; CompileDate string; GitCommit string }` json tags `version`, `compileDate`, `gitCommit`.
  - `(*client.Client).GetVersion() (*models.VersionResponse, error)` — later used by `acceptance.LivePreCheck` (Task 2).

- [ ] **Step 1: Write the failing test**

Create `internal/client/version_test.go`:

```go
package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code":    0,
			"version": map[string]string{"version": "1.3.0", "compileDate": "28/06/2026", "gitCommit": "abc"},
		})
	}))
	defer server.Close()

	c, err := New(server.URL, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := c.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if got.Code != 0 {
		t.Fatalf("Code = %d, want 0", got.Code)
	}
	if got.Version.Version != "1.3.0" {
		t.Fatalf("Version.Version = %q, want 1.3.0", got.Version.Version)
	}
	if got.Version.CompileDate != "28/06/2026" {
		t.Fatalf("Version.CompileDate = %q", got.Version.CompileDate)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/client/ -run TestGetVersion -v`
Expected: compile error `c.GetVersion undefined (type *Client has no field or method GetVersion)`.

- [ ] **Step 3: Implement model + client method**

Create `internal/models/version.go`:

```go
package models

type VersionResponse struct {
	Code    int     `json:"code"`
	Version Version `json:"version"`
}

type Version struct {
	Version     string `json:"version"`
	CompileDate string `json:"compileDate"`
	GitCommit   string `json:"gitCommit"`
}
```

Create `internal/client/version.go`:

```go
package client

import "github.com/gravitino/terraform-provider-gravitino/internal/models"

func (c *Client) GetVersion() (*models.VersionResponse, error) {
	var result models.VersionResponse
	err := c.Get("/version", &result)
	return &result, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/client/ -run TestGetVersion -v`
Expected: `--- PASS: TestGetVersion`.

- [ ] **Step 5: Run lint and commit**

Run: `golangci-lint run --config .github/golangci.yml ./internal/client/...`
Expected: no findings.

```bash
git add internal/models/version.go internal/client/version.go internal/client/version_test.go
git commit -m "feat(client): add GetVersion for live-server verification"
```

---
### Task 2: Shared live-acceptance helpers package

**Files:**
- Create: `internal/acceptance/live.go`
- Test: `internal/acceptance/live_test.go`

**Interfaces:**
- Consumes: Task 1 `client.Client.GetVersion`, `provider.New("test")()` from `internal/provider`.
- Produces (used by Tasks 4-7):
  - `acceptance.LivePreCheck(t *testing.T) func()` — returns a `resource.TestCase.PreCheck` (type `func()`); skips when `GRAVITINO_URI` is unset, `t.Fatalf` when the server is unreachable / not Gravitino / version mismatch.
  - `acceptance.ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error)`
  - `acceptance.UniqueName(prefix string) string` — returns `acc_<prefix>_<unixnano>`.

- [ ] **Step 1: Write the failing unit test for `UniqueName`**

Create `internal/acceptance/live_test.go`:

```go
package acceptance

import (
	"regexp"
	"testing"
)

func TestUniqueNameFormat(t *testing.T) {
	name := UniqueName("live")
	re := regexp.MustCompile(`^acc_live_[0-9]+$`)
	if !re.MatchString(name) {
		t.Fatalf("UniqueName(\"live\") = %q, want acc_live_<digits>", name)
	}
}

func TestUniqueNameDistinct(t *testing.T) {
	if UniqueName("live") == UniqueName("live") {
		t.Fatal("UniqueName returned the same value twice")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/acceptance/ -v`
Expected: compile error `undefined: UniqueName`.

- [ ] **Step 3: Implement helpers**

Create `internal/acceptance/live.go`:

```go
package acceptance

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/client"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func LivePreCheck(t *testing.T) func() {
	return func() {
		uri := strings.TrimRight(os.Getenv("GRAVITINO_URI"), "/")
		if uri == "" {
			t.Skip("GRAVITINO_URI is not set; skipping live acceptance tests")
		}

		c, err := client.New(uri, nil)
		if err != nil {
			t.Fatalf("invalid GRAVITINO_URI %q: %v", uri, err)
		}

		ver, err := c.GetVersion()
		if err != nil {
			t.Fatalf("server at %s is not reachable or not Gravitino: %v", uri, err)
		}
		if ver.Version.Version == "" {
			t.Fatalf("server at %s returned an empty version; refusing to run against a mock", uri)
		}
		if want := os.Getenv("GRAVITINO_EXPECT_VERSION"); want != "" && ver.Version.Version != want {
			t.Fatalf("server at %s reports version %q, want %q", uri, ver.Version.Version, want)
		}
	}
}

func ProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

func UniqueName(prefix string) string {
	return fmt.Sprintf("acc_%s_%d", prefix, time.Now().UnixNano())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/acceptance/ -v`
Expected: `--- PASS: TestUniqueNameFormat` and `--- PASS: TestUniqueNameDistinct`.

- [ ] **Step 5: Commit**

```bash
git add internal/acceptance/live.go internal/acceptance/live_test.go
git commit -m "feat(acceptance): add live-server precheck and shared test helpers"
```

---
### Task 3: podman-compose `acc` profile + make targets

**Files:**
- Modify: `docker-compose.yml` (add `acc` service after the `test` service block, before `volumes:`)
- Create: `scripts/testacc-live.sh`
- Modify: `GNUmakefile`

**Interfaces:**
- Consumes: nothing new. Produces the environment Tasks 4-7 run against:
  - env `TF_ACC=1`, `GRAVITINO_URI=http://gravitino:8090`, `GRAVITINO_AUTH=none`, `GRAVITINO_USERNAME=""`, `GRAVITINO_PASSWORD=""`, `GRAVITINO_OAUTH_TOKEN=""`, `GRAVITINO_EXPECT_VERSION=1.3.0`
  - `make testacc-live` and `make testacc-live F=<pattern>` targets.

- [ ] **Step 1: Add the `acc` service to `docker-compose.yml`**

Edit `docker-compose.yml` and insert the `acc` service between the `test` service block (ends line 37 with `service_healthy`) and the `volumes:` block:

```yaml
  acc:
    image: golang:1.26.4
    volumes:
      - .:/app
      - go-modules:/go/pkg/mod
    working_dir: /app
    entrypoint: ["bash", "/app/scripts/testacc-live.sh"]
    environment:
      TF_ACC: "1"
      GRAVITINO_URI: "http://gravitino:8090"
      GRAVITINO_AUTH: "none"
      GRAVITINO_USERNAME: ""
      GRAVITINO_PASSWORD: ""
      GRAVITINO_OAUTH_TOKEN: ""
      GRAVITINO_EXPECT_VERSION: "1.3.0"
    depends_on:
      gravitino:
        condition: service_healthy
```

- [ ] **Step 2: Create the live test entrypoint script**

Create `scripts/testacc-live.sh` (must be executable):

```bash
#!/usr/bin/env bash
set -euo pipefail

URI="${GRAVITINO_URI:-http://gravitino:8090}"
FILTER="${GO_TEST_FILTER:-TestLiveAcc}"

echo "Waiting for Gravitino at ${URI}/api/health ..."
ready=""
for i in $(seq 1 60); do
  if curl -fsS -m 3 "${URI}/api/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  echo "  not ready (attempt ${i}/60)"
  sleep 2
done
if [ -z "${ready}" ]; then
  echo "Gravitino did not become ready at ${URI}" >&2
  exit 1
fi
echo "Gravitino is ready."

curl -fsS -m 5 "${URI}/api/version" >/dev/null || {
  echo "version endpoint unreachable at ${URI}" >&2
  exit 1
}

go mod download && go mod verify

go test -v -timeout 30m -run "${FILTER}" \
  ./internal/resources/metalake/ \
  ./internal/resources/catalog/ \
  ./internal/resources/tag/ \
  ./internal/datasources/health/ \
  ./internal/datasources/metalake/ \
  ./internal/datasources/authentication/
```

Run: `chmod +x scripts/testacc-live.sh`

- [ ] **Step 3: Add make targets to `GNUmakefile`**

Append inside the targets and extend `.PHONY` (edit the `testacc-docker:` block region):

```make
testacc-live:
	podman compose up -d gravitino
	podman compose run --rm acc

testacc-live-filter:
	@test -n "$(F)" || (echo "usage: make testacc-live-filter F=TestLiveAccMetalakeResource"; exit 1)
	podman compose up -d gravitino
	GO_TEST_FILTER="$(F)" podman compose run --rm acc
```

Extend the `.PHONY` line to include `testacc-live testacc-live-filter`.

- [ ] **Step 4: Verify infra end-to-end (no live tests exist yet)**

Run: `make testacc-live`
Expected output, in order:
- podman pulls/runs `apache/gravitino:1.3.0` and reports it up;
- script prints `Waiting for Gravitino at http://gravitino:8090/api/health ...` then `Gravitino is ready.`;
- `go test -v ...` prints one `ok ... [no tests to run]` line per listed package;
- exit code 0.

If the pull is slow, wait (image is large, ~600MB). If `curl` is missing inside the golang image, replace `curl` with `wget -q -O -` in `scripts/testacc-live.sh` and retry.

- [ ] **Step 5: Commit**

```bash
git add docker-compose.yml scripts/testacc-live.sh GNUmakefile
git commit -m "feat: add podman-compose profile for live Gravitino acceptance tests"
```

---
### Task 4: Live metalake acceptance test

**Files:**
- Create: `internal/resources/metalake/live_test.go` (package `metalake_test`)

**Interfaces:**
- Consumes: `acceptance.LivePreCheck(t)`, `acceptance.ProtoV6ProviderFactories()`, `acceptance.UniqueName(prefix)`.
- Produces: `TestLiveAccMetalakeResource`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/metalake/live_test.go`:

```go
package metalake_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccMetalakeResource(t *testing.T) {
	mlName := acceptance.UniqueName("metalive")

	cfgCreate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name       = %[1]q
  comment    = "live acceptance metalake"
  properties = { "env" = "dev" }
}

data "gravitino_principal" "me" {}
`, mlName)

	cfgUpdate := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name       = %[1]q
  comment    = "live acceptance metalake (updated)"
  properties = { "env" = "prod" }
}

data "gravitino_principal" "me" {}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfgCreate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "name", mlName),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "live acceptance metalake"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "dev"),
					resource.TestCheckResourceAttrPair(
						"gravitino_metalake.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_metalake.this", "audit.create_time"),
				),
			},
			{
				Config: cfgUpdate,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_metalake.this", "comment", "live acceptance metalake (updated)"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.%", "1"),
					resource.TestCheckResourceAttr("gravitino_metalake.this", "properties.env", "prod"),
				),
			},
			{
				ResourceName:      "gravitino_metalake.this",
				ImportState:       true,
				ImportStateId:     mlName,
				ImportStateVerify: true,
			},
		},
	})
}
```

- [ ] **Step 2: Run the live test against the real server**

Run: `make testacc-live-filter F=TestLiveAccMetalakeResource`
Expected: the test runs against the podman Gravitino; on a fresh image with auth=none the audit creator equals principal `"anonymous"`, so all steps PASS. If it fails on `properties.%`/`audit` assertions, capture the plan/state diff from the output and adjust only the assertion to match the verified real-server response documented in the spec.

- [ ] **Step 3: Verify it skips when no live server is configured**

Run: `TF_ACC=1 go test ./internal/resources/metalake/ -run TestLiveAccMetalakeResource -v`
Expected: `--- SKIP: TestLiveAccMetalakeResource (acceptance_test.go ... GRAVITINO_URI is not set ...)` and `PASS` (no server contact attempted). Do NOT set `GRAVITINO_URI` here.

- [ ] **Step 4: Commit**

```bash
git add internal/resources/metalake/live_test.go
git commit -m "test(metalake): add live acceptance test against real Gravitino"
```

---
### Task 5: Live catalog acceptance test

**Files:**
- Create: `internal/resources/catalog/live_test.go` (package `catalog_test`)

**Interfaces:**
- Consumes: Task 2 helpers.
- Produces: `TestLiveAccCatalogResource`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/catalog/live_test.go`:

```go
package catalog_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccCatalogResource(t *testing.T) {
	mlName := acceptance.UniqueName("catml")
	catName := acceptance.UniqueName("hive")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live catalog metalake"
}

resource "gravitino_catalog" "this" {
  metalake         = gravitino_metalake.this.name
  name             = %[2]q
  type             = "relational"
  catalog_provider = "hive"
  comment          = "live hive catalog"
  properties = {
    "metastore.uris" = "thrift://live-dummy:9083"
  }
}

data "gravitino_principal" "me" {}
`, mlName, catName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_catalog.this", "name", catName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "type", "relational"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "catalog_provider", "hive"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "comment", "live hive catalog"),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "id", mlName+"."+catName),
					resource.TestCheckResourceAttr("gravitino_catalog.this", "properties.metastore.uris", "thrift://live-dummy:9083"),
					resource.TestCheckResourceAttrPair(
						"gravitino_catalog.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_catalog.this", "audit.create_time"),
				),
			},
		},
	})
}
```

- [ ] **Step 2: Run the live test against the real server**

Run: `make testacc-live-filter F=TestLiveAccCatalogResource`
Expected: PASS (create with a dummy `metastore.uris` succeeds on the real server; teardown drops the catalog with `force=true`). If teardown fails with `CatalogInUseException`, that means `force=true` is not being applied — do not change provider code in this task; stop and report.

- [ ] **Step 3: Commit**

```bash
git add internal/resources/catalog/live_test.go
git commit -m "test(catalog): add live acceptance test against real Gravitino"
```

---
### Task 6: Live tag acceptance test

**Files:**
- Create: `internal/resources/tag/live_test.go` (package `tag_test`)

**Interfaces:**
- Consumes: Task 2 helpers.
- Produces: `TestLiveAccTagResource`.

- [ ] **Step 1: Write the failing test**

Create `internal/resources/tag/live_test.go`:

```go
package tag_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccTagResource(t *testing.T) {
	mlName := acceptance.UniqueName("tagml")
	tagName := acceptance.UniqueName("tag")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live tag metalake"
}

resource "gravitino_tag" "this" {
  metalake  = gravitino_metalake.this.name
  name      = %[2]q
  comment   = "live acceptance tag"
  properties = {}
}

data "gravitino_principal" "me" {}
`, mlName, tagName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_tag.this", "name", tagName),
					resource.TestCheckResourceAttr("gravitino_tag.this", "comment", "live acceptance tag"),
					resource.TestCheckResourceAttrPair(
						"gravitino_tag.this", "audit.creator",
						"data.gravitino_principal.me", "name"),
					resource.TestCheckResourceAttrSet("gravitino_tag.this", "audit.create_time"),
				),
			},
		},
	})
}
```

- [ ] **Step 2: Run the live test against the real server**

Run: `make testacc-live-filter F=TestLiveAccTagResource`
Expected: PASS. If `properties = {}` causes a create error, remove the `properties = {}` line and retry (tag properties are optional).

- [ ] **Step 3: Commit**

```bash
git add internal/resources/tag/live_test.go
git commit -m "test(tag): add live acceptance test against real Gravitino"
```

---
### Task 7: Live datasource acceptance tests

**Files:**
- Create: `internal/datasources/health/live_test.go` (package `health_test`)
- Create: `internal/datasources/authentication/live_test.go` (package `authentication_test`)
- Create: `internal/datasources/metalake/live_test.go` (package `metalake_test`)

**Interfaces:**
- Consumes: Task 2 helpers. Data source types verified in the codebase: `gravitino_health`, `gravitino_liveness`, `gravitino_readiness` (status attr), `gravitino_principal` (name attr), `gravitino_metalake` (get, requires `name`), `gravitino_metalakes` (list).
- Produces: `TestLiveAccHealthDataSources`, `TestLiveAccPrincipalDataSource`, `TestLiveAccMetalakeDataSources`.

- [ ] **Step 1: Write the health datasource live test**

Create `internal/datasources/health/live_test.go`:

```go
package health_test

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccHealthDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_health" "h" {}

data "gravitino_liveness" "l" {}

data "gravitino_readiness" "r" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_health.h", "status", "up"),
					resource.TestCheckResourceAttr("data.gravitino_liveness.l", "status", "up"),
					resource.TestCheckResourceAttr("data.gravitino_readiness.r", "status", "up"),
				),
			},
		},
	})
}
```

- [ ] **Step 2: Run the health datasource live test**

Run: `make testacc-live-filter F=TestLiveAccHealthDataSources`
Expected: PASS (server reports lowercase `"up"`; a mock that returns anything else would fail here).

- [ ] **Step 3: Write the principal datasource live test**

Create `internal/datasources/authentication/live_test.go`:

```go
package authentication_test

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccPrincipalDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
data "gravitino_principal" "me" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_principal.me", "name", "anonymous"),
				),
			},
		},
	})
}
```

- [ ] **Step 4: Run the principal datasource live test**

Run: `make testacc-live-filter F=TestLiveAccPrincipalDataSource`
Expected: PASS (default Gravitino server principal is `"anonymous"`).

- [ ] **Step 5: Write the metalake datasource live test**

Create `internal/datasources/metalake/live_test.go`:

```go
package metalake_test

import (
	"fmt"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/acceptance"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestLiveAccMetalakeDataSources(t *testing.T) {
	mlName := acceptance.UniqueName("dsml")

	cfg := fmt.Sprintf(`
resource "gravitino_metalake" "this" {
  name    = %[1]q
  comment = "live datasource metalake"
}

data "gravitino_metalake" "by_name" {
  name = gravitino_metalake.this.name
}

data "gravitino_metalakes" "all" {}
`, mlName)

	resource.Test(t, resource.TestCase{
		PreCheck:                 acceptance.LivePreCheck(t),
		ProtoV6ProviderFactories: acceptance.ProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.gravitino_metalake.by_name", "name", mlName),
					resource.TestCheckResourceAttr("data.gravitino_metalake.by_name", "comment", "live datasource metalake"),
				),
			},
		},
	})
}
```

- [ ] **Step 6: Run the metalake datasource live test**

Run: `make testacc-live-filter F=TestLiveAccMetalakeDataSources`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/datasources/health/live_test.go internal/datasources/authentication/live_test.go internal/datasources/metalake/live_test.go
git commit -m "test(datasources): add live acceptance tests for health, principal, metalake"
```

---
### Task 8: Docs + final full verification

**Files:**
- Modify: `README.md` (Testing section)
- Modify: `AGENTS.md` (Testing section)
- Modify: `CHANGELOG.md` (add entry under Unreleased/next)

**Interfaces:**
- Consumes: everything above. Produces: reproducible documentation of the live-test workflow.

- [ ] **Step 1: Document the live-test workflow in README.md**

Find the Testing/acceptance section in `README.md` and add below the existing docker acceptance instructions:

```markdown
### Live acceptance tests against a real Gravitino (podman)

The repository also contains `TestLiveAcc*` acceptance tests that run against a **real**
Gravitino server (image `apache/gravitino:1.3.0`) and prove the responses are not mocks:
`acceptance.LivePreCheck` refuses to run unless `GRAVITINO_URI` answers `GET /api/version`
with a matching Gravitino version.

Run them with podman (needs a running `podman machine` and `podman-compose`):

```bash
make testacc-live                # all live acceptance tests
make testacc-live-filter F=TestLiveAccMetalakeResource  # one test
```

Unlike the mock-based `TestAcc*` tests (which spin up an `httptest` server), these tests
target `http://gravitino:8090` inside the podman-compose network.
```

- [ ] **Step 2: Document the live-test workflow in AGENTS.md**

In the "Testing" section of `AGENTS.md`, under the existing acceptance test instructions, add:

```markdown
**Live acceptance tests (real server, not mocks):** `TestLiveAcc*` tests (see
`internal/resources/*/live_test.go`, `internal/datasources/*/live_test.go`) only run against a
real Gravitino via `make testacc-live` (podman) — the `acceptance.LivePreCheck` gate skips them
unless `GRAVITINO_URI` answers `/api/version`. They never start their own HTTP mock.
```

- [ ] **Step 3: Add a CHANGELOG entry**

Append under an `## [Unreleased]` / next-version section in `CHANGELOG.md`:

```markdown
### Added
- Live acceptance tests (`TestLiveAcc*`) that run against a real Gravitino server via podman (`make testacc-live`), including a `/api/version` precheck so tests never silently pass against a mock.
```

- [ ] **Step 4: Run the full live suite**

Run: `make testacc-live`
Expected: the six listed packages all PASS (including the existing mock `TestAcc*` tests in metalake/table only if present in the filtered run — the `-run TestLiveAcc` filter runs only the new live tests; every live test PASSES against the real server).

- [ ] **Step 5: Run the standard unit suite and lint**

Run: `go test ./internal/...`
Expected: all non-acceptance tests PASS and the `TestLiveAcc*`/`TestAcc*` acceptance tests report SKIP (no `TF_ACC`).

Run: `golangci-lint run --config .github/golangci.yml ./...`
Expected: no findings.

- [ ] **Step 6: Commit**

```bash
git add README.md AGENTS.md CHANGELOG.md
git commit -m "docs: document live Gravitino acceptance tests via podman"
```

---
## Self-Review

**Spec coverage:** The design's "not-a-mock" layers map to: (1) `LivePreCheck` requiring a live `/api/version` (Task 2), (2) explicit version assert vs `GRAVITINO_EXPECT_VERSION` (Task 2 + compose env in Task 3), (3) real-behaviour assertions: `audit.creator == principal` pair checks and lowercase `"up"` health (Tasks 4-7). podman orchestration (compose `acc` + poll script + make targets) is Task 3. Docs are Task 8. Out-of-scope items (user/group/role/owner, full hierarchy) are not implemented, per design.

**Placeholder scan:** No TBD/TODO. Every task contains full code, exact paths, and exact commands with expected output.

**Type/name consistency:** `GetVersion`/`VersionResponse`/`Version.Version` (Task 1) match `acceptance.LivePreCheck` (Task 2). `acceptance.LivePreCheck(t) func()`, `acceptance.ProtoV6ProviderFactories()`, `acceptance.UniqueName(prefix)` used identically in Tasks 4-7. Data source type names verified against the codebase (`gravitino_health`, `gravitino_liveness`, `gravitino_readiness`, `gravitino_principal`, `gravitino_metalake`, `gravitino_metalakes`). HCL attribute names verified against resource schemas (`catalog_provider`, `type` lowercase values, `metastore.uris`).
