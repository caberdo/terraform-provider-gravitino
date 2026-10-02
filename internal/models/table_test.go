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
