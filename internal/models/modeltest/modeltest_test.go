package modeltest_test

import (
	"testing"

	"github.com/gravitino/terraform-provider-gravitino/internal/models/modeltest"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sampleModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Skip  string       `tfsdk:"-"`
	Other string       `tfsdk:"other" json:"-"`
}

func TestAttributeNames(t *testing.T) {
	want := []string{"id", "name", "other"}
	got := modeltest.AttributeNames(sampleModel{})
	if len(got) != len(want) {
		t.Fatalf("AttributeNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AttributeNames = %v, want %v", got, want)
		}
	}

	if got := modeltest.AttributeNames(&sampleModel{}); len(got) != len(want) {
		t.Fatalf("AttributeNames(pointer) = %v, want %v", got, want)
	}
}

// fakeSchemaValue stands in for the framework's schema attribute and block
// values: AssertSchemaMatches only needs the map keys.
type fakeSchemaValue struct{}

func TestAssertSchemaMatchesJoinsAttributesAndBlocks(t *testing.T) {
	attributes := map[string]fakeSchemaValue{"id": {}, "name": {}}
	blocks := map[string]fakeSchemaValue{"other": {}}

	modeltest.AssertSchemaMatches(t, sampleModel{}, attributes, blocks)
}
