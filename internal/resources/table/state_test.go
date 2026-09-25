package table

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestMergeTableProperties covers the property rules of the resource: the
// configuration owns the keys it manages, and the keys a catalog adds on its
// own (Hive reports location and table-type) only surface when the
// configuration manages no properties at all.
func TestMergeTableProperties_ConfiguredKeysAreNotPolluted(t *testing.T) {
	desired := mustMap(map[string]string{"format": "ORC"})

	merged, diags := mergeTableProperties(context.Background(), map[string]string{
		"format":     "ORC",
		"location":   "hdfs://namenode/user/hive/warehouse/t",
		"table-type": "MANAGED_TABLE",
	}, desired, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	properties := make(map[string]string)
	if d := merged.ElementsAs(context.Background(), &properties, false); d.HasError() {
		t.Fatalf("reading properties: %v", d)
	}
	if len(properties) != 1 || properties["format"] != "ORC" {
		t.Fatalf("expected only the configured property, got %#v", properties)
	}
}

func TestMergeTableProperties_ServerWinsOnRefresh(t *testing.T) {
	desired := mustMap(map[string]string{"format": "ORC", "region": "eu"})

	merged, diags := mergeTableProperties(context.Background(), map[string]string{
		"format": "PARQUET",
	}, desired, true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	properties := make(map[string]string)
	if d := merged.ElementsAs(context.Background(), &properties, false); d.HasError() {
		t.Fatalf("reading properties: %v", d)
	}
	if properties["format"] != "PARQUET" {
		t.Errorf("format = %q, want the value reported by the server", properties["format"])
	}
	if properties["region"] != "eu" {
		t.Errorf("region = %q, want the configured value", properties["region"])
	}
}

func TestMergeTableProperties_UnmanagedSurfacesServerProperties(t *testing.T) {
	merged, diags := mergeTableProperties(context.Background(), map[string]string{
		"env":    "dev",
		"in-use": "true",
	}, types.MapNull(types.StringType), false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	properties := make(map[string]string)
	if d := merged.ElementsAs(context.Background(), &properties, false); d.HasError() {
		t.Fatalf("reading properties: %v", d)
	}
	if properties["env"] != "dev" {
		t.Errorf("env = %q, want dev", properties["env"])
	}
	if _, ok := properties["in-use"]; ok {
		t.Errorf("the reserved in-use property must be filtered, got %#v", properties)
	}
}

func TestMergeTableProperties_EmptyConfigurationStaysEmpty(t *testing.T) {
	desired, diags := types.MapValueFrom(context.Background(), types.StringType, map[string]string{})
	if diags.HasError() {
		t.Fatalf("building empty map: %v", diags)
	}

	merged, mergeDiags := mergeTableProperties(context.Background(), map[string]string{"env": "dev"}, desired, true)
	if mergeDiags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", mergeDiags)
	}
	if merged.IsNull() {
		t.Fatal("an empty properties map configured by the user must stay empty, not become null")
	}
	if len(merged.Elements()) != 0 {
		t.Errorf("expected an empty map, got %v", merged.Elements())
	}
}

func TestMergeTableProperties_NoServerPropertiesAndNoConfiguration(t *testing.T) {
	merged, diags := mergeTableProperties(context.Background(), nil, types.MapNull(types.StringType), true)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !merged.IsNull() {
		t.Errorf("expected null, got %v", merged)
	}
}
