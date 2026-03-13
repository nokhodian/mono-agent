package workflow

import (
	"testing"
)

func TestLoadDefaultSchema_KnownType(t *testing.T) {
	schema, err := LoadDefaultSchema("service.google_sheets")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(schema.Fields) == 0 {
		t.Fatal("expected fields for service.google_sheets")
	}
	var found bool
	for _, f := range schema.Fields {
		if f.Key == "spreadsheet_id" && f.Type == "resource_picker" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected spreadsheet_id resource_picker field")
	}
}

func TestLoadDefaultSchema_UnknownType(t *testing.T) {
	schema, err := LoadDefaultSchema("unknown.node_type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if schema == nil {
		t.Fatal("expected non-nil schema for unknown type")
	}
	if len(schema.Fields) != 0 {
		t.Fatalf("expected empty fields for unknown type, got %d", len(schema.Fields))
	}
}

func TestListEmbeddedSchemas(t *testing.T) {
	types := ListEmbeddedSchemas()
	if len(types) < 50 {
		t.Fatalf("expected at least 50 embedded schemas, got %d", len(types))
	}
}
