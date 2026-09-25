package role_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func roleTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// roleFakeServer is a minimal, spec-faithful Gravitino role API. Responses mirror the
// RoleResponse/NameListResponse examples of roles.yaml, including the audit block the
// v1.3.0 server always returns.
type roleFakeServer struct {
	t                 *testing.T
	server            *httptest.Server
	mu                sync.Mutex
	roles             map[string]models.Role
	creates           int
	deletes           int
	createsBody       []models.RoleCreateRequest
	overrides         []models.PrivilegeOverrideRequest
	requests          []string
	reversePrivileges bool
}

func newRoleFakeServer(t *testing.T) *roleFakeServer {
	t.Helper()

	f := &roleFakeServer{t: t, roles: map[string]models.Role{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

// createdAudit mirrors the audit block of a freshly created role on a real Gravitino
// server: only creator and createTime are set, lastModifier/lastModifiedTime are absent.
func (f *roleFakeServer) createdAudit() *models.Audit {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	return &models.Audit{Creator: "anonymous", CreateTime: &now}
}

// modifiedAudit mirrors the audit block after a privilege override: the real server
// fills in lastModifier and lastModifiedTime.
func (f *roleFakeServer) modifiedAudit() *models.Audit {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	modified := time.Date(2026, 1, 2, 3, 5, 6, 0, time.UTC)
	return &models.Audit{Creator: "anonymous", CreateTime: &now, LastModifier: "anonymous", LastModifiedTime: &modified}
}

func (f *roleFakeServer) writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// wire mirrors the real Gravitino 1.3.0 serialisation: enums are lower case in every
// response (verified against a live server), while the input is case-insensitive.
func (f *roleFakeServer) wire(role models.Role) models.Role {
	objects := make([]models.SecurableObject, 0, len(role.SecurableObjects))
	for _, o := range role.SecurableObjects {
		privileges := make([]models.Privilege, 0, len(o.Privileges))
		for _, p := range o.Privileges {
			privileges = append(privileges, models.Privilege{
				Name:      strings.ToLower(p.Name),
				Condition: strings.ToLower(p.Condition),
			})
		}
		objects = append(objects, models.SecurableObject{
			FullName:   o.FullName,
			Type:       strings.ToLower(o.Type),
			Privileges: privileges,
		})
	}

	role.SecurableObjects = objects
	return role
}

func (f *roleFakeServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, r.Method+" "+r.URL.Path)

	roleName := func() string {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/metalakes/ml/roles/"), "/")
		return parts[0]
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/metalakes/ml/roles":
		var req models.RoleCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			f.t.Errorf("failed to decode create request: %v", err)
			f.writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Code: 1001, Type: "IllegalArgumentException", Message: err.Error()})
			return
		}
		f.creates++
		f.createsBody = append(f.createsBody, req)

		if req.SecurableObjects == nil {
			// The real server rejects a request without the array.
			f.writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Code: 1001, Type: "IllegalArgumentException", Message: `"securableObjects" can't null`})
			return
		}

		role := models.Role{
			Name:             req.Name,
			Properties:       req.Properties,
			SecurableObjects: req.SecurableObjects,
			Audit:            f.createdAudit(),
		}
		f.roles[req.Name] = role
		f.writeJSON(w, http.StatusOK, models.RoleResponse{Code: 0, Role: f.wire(role)})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/roles/"):
		role, ok := f.roles[roleName()]
		if !ok {
			f.writeJSON(w, http.StatusNotFound, models.ErrorResponse{
				Code:    1003,
				Type:    "NoSuchRoleException",
				Message: "Role does not exist",
				Stack:   []string{"org.apache.gravitino.exceptions.NoSuchRoleException: Role does not exist", "..."},
			})
			return
		}
		f.writeJSON(w, http.StatusOK, models.RoleResponse{Code: 0, Role: f.wire(role)})

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/permissions/roles/"):
		var req models.PrivilegeOverrideRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			f.t.Errorf("failed to decode override request: %v", err)
			f.writeJSON(w, http.StatusBadRequest, models.ErrorResponse{Code: 1001, Type: "IllegalArgumentException", Message: err.Error()})
			return
		}
		f.overrides = append(f.overrides, req)

		name := strings.TrimPrefix(r.URL.Path, "/api/metalakes/ml/permissions/roles/")
		role := f.roles[name]
		role.SecurableObjects = req.Overrides
		role.Audit = f.modifiedAudit()
		f.roles[name] = role
		f.writeJSON(w, http.StatusOK, models.RoleResponse{Code: 0, Role: f.wire(role)})

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/metalakes/ml/roles/"):
		f.deletes++
		delete(f.roles, roleName())
		f.writeJSON(w, http.StatusOK, models.DropResponse{Code: 0, Dropped: true})

	default:
		http.NotFound(w, r)
	}
}

