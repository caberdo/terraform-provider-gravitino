package models

import (
	"encoding/json"
	"testing"
)

// TestIndexPropertiesSerialization pins the wire format of the Gravitino v1.3.1
// IndexSpec addition: the extra index parameters are serialised under
// "properties", and the key is omitted entirely for an index that carries none.
// The schema is indexes.yaml#/IndexSpec, which has no example payload for the
// new field, so the JSON below follows the schema directly.
func TestIndexPropertiesSerialization(t *testing.T) {
	raw := `[
		{
			"indexType": "data_skipping_minmax",
			"name": "idx_minmax",
			"fieldNames": [["id"]],
			"properties": {"granularity": "3"}
		},
		{
			"indexType": "primary_key",
			"name": "PRIMARY",
			"fieldNames": [["id"]]
		}
	]`

	var indexes []Index
	if err := json.Unmarshal([]byte(raw), &indexes); err != nil {
		t.Fatalf("unmarshalling the spec payload: %v", err)
	}
	if len(indexes) != 2 {
		t.Fatalf("got %d indexes, want 2", len(indexes))
	}
	if got := indexes[0].Properties["granularity"]; got != "3" {
		t.Errorf("granularity = %q, want 3", got)
	}
	if indexes[1].Properties != nil {
		t.Errorf("an index without properties must unmarshal to none, got %v", indexes[1].Properties)
	}

	encoded, err := json.Marshal(indexes)
	if err != nil {
		t.Fatalf("marshalling the indexes: %v", err)
	}

	var roundTripped []map[string]interface{}
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshalling the encoded indexes: %v", err)
	}
	properties, ok := roundTripped[0]["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("the properties of the data-skipping index are missing: %s", encoded)
	}
	if got := properties["granularity"]; got != "3" {
		t.Errorf("round-tripped granularity = %v, want 3", got)
	}
	if _, exists := roundTripped[1]["properties"]; exists {
		t.Errorf("properties must be omitted for an index without any: %s", encoded)
	}
}

// TestDistributionSerialisesFuncArgs pins issue #64: Gravitino's
// JsonUtils.DistributionDeserializer does node.get("funcArgs").forEach(...)
// without a null check, so a create request whose distribution omits the
// member fails with "Malformed json request" and a NullPointerException. Every
// distribution the provider sends must carry funcArgs, including as [].
func TestDistributionSerialisesFuncArgs(t *testing.T) {
	tests := []struct {
		name         string
		distribution Distribution
	}{
		{
			name:         "nil args",
			distribution: Distribution{Strategy: "hash", Number: 1},
		},
		{
			name:         "empty args",
			distribution: Distribution{Strategy: "hash", Number: 1, FuncArgs: []Expression{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(tt.distribution)
			if err != nil {
				t.Fatalf("marshalling the distribution: %v", err)
			}

			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("unmarshalling the encoded distribution: %v", err)
			}

			args, ok := decoded["funcArgs"]
			if !ok {
				t.Fatalf("distribution is missing funcArgs: %s", encoded)
			}
			if string(args) != "[]" {
				t.Errorf("funcArgs = %s, want []", args)
			}
			if _, ok := decoded["number"]; !ok {
				t.Errorf("distribution is missing number: %s", encoded)
			}
		})
	}
}
