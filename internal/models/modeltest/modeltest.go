// Package modeltest provides assertions that a Terraform model covers exactly
// the attributes of its schema.
package modeltest

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// AttributeNames returns the sorted tfsdk tag names of the exported fields of
// model.
func AttributeNames(model any) []string {
	typ := reflect.TypeOf(model)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	names := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		tag := typ.Field(i).Tag.Get("tfsdk")
		if tag == "" || tag == "-" {
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	sort.Strings(names)
	return names
}

// Keys returns the sorted keys of a schema attribute or block map.
func Keys[V any](values map[string]V) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AssertSchemaMatches fails the test when the model's tfsdk tags differ from
// the attribute and block names of a resource or data source schema. The
// framework keeps the resource and data source schema types separate, so the
// caller passes both name maps and this helper joins them.
func AssertSchemaMatches[AV, BV any](t *testing.T, model any, attributes map[string]AV, blocks map[string]BV) {
	t.Helper()
	names := append(Keys(attributes), Keys(blocks)...)
	AssertAttributesMatch(t, model, names)
}

// AssertAttributesMatch fails the test when the model's tfsdk tags differ from
// want.
func AssertAttributesMatch(t *testing.T, model any, want []string) {
	t.Helper()

	got := AttributeNames(model)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("%T attributes = %v, schema attributes = %v", model, got, expected)
	}
}
