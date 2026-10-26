package resourceutil

import (
	"reflect"
	"testing"
)

func TestAssociationID(t *testing.T) {
	if got := AssociationID("ml", "TABLE", "sales.orders"); got != "ml.TABLE.sales.orders" {
		t.Errorf("AssociationID = %q", got)
	}
}

func TestDiffAssociationNames(t *testing.T) {
	tests := []struct {
		name       string
		current    []string
		desired    []string
		wantAdd    []string
		wantRemove []string
	}{
		{
			name:    "nothing changes",
			current: []string{"a", "b"},
			desired: []string{"b", "a"},
		},
		{
			name:       "add and remove",
			current:    []string{"a", "b"},
			desired:    []string{"b", "c"},
			wantAdd:    []string{"c"},
			wantRemove: []string{"a"},
		},
		{
			name:    "empty current",
			current: nil,
			desired: []string{"a"},
			wantAdd: []string{"a"},
		},
		{
			name:       "empty desired",
			current:    []string{"a"},
			desired:    nil,
			wantRemove: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAdd, gotRemove := DiffAssociationNames(tt.current, tt.desired)
			if !reflect.DeepEqual(gotAdd, tt.wantAdd) {
				t.Errorf("toAdd = %v, want %v", gotAdd, tt.wantAdd)
			}
			if !reflect.DeepEqual(gotRemove, tt.wantRemove) {
				t.Errorf("toRemove = %v, want %v", gotRemove, tt.wantRemove)
			}
		})
	}
}
