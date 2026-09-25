package job_template_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/provider"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func jobTemplateTestAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"gravitino": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// jobTemplateMock is a stateful stand-in for the Gravitino jobs API. Its
// behaviour was verified against a real Gravitino 1.3.0 server:
//
//   - POST /api/metalakes/ml/jobs/templates stores the decoded jobTemplate and
//     answers {"code": 0}
//   - GET .../jobs/templates/{name} returns the stored template with audit
//   - PUT .../jobs/templates/{name} applies the updates array and returns the
//     resulting template
//   - DELETE .../jobs/templates/{name} answers {"code": 0, "dropped": bool}
//   - a missing template yields HTTP 404 with a NoSuchJobTemplateException body
type jobTemplateMock struct {
	mu        sync.Mutex
	templates map[string]models.JobTemplate
	requests  []string
}

const mockTemplatePrefix = "/api/metalakes/ml/jobs/templates"

func newJobTemplateMock() *jobTemplateMock {
	return &jobTemplateMock{templates: map[string]models.JobTemplate{}}
}

func (m *jobTemplateMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.gravitino.v1+json")

	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, r.Method+" "+r.URL.Path)

	name := strings.TrimPrefix(r.URL.Path, mockTemplatePrefix+"/")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == mockTemplatePrefix:
		var req models.JobTemplateRegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		template := req.JobTemplate
		template.Audit = mockAudit(nil)
		m.templates[template.Name] = template
		m.writeJSON(w, models.BaseResponse{Code: 0})

	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, mockTemplatePrefix+"/"):
		template, ok := m.templates[name]
		if !ok {
			m.writeNotFound(w, name)
			return
		}
		m.writeJSON(w, models.JobTemplateResponse{Code: 0, JobTemplate: template})

	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, mockTemplatePrefix+"/"):
		template, ok := m.templates[name]
		if !ok {
			m.writeNotFound(w, name)
			return
		}
		var req models.JobTemplateUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated := applyTemplateUpdates(template, req.Updates)
		delete(m.templates, name)
		// A real Gravitino stamps lastModifier/lastModifiedTime on update.
		updated.Audit = mockAudit(&mockModifiedTime)
		m.templates[updated.Name] = updated
		m.writeJSON(w, models.JobTemplateResponse{Code: 0, JobTemplate: updated})

	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, mockTemplatePrefix+"/"):
		_, ok := m.templates[name]
		delete(m.templates, name)
		m.writeJSON(w, models.DropResponse{Code: 0, Dropped: ok})

	default:
		http.NotFound(w, r)
	}
}

func (m *jobTemplateMock) writeJSON(w http.ResponseWriter, body interface{}) {
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (m *jobTemplateMock) writeNotFound(w http.ResponseWriter, name string) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = fmt.Fprintf(w, `{"code":1003,"type":"NoSuchJobTemplateException","message":"Failed to operate job template(s) [%s] operation [GET] under object [ml], reason [Job template with name %s under metalake ml does not exist]","stack":["org.apache.gravitino.exceptions.NoSuchJobTemplateException: Job template with name %s under metalake ml does not exist"]}`, name, name, name)
}

// applyTemplateUpdates replays an updates array the way the server does, using
// the same DTO the provider sends.
func applyTemplateUpdates(template models.JobTemplate, updates []interface{}) models.JobTemplate {
	for _, raw := range updates {
		update, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch fmt.Sprint(update["@type"]) {
		case "rename":
			template.Name = fmt.Sprint(update["newName"])
		case "updateComment":
			template.Comment = fmt.Sprint(update["newComment"])
		case "updateTemplate":
			newTemplate, ok := update["newTemplate"].(map[string]interface{})
			if !ok {
				continue
			}
			patch, err := json.Marshal(newTemplate)
			if err != nil {
				continue
			}
			var content models.JobTemplateContentUpdate
			if err := json.Unmarshal(patch, &content); err != nil {
				continue
			}
			if content.NewExecutable != nil {
				template.Executable = *content.NewExecutable
			}
			if content.NewArguments != nil {
				template.Arguments = *content.NewArguments
			}
			if content.NewEnvironments != nil {
				template.Environments = *content.NewEnvironments
			}
			if content.NewCustomFields != nil {
				template.CustomFields = *content.NewCustomFields
			}
			if content.NewScripts != nil {
				template.Scripts = *content.NewScripts
			}
			if content.NewClassName != nil {
				template.ClassName = *content.NewClassName
			}
			if content.NewJars != nil {
				template.Jars = *content.NewJars
			}
			if content.NewFiles != nil {
				template.Files = *content.NewFiles
			}
			if content.NewArchives != nil {
				template.Archives = *content.NewArchives
			}
			if content.NewConfigs != nil {
				template.Configs = *content.NewConfigs
			}
		}
	}
	return template
}

