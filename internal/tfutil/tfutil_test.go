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
