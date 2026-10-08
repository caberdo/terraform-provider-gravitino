// Package resourcetest holds test helpers shared by the resource and data
// source test packages, replacing the copies that were duplicated per package.
package resourcetest

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// schemaTyper is satisfied by both resource/schema.Schema and
// datasource/schema.Schema.
type schemaTyper interface {
	Type() attr.Type
}

// TFValue converts a resource or data source model into the tftypes.Value the
// framework passes to plan modifiers, using the schema to type the object.
func TFValue(t *testing.T, ctx context.Context, s schemaTyper, model any) tftypes.Value {
	t.Helper()

	obj, diags := types.ObjectValueFrom(ctx, s.Type().(types.ObjectType).AttributeTypes(), model)
	if diags.HasError() {
		t.Fatalf("failed to build object value: %v", diags)
	}
	v, err := obj.ToTerraformValue(ctx)
	if err != nil {
		t.Fatalf("failed to convert to terraform value: %v", err)
	}
	return v
}

// AssertJSONEqual fails the test when the two maps do not marshal to the same
// JSON, which compares them regardless of key order.
func AssertJSONEqual(t *testing.T, want, got map[string]any) {
	t.Helper()

	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Fatalf("unexpected payload:\n want %s\n  got %s", wantJSON, gotJSON)
	}
}

// AssertJSONEqualUnordered fails the test when the two JSON documents are not
// deeply equal, ignoring key order.
func AssertJSONEqualUnordered(t *testing.T, want string, got []byte) {
	t.Helper()

	var wantVal, gotVal interface{}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("invalid expected JSON: %v", err)
	}
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("request body is not valid JSON: %q (%v)", string(got), err)
	}
	if !reflect.DeepEqual(wantVal, gotVal) {
		t.Errorf("unexpected request body\n got: %s\nwant: %s", string(got), want)
	}
}
