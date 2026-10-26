// Package tfutil holds small, pure conversions between Terraform Plugin
// Framework values and the plain Go types the Gravitino API uses. They were
// duplicated across the resource packages before.
package tfutil

import (
	"context"
	"reflect"
	"strings"

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

// FullyKnown reports whether every Terraform value reachable from value is
// known. value may be an attr.Value, a TFSDK model struct, a pointer to one, or
// a slice or map of those. Fields whose tfsdk tag (or Go name) appears in skip
// are ignored, which is how a value the API assigns — an index name, for
// example — is excluded from the check.
//
// This is the "is this block fully known" half of the unknown-value rule: a
// block that still contains an unknown value cannot be compared with the state
// yet, so the caller must not report a diff for it.
func FullyKnown(value any, skip ...string) bool {
	return fullyKnown(reflect.ValueOf(value), skip)
}

func fullyKnown(value reflect.Value, skip []string) bool {
	if !value.IsValid() {
		return true
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
	}
	if value.CanInterface() {
		if attribute, ok := value.Interface().(attr.Value); ok {
			return !attribute.IsUnknown()
		}
	}
	switch value.Kind() {
	case reflect.Struct:
		typ := value.Type()
		for i := range value.NumField() {
			field := typ.Field(i)
			if field.PkgPath != "" { // unexported
				continue
			}
			if skippedField(field, skip) {
				continue
			}
			if !fullyKnown(value.Field(i), skip) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range value.Len() {
			if !fullyKnown(value.Index(i), skip) {
				return false
			}
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if !fullyKnown(iter.Value(), skip) {
				return false
			}
		}
	}
	return true
}

func skippedField(field reflect.StructField, skip []string) bool {
	tag := strings.Split(field.Tag.Get("tfsdk"), ",")[0]
	for _, name := range skip {
		if name != "" && (name == tag || name == field.Name) {
			return true
		}
	}
	return false
}

// StringOrPrevious returns planned when it is known, previous when planned is
// unknown and previous is usable, and null otherwise. It is the state-write
// half of the unknown-value rule: a value that was resolved during apply must
// never reach the provider-produced state as unknown.
func StringOrPrevious(planned, previous types.String) types.String {
	if !planned.IsUnknown() {
		return planned
	}
	if previous.IsNull() || previous.IsUnknown() {
		return types.StringNull()
	}
	return previous
}

// MapOrPrevious is the types.Map form of StringOrPrevious.
func MapOrPrevious(ctx context.Context, planned, previous types.Map) types.Map {
	if !planned.IsUnknown() {
		return planned
	}
	if previous.IsNull() || previous.IsUnknown() {
		return types.MapNull(planned.ElementType(ctx))
	}
	return previous
}

// ObjectOrPrevious is the types.Object form of StringOrPrevious.
func ObjectOrPrevious(ctx context.Context, planned, previous types.Object) types.Object {
	if !planned.IsUnknown() {
		return planned
	}
	if previous.IsNull() || previous.IsUnknown() {
		return types.ObjectNull(planned.AttributeTypes(ctx))
	}
	return previous
}
