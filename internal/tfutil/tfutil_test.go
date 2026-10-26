package tfutil

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestStringMap(t *testing.T) {
	ctx := context.Background()

	value := types.MapValueMust(types.StringType, map[string]attr.Value{
		"key": types.StringValue("value"),
	})

	var diags diag.Diagnostics
	got := StringMap(ctx, value, &diags)
	if diags.HasError() {
		t.Fatalf("StringMap diagnostics: %v", diags)
	}
	if len(got) != 1 || got["key"] != "value" {
		t.Fatalf("StringMap = %v, want {key: value}", got)
	}

	for _, nullish := range []types.Map{
		types.MapNull(types.StringType),
		types.MapUnknown(types.StringType),
	} {
		got := StringMap(ctx, nullish, &diags)
		if got == nil || len(got) != 0 {
			t.Fatalf("StringMap(null/unknown) = %v, want an empty non-nil map", got)
		}
	}
}

func TestSetToStrings(t *testing.T) {
	value := types.SetValueMust(types.StringType, []attr.Value{
		types.StringValue("a"),
		types.StringValue("b"),
	})

	got := SetToStrings(value)
	if len(got) != 2 {
		t.Fatalf("SetToStrings = %v, want two elements", got)
	}

	if got := SetToStrings(types.SetNull(types.StringType)); got != nil {
		t.Fatalf("SetToStrings(null) = %v, want nil", got)
	}
	if got := SetToStrings(types.SetUnknown(types.StringType)); got != nil {
		t.Fatalf("SetToStrings(unknown) = %v, want nil", got)
	}
}

func TestListToStrings(t *testing.T) {
	value := types.ListValueMust(types.StringType, []attr.Value{types.StringValue("a")})

	if got := ListToStrings(value); len(got) != 1 || got[0] != "a" {
		t.Fatalf("ListToStrings = %v, want [a]", got)
	}
	if got := ListToStrings(types.ListNull(types.StringType)); got != nil {
		t.Fatalf("ListToStrings(null) = %v, want nil", got)
	}
}

func TestStringsToSetAndList(t *testing.T) {
	set, diags := StringsToSet([]string{"a", "b"})
	if diags.HasError() {
		t.Fatalf("StringsToSet diagnostics: %v", diags)
	}
	if set.IsNull() || len(set.Elements()) != 2 {
		t.Fatalf("StringsToSet = %v, want two elements", set)
	}

	emptySet, diags := StringsToSet(nil)
	if diags.HasError() {
		t.Fatalf("StringsToSet(nil) diagnostics: %v", diags)
	}
	if emptySet.IsNull() || len(emptySet.Elements()) != 0 {
		t.Fatalf("StringsToSet(nil) = %v, want an empty set", emptySet)
	}

	list, diags := StringsToList([]string{"a"})
	if diags.HasError() {
		t.Fatalf("StringsToList diagnostics: %v", diags)
	}
	if list.IsNull() || len(list.Elements()) != 1 {
		t.Fatalf("StringsToList = %v, want one element", list)
	}
}

func TestFullyKnown(t *testing.T) {
	type block struct {
		Name  types.String `tfsdk:"name"`
		Value types.String `tfsdk:"value"`
	}

	known := block{Name: types.StringValue("a"), Value: types.StringNull()}
	if !FullyKnown(known) {
		t.Fatal("a null value is known and must not make a block unknown")
	}
	if !FullyKnown(known, "name", "value") {
		t.Fatal("skipping every field must make the block fully known")
	}

	unknown := block{Name: types.StringUnknown(), Value: types.StringValue("b")}
	if FullyKnown(unknown) {
		t.Fatal("an unknown field must make the block unknown")
	}
	if !FullyKnown(unknown, "name") {
		t.Fatal("skipping the unknown field must make the block fully known")
	}

	if FullyKnown([]block{known, unknown}) {
		t.Fatal("an unknown element must make the slice unknown")
	}
	if FullyKnown(&unknown) {
		t.Fatal("an unknown field behind a pointer must make the block unknown")
	}
	if !FullyKnown((*block)(nil)) {
		t.Fatal("a nil pointer carries no unknown value")
	}
	if !FullyKnown(map[string]block{"a": known}) {
		t.Fatal("a map of known blocks must be fully known")
	}
	if FullyKnown(map[string]block{"a": unknown}) {
		t.Fatal("an unknown map element must make the map unknown")
	}
}

func TestStringOrPrevious(t *testing.T) {
	previous := types.StringValue("previous")

	if got := StringOrPrevious(types.StringValue("planned"), previous); got.ValueString() != "planned" {
		t.Fatalf("StringOrPrevious(known) = %v, want the planned value", got)
	}
	if got := StringOrPrevious(types.StringUnknown(), previous); got.ValueString() != "previous" {
		t.Fatalf("StringOrPrevious(unknown) = %v, want the previous value", got)
	}
	if got := StringOrPrevious(types.StringUnknown(), types.StringNull()); !got.IsNull() {
		t.Fatalf("StringOrPrevious(unknown, null) = %v, want null", got)
	}
	if got := StringOrPrevious(types.StringUnknown(), types.StringUnknown()); !got.IsNull() {
		t.Fatalf("StringOrPrevious(unknown, unknown) = %v, want null", got)
	}
}

func TestMapOrPrevious(t *testing.T) {
	previous := types.MapValueMust(types.StringType, map[string]attr.Value{"k": types.StringValue("v")})
	planned := types.MapValueMust(types.StringType, map[string]attr.Value{"other": types.StringValue("x")})

	if got := MapOrPrevious(context.Background(), planned, previous); !got.Equal(planned) {
		t.Fatalf("MapOrPrevious(known) = %v, want the planned map", got)
	}
	if got := MapOrPrevious(context.Background(), types.MapUnknown(types.StringType), previous); !got.Equal(previous) {
		t.Fatalf("MapOrPrevious(unknown) = %v, want the previous map", got)
	}
	got := MapOrPrevious(context.Background(), types.MapUnknown(types.StringType), types.MapNull(types.StringType))
	if !got.IsNull() || got.ElementType(context.Background()) != types.StringType {
		t.Fatalf("MapOrPrevious(unknown, null) = %v, want a null map of strings", got)
	}
}

func TestObjectOrPrevious(t *testing.T) {
	ctx := context.Background()
	attrTypes := map[string]attr.Type{"name": types.StringType}
	previous := types.ObjectValueMust(attrTypes, map[string]attr.Value{"name": types.StringValue("previous")})
	planned := types.ObjectValueMust(attrTypes, map[string]attr.Value{"name": types.StringValue("planned")})

	if got := ObjectOrPrevious(ctx, planned, previous); !got.Equal(planned) {
		t.Fatalf("ObjectOrPrevious(known) = %v, want the planned object", got)
	}
	if got := ObjectOrPrevious(ctx, types.ObjectUnknown(attrTypes), previous); !got.Equal(previous) {
		t.Fatalf("ObjectOrPrevious(unknown) = %v, want the previous object", got)
	}
	got := ObjectOrPrevious(ctx, types.ObjectUnknown(attrTypes), types.ObjectNull(attrTypes))
	if !got.IsNull() || len(got.AttributeTypes(ctx)) != 1 {
		t.Fatalf("ObjectOrPrevious(unknown, null) = %v, want a null object with the planned type", got)
	}
}