// checkStoredObjects fails when the role the fake server holds does not carry exactly the
// given securable objects. It runs inside a test step, while the role still exists.
func (f *roleFakeServer) checkStoredObjects(name string, want []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		role, ok := f.storedRole(name)
		if !ok {
			return fmt.Errorf("role %q is not stored by the server", name)
		}

		got := make([]string, 0, len(role.SecurableObjects))
		for _, o := range role.SecurableObjects {
			got = append(got, o.FullName)
		}
		if len(got) != len(want) {
			return fmt.Errorf("expected the role to hold %v, got %v", want, got)
		}
		for i := range want {
			if got[i] != want[i] {
				return fmt.Errorf("expected the role to hold %v, got %v", want, got)
			}
		}
		return nil
	}
}

func (f *roleFakeServer) counts() (creates, deletes, overrides int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.creates, f.deletes, len(f.overrides)
}

// overrideBodies returns the recorded privilege override payloads.
func (f *roleFakeServer) overrideBodies() []models.PrivilegeOverrideRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.PrivilegeOverrideRequest{}, f.overrides...)
}

// storedRole returns the role as the fake server currently holds it.
func (f *roleFakeServer) storedRole(name string) (models.Role, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.roles[name]
	return r, ok
}

// requestsLog returns the requests in the order in which the server received them.
func (f *roleFakeServer) requestsLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.requests...)
}

func (f *roleFakeServer) createdBodies() []models.RoleCreateRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.RoleCreateRequest{}, f.createsBody...)
}

// TestAccRoleResource_Lifecycle applies the RoleCreateRequest/RoleResponse examples of
// the v1.3.0 spec and verifies import round-trips.
func TestAccRoleResource_Lifecycle(t *testing.T) {
	fake := newRoleFakeServer(t)
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  properties = {
    k1 = "v1"
  }
  securable_objects = [{
    full_name = "catalog1.schema1.table1"
    type      = "TABLE"
    privileges = [{
      name      = "SELECT_TABLE"
      condition = "ALLOW"
    }]
  }]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "id", "ml.role1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "properties.k1", "v1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.full_name", "catalog1.schema1.table1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.type", "TABLE"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.0.name", "SELECT_TABLE"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.0.condition", "ALLOW"),
					// The fake server reports the anonymous principal, like a real
					// server without authentication does.
					resource.TestCheckResourceAttr("gravitino_role.role1", "audit.creator", "anonymous"),
				),
			},
			{
				ResourceName:      "gravitino_role.role1",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})

	creates, _, _ := fake.counts()
	if creates != 1 {
		t.Fatalf("expected 1 create, got %d", creates)
	}

	bodies := fake.createdBodies()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 recorded create body, got %d", len(bodies))
	}
	if !jsonEqual(t, specRoleCreateRequest, mustJSON(t, bodies[0])) {
		t.Fatalf("create body does not match the spec example: %s", mustJSON(t, bodies[0]))
	}
}

