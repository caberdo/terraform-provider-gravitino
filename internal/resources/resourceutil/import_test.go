package resourceutil

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func importStateRequest(id string) resource.ImportStateRequest {
	return resource.ImportStateRequest{ID: id}
}

func TestSplitImportID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		n       int
		exact   bool
		wantOK  bool
		wantLen int
	}{
		{name: "four segments", id: "ml.cat.sch.tbl", n: 4, wantOK: true, wantLen: 4},
		{name: "final segment may contain dots", id: "ml.cat.sch.a.b", n: 4, wantOK: true, wantLen: 4},
		{name: "too few", id: "ml.cat", n: 4, wantOK: false},
		{name: "empty segment", id: "ml..sch.tbl", n: 4, wantOK: false},
		{name: "exact rejects extra dots", id: "ml.cat.sch.a.b", n: 4, exact: true, wantOK: false},
		{name: "exact accepts exactly n", id: "ml.role", n: 2, exact: true, wantOK: true, wantLen: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &resource.ImportStateResponse{}
			var parts []string
			var ok bool
			if tt.exact {
				parts, ok = SplitImportIDExact(importStateRequest(tt.id), resp, tt.n, "expected")
			} else {
				parts, ok = SplitImportID(importStateRequest(tt.id), resp, tt.n, "expected")
			}

			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t (diagnostics: %v)", ok, tt.wantOK, resp.Diagnostics)
			}
			if tt.wantOK {
				if len(parts) != tt.wantLen {
					t.Errorf("parts = %v, want %d segments", parts, tt.wantLen)
				}
				if resp.Diagnostics.HasError() {
					t.Errorf("unexpected diagnostics: %v", resp.Diagnostics)
				}
			} else if !resp.Diagnostics.HasError() {
				t.Error("expected an error diagnostic")
			}
		})
	}
}
