package job_template

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// optionalStringToTF renders an absent server value as null, so an optional attribute
// that the API omits does not become an empty string in state.
func optionalStringToTF(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

// timeFormat matches the timestamp format used by the other resources.
const timeFormat = "2006-01-02T15:04:05Z07:00"

func mapFromTF(m types.Map) map[string]string {
	result := make(map[string]string)
	if m.IsNull() || m.IsUnknown() {
		return result
	}
	for k, v := range m.Elements() {
		if strVal, ok := v.(types.String); ok {
			result[k] = strVal.ValueString()
		}
	}
	return result
}

// mapToTF renders nil and empty maps as null so an omitted attribute in the
// configuration does not become an empty map in state (perpetual diff).
func mapToTF(m map[string]string) types.Map {
	if len(m) == 0 {
		return types.MapNull(types.StringType)
	}
	attrs := make(map[string]attr.Value, len(m))
	for k, v := range m {
		attrs[k] = types.StringValue(v)
	}
	return types.MapValueMust(types.StringType, attrs)
}

func stringsFromTF(ctx context.Context, l types.List) []string {
	values, diags := stringListFromTF(ctx, l)
	if diags.HasError() || len(values) == 0 {
		return nil
	}
	return values
}

func stringListFromTF(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if l.IsNull() || l.IsUnknown() {
		return nil, diags
	}

	var values []string
	diags.Append(l.ElementsAs(ctx, &values, false)...)
	if diags.HasError() {
		return nil, diags
	}
	return values, diags
}

// stringListToTF renders nil and empty slices as null, mirroring mapToTF.
func stringListToTF(ctx context.Context, s []string) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(s) == 0 {
		return types.ListNull(types.StringType), diags
	}

	attrs := make([]attr.Value, 0, len(s))
	for _, v := range s {
		attrs = append(attrs, types.StringValue(v))
	}

	list, d := types.ListValue(types.StringType, attrs)
	diags.Append(d...)
	return list, diags
}