// TestAccRoleResource_PluralObjectTypeRejected proves that only the MetadataObject.Type
// values of the spec are accepted: a real Gravitino 1.3.0 server answers "catalogs" with
// HTTP 400 and "No enum constant org.apache.gravitino.MetadataObject.Type.CATALOGS".
func TestAccRoleResource_PluralObjectTypeRejected(t *testing.T) {
	fake := newRoleFakeServer(t)
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  securable_objects = [{
    full_name = "catalog1"
    type      = "catalogs"
    privileges = [{
      name      = "USE_CATALOG"
      condition = "ALLOW"
    }]
  }]
}
`,
				ExpectError: regexp.MustCompile(`(?s)must be one of`),
			},
		},
	})
}

// TestAccRoleResource_LowercaseServerValues proves the provider normalises the lower-case
// enums the Gravitino API returns back to the canonical upper-case spelling, so an apply
// does not fail with "Provider produced inconsistent result after apply". Verified against
// a live Gravitino 1.3.0: POST/GET/PUT all answer with "type":"metalake",
// "name":"create_catalog", "condition":"allow".
func TestAccRoleResource_LowercaseServerValues(t *testing.T) {
	fake := newRoleFakeServer(t)
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  securable_objects = [{
    full_name = "ml"
    type      = "METALAKE"
    privileges = [{
      name      = "CREATE_CATALOG"
      condition = "ALLOW"
    }]
  }]
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.type", "METALAKE"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.0.name", "CREATE_CATALOG"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.0.condition", "ALLOW"),
				),
			},
			{
				// The lower-case values of the server must not produce a diff.
				Config: `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  securable_objects = [{
    full_name = "ml"
    type      = "METALAKE"
    privileges = [{
      name      = "CREATE_CATALOG"
      condition = "ALLOW"
    }]
  }]
}
`,
				PlanOnly: true,
			},
		},
	})
}

// TestAccRoleResource_CreateWithoutSecurableObjects proves that a role without securable
// objects sends the empty array the API requires.
func TestAccRoleResource_CreateWithoutSecurableObjects(t *testing.T) {
	fake := newRoleFakeServer(t)
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "0"),
				),
			},
		},
	})

	bodies := fake.createdBodies()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 create body, got %d", len(bodies))
	}
	if bodies[0].SecurableObjects == nil {
		t.Fatal("the create request must contain an empty securableObjects array")
	}
}

// TestAccRoleResource_UpdatePrivileges pushes privilege changes through the override
// endpoint instead of recreating the role, including the removal of a securable object:
// PUT /permissions/roles/{role} replaces the complete set (verified against a live
// Gravitino 1.3.0, where an object omitted from `overrides` disappears from the role).
func TestAccRoleResource_UpdatePrivileges(t *testing.T) {
	fake := newRoleFakeServer(t)
	fake.reversePrivileges = true
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	config := func(objects string) string {
		return `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  securable_objects = [` + objects + `]
}
`
	}

	table := func(privileges string) string {
		return `
    {
      full_name = "catalog1.schema1.table1"
      type      = "TABLE"
      privileges = [` + privileges + `]
    }`
	}

	selectTable := `
        {
          name      = "SELECT_TABLE"
          condition = "ALLOW"
        }`
	modifyTable := `
        {
          name      = "MODIFY_TABLE"
          condition = "ALLOW"
        }`
	catalog := `
    {
      full_name = "catalog1"
      type      = "CATALOG"
      privileges = [{
        name      = "USE_CATALOG"
        condition = "ALLOW"
      }]
    }`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(table(selectTable)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.#", "1"),
				),
			},
			{
				// Add a privilege to the single existing object.
				Config: config(table(selectTable + "," + modifyTable)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.#", "2"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "audit.last_modifier", "anonymous"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "audit.last_modified_time", "2026-01-02T03:05:06Z"),
				),
			},
			{
				// Add a second securable object.
				Config: config(table(selectTable+","+modifyTable) + "," + catalog),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "2"),
				),
			},
			{
				// Remove the second securable object again: the override carries the
				// complete set, so the catalog object disappears from the role.
				Config: config(table(selectTable + "," + modifyTable)),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.#", "1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.full_name", "catalog1.schema1.table1"),
					resource.TestCheckResourceAttr("gravitino_role.role1", "securable_objects.0.privileges.#", "2"),
					fake.checkStoredObjects("role1", []string{"catalog1.schema1.table1"}),
				),
			},
		},
	})

	creates, deletes, overrides := fake.counts()
	if creates != 1 {
		t.Fatalf("expected 1 create (no recreation), got %d; requests: %v", creates, fake.requestsLog())
	}
	if deletes != 1 {
		// Only the final destroy of the test must delete the role.
		t.Fatalf("expected only the final destroy to delete the role, got %d deletes; requests: %v", deletes, fake.requestsLog())
	}
	if overrides != 3 {
		t.Fatalf("expected 3 privilege overrides, got %d; requests: %v", overrides, fake.requestsLog())
	}

	bodies := fake.overrideBodies()
	if len(bodies) != 3 {
		t.Fatalf("expected 3 recorded override payloads, got %d", len(bodies))
	}
	// The last override must carry the complete desired set, so the removed catalog
	// object is not part of the role anymore.
	last := bodies[len(bodies)-1]
	if len(last.Overrides) != 1 {
		t.Fatalf("expected the last override to contain the single remaining object, got %d", len(last.Overrides))
	}
	if last.Overrides[0].FullName != "catalog1.schema1.table1" {
		t.Fatalf("unexpected object in the last override: %q", last.Overrides[0].FullName)
	}

}

// TestAccRoleResource_PropertiesForceRecreate proves that changing `properties`
// destroys and recreates the role instead of silently doing nothing: Gravitino v1.3.0
// only exposes PUT /metalakes/{metalake}/permissions/roles/{role} (privilege override)
// for roles.
func TestAccRoleResource_PropertiesForceRecreate(t *testing.T) {
	fake := newRoleFakeServer(t)
	t.Setenv("GRAVITINO_URI", fake.server.URL)

	config := func(managedBy string) string {
		return `
resource "gravitino_role" "role1" {
  metalake = "ml"
  name     = "role1"
  properties = {
    managed_by = "` + managedBy + `"
  }
  securable_objects = [{
    full_name = "catalog1.schema1.table1"
    type      = "TABLE"
    privileges = [{
      name      = "SELECT_TABLE"
      condition = "ALLOW"
    }]
  }]
}
`
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: roleTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config("security-team"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "properties.managed_by", "security-team"),
				),
			},
			{
				Config: config("platform-team"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_role.role1", "properties.managed_by", "platform-team"),
				),
			},
		},
	})

	creates, deletes, overrides := fake.counts()
	// One create for step 1, one for the replacement in step 2.
	if creates != 2 {
		t.Fatalf("expected the role to be recreated (2 creates), got %d creates", creates)
	}
	// One delete for the replacement in step 2, one for the final destroy.
	if deletes != 2 {
		t.Fatalf("expected the role to be destroyed and recreated (2 deletes), got %d deletes", deletes)
	}
	if overrides != 0 {
		t.Fatalf("properties must not be pushed through the privilege override endpoint, got %d overrides", overrides)
	}
}