// mockModifiedTime is the timestamp the mock stamps on an update, mirroring a
// real Gravitino response.
var mockModifiedTime = time.Date(2025, 8, 13, 9, 0, 0, 0, time.UTC)

func mockAudit(lastModified *time.Time) *models.Audit {
	created := time.Date(2025, 8, 12, 2, 14, 28, 205023000, time.UTC)
	audit := &models.Audit{
		Creator:    "anonymous",
		CreateTime: &created,
	}
	if lastModified != nil {
		modified := *lastModified
		audit.LastModifier = "anonymous"
		audit.LastModifiedTime = &modified
	}
	return audit
}

// TestAccJobTemplateResource_Create runs a create through Terraform Core against
// the mock. Terraform Core rejects the apply with "Provider produced inconsistent
// result after apply" / "Provider returned invalid result object after apply" if
// the provider does not reproduce every planned attribute exactly, which is the
// regression this family is prone to.
func TestAccJobTemplateResource_Create(t *testing.T) {
	server := httptest.NewServer(newJobTemplateMock())
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: jobTemplateTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "gravitino_job_template" "this" {
  metalake     = "ml"
  name         = "test_run_get"
  job_type     = "shell"
  comment      = "Test shell job template"
  executable   = %q
  arguments    = ["{{arg1}}", "{{arg2}}"]
  environments = { ENV_VAR = "{{env_var}}" }
  scripts      = [%q]
}
`, shellExecutable, shellCommonScript),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "id", "ml.test_run_get"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "name", "test_run_get"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "job_type", "shell"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "comment", "Test shell job template"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "executable", shellExecutable),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.#", "2"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.0", "{{arg1}}"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "environments.ENV_VAR", "{{env_var}}"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "scripts.#", "1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "audit.creator", "anonymous"),
					resource.TestCheckNoResourceAttr("gravitino_job_template.this", "custom_fields"),
				),
			},
		},
	})
}

// TestAccJobTemplateResource_Update runs a rename plus a comment and content
// update through Terraform Core. It uses a spark template because a shell
// template cannot even be created: the resource reports class_name as "" where
// the plan says null (see TestAccJobTemplateResource_Create).
//
// The rename changes the composite id, and the server stamps
// lastModifier/lastModifiedTime on the audit object, so this guards both the
// `id` and `audit` plan modifiers.
func TestAccJobTemplateResource_Update(t *testing.T) {
	server := httptest.NewServer(newJobTemplateMock())
	defer server.Close()
	t.Setenv("GRAVITINO_URI", server.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: jobTemplateTestAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "gravitino_job_template" "this" {
  metalake   = "ml"
  name       = "test_run_get_spark"
  job_type   = "spark"
  comment    = "Test spark job template"
  executable = %q
  class_name = "org.apache.spark.examples.SparkPi"
  jars       = [%q]
}
`, sparkExecutable, sparkJar),
			},
			{
				Config: fmt.Sprintf(`
resource "gravitino_job_template" "this" {
  metalake      = "ml"
  name          = "new_test_run_get_spark"
  job_type      = "spark"
  comment       = "Updated comment for the job template"
  executable    = %q
  arguments     = ["--arg1"]
  environments  = { NEW_ENV_VAR = "{{new_env_var}}" }
  custom_fields = { field1 = "value1" }
  class_name    = "org.apache.spark.examples.SparkPi"
  jars          = [%q]
}
`, sparkExecutable, sparkJar),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("gravitino_job_template.this", "id", "ml.new_test_run_get_spark"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "name", "new_test_run_get_spark"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "comment", "Updated comment for the job template"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.#", "1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "arguments.0", "--arg1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "custom_fields.field1", "value1"),
					resource.TestCheckResourceAttr("gravitino_job_template.this", "jars.0", sparkJar),
				),
			},
		},
	})
}
