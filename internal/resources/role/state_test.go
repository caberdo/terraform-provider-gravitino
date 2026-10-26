package role

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"
	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// TestRoleResourceModelMatchesSchema keeps the Terraform model, the nested
// securable object tree and the audit object in lockstep with the schema.
func TestRoleResourceModelMatchesSchema(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewRoleResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema diagnostics: %v", resp.Diagnostics)
	}
	modeltest.AssertSchemaMatches(t, RoleResourceModel{}, resp.Schema.Attributes, resp.Schema.Blocks)

	securableObjects, ok := resp.Schema.Attributes["securable_objects"].(schema.SetNestedAttribute)
	if !ok {
		t.Fatalf("securable_objects attribute = %T, want schema.SetNestedAttribute", resp.Schema.Attributes["securable_objects"])
	}
	securableObjectAttributes := securableObjects.NestedObject.Attributes
	modeltest.AssertAttributesMatch(t, securableObjectModel{}, modeltest.Keys(securableObjectAttributes))

	privileges, ok := securableObjectAttributes["privileges"].(schema.SetNestedAttribute)
	if !ok {
		t.Fatalf("privileges attribute = %T, want schema.SetNestedAttribute", securableObjectAttributes["privileges"])
	}
	modeltest.AssertAttributesMatch(t, privilegeModel{}, modeltest.Keys(privileges.NestedObject.Attributes))

	audit, ok := resp.Schema.Attributes["audit"].(schema.ObjectAttribute)
	if !ok {
		t.Fatalf("audit attribute = %T, want schema.ObjectAttribute", resp.Schema.Attributes["audit"])
	}
	modeltest.AssertAttributesMatch(t, models.AuditTFSDK{}, modeltest.Keys(audit.AttributeTypes))
}

func TestSecurableObjectsToTF_NormalizesToUppercase(t *testing.T) {
	objects := []models.SecurableObject{
		{
			FullName: "acme",
			Type:     "metalake",
			Privileges: []models.Privilege{
				{Name: "create_catalog", Condition: "allow"},
				{Name: "use_catalog", Condition: "deny"},
			},
		},
	}

	list, diags := securableObjectsToTF(context.Background(), objects)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	elements := list.Elements()
	if len(elements) != 1 {
		t.Fatalf("expected 1 securable object, got %d", len(elements))
	}
	so, ok := elements[0].(types.Object)
	if !ok {
		t.Fatalf("expected types.Object, got %T", elements[0])
	}
	model := securableObjectModel{}
	if d := so.As(context.Background(), &model, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("failed to read securable object: %v", d)
	}
	if model.Type.ValueString() != "METALAKE" {
		t.Fatalf("expected type METALAKE, got %q", model.Type.ValueString())
	}

	privs := model.Privileges.Elements()
	if len(privs) != 2 {
		t.Fatalf("expected 2 privileges, got %d", len(privs))
	}
	priv1 := privilegeModel{}
	if d := privs[0].(types.Object).As(context.Background(), &priv1, basetypes.ObjectAsOptions{}); d.HasError() {
		t.Fatalf("failed to read privilege: %v", d)
	}
	if priv1.Name.ValueString() != "CREATE_CATALOG" {
		t.Fatalf("expected privilege name CREATE_CATALOG, got %q", priv1.Name.ValueString())
	}
	if priv1.Condition.ValueString() != "ALLOW" {
		t.Fatalf("expected condition ALLOW, got %q", priv1.Condition.ValueString())
	}
}
