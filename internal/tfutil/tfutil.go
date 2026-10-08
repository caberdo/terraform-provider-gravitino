// Package tfutil holds small, pure conversions between Terraform Plugin
// Framework values and the plain Go types the Gravitino API uses. They were
// duplicated across the resource packages before.
package tfutil

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// StringMap converts a Terraform map of strings into a Go map. A null or unknown
// map yields an empty (non-nil) map, which is what the API expects for an unset
// properties attribute.
func StringMap(ctx context.Context, value types.Map, diags *diag.Diagnostics) map[string]string {
	result := make(map[string]string)
	if value.IsNull() || value.IsUnknown() {
		return result
	}
	diags.Append(value.ElementsAs(ctx, &result, false)...)
	return result
}

// SetToStrings converts a Terraform set of strings into a Go slice. A null or
// unknown set yields nil.
func SetToStrings(value types.Set) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := make([]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		if s, ok := element.(types.String); ok {
			result = append(result, s.ValueString())
		}
	}
	return result
}

// ListToStrings converts a Terraform list of strings into a Go slice. A null or
// unknown list yields nil.
func ListToStrings(value types.List) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := make([]string, 0, len(value.Elements()))
	for _, element := range value.Elements() {
		if s, ok := element.(types.String); ok {
			result = append(result, s.ValueString())
		}
	}
	return result
}

// StringsToSet converts a Go slice of strings into a Terraform set.
func StringsToSet(values []string) (types.Set, diag.Diagnostics) {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	return types.SetValue(types.StringType, elements)
}

// StringsToList converts a Go slice of strings into a Terraform list.
func StringsToList(values []string) (types.List, diag.Diagnostics) {
	elements := make([]attr.Value, 0, len(values))
	for _, value := range values {
		elements = append(elements, types.StringValue(value))
	}
	return types.ListValue(types.StringType, elements)
}
