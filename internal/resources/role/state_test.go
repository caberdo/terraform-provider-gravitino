package role

import (
	"context"
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

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
