package workflow

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed schemas/*.json
var embeddedSchemas embed.FS

// NodeSchemaField represents one configurable field in a node's schema.
type NodeSchemaField struct {
	Key         string                `json:"key"`
	Label       string                `json:"label"`
	Type        string                `json:"type"`
	Required    bool                  `json:"required"`
	Default     interface{}           `json:"default,omitempty"`
	Placeholder string                `json:"placeholder,omitempty"`
	Help        string                `json:"help,omitempty"`
	Options     []string              `json:"options,omitempty"`
	Language    string                `json:"language,omitempty"`
	Rows        int                   `json:"rows,omitempty"`
	Min         *float64              `json:"min,omitempty"`
	Max         *float64              `json:"max,omitempty"`
	ItemType    string                `json:"item_type,omitempty"`
	Resource    *ResourcePickerConfig `json:"resource,omitempty"`
	DependsOn   *FieldDependency      `json:"depends_on,omitempty"`
}

// ResourcePickerConfig configures a resource_picker field.
type ResourcePickerConfig struct {
	Type        string `json:"type"`
	CreateLabel string `json:"create_label,omitempty"`
	ParamField  string `json:"param_field,omitempty"`
}

// FieldDependency hides a field unless another field has one of the given values.
// Uses "key" (not "field") to reference the sibling field's key.
type FieldDependency struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

// NodeSchema is the schema embedded in each workflow node.
type NodeSchema struct {
	CredentialPlatform *string           `json:"credential_platform"`
	Fields             []NodeSchemaField `json:"fields"`
}

// browserPlatforms are the platform prefixes whose nodes fall back to browser.generic
// when no platform-specific schema file exists.
var browserPlatforms = map[string]bool{
	"instagram": true,
	"linkedin":  true,
	"x":         true,
	"tiktok":    true,
}

// LoadDefaultSchema loads the embedded schema JSON for the given node type.
// For browser platform nodes (e.g. "linkedin.find_by_keyword") that have no
// dedicated schema file, falls back to browser.generic.json.
// Returns an empty schema (no fields) if no schema file exists for the type.
func LoadDefaultSchema(nodeType string) (*NodeSchema, error) {
	fileName := "schemas/" + nodeType + ".json"
	data, err := embeddedSchemas.ReadFile(fileName)
	if err != nil {
		// For browser platform nodes, try the action-suffix schema first
		// (e.g. "action.find_by_keyword.json"), then fall back to browser.generic.
		if dot := strings.Index(nodeType, "."); dot > 0 {
			if browserPlatforms[nodeType[:dot]] {
				suffix := nodeType[dot+1:]
				data, err = embeddedSchemas.ReadFile("schemas/action." + suffix + ".json")
				if err != nil {
					data, err = embeddedSchemas.ReadFile("schemas/browser.generic.json")
				}
			}
		}
	}
	if err != nil {
		return &NodeSchema{Fields: []NodeSchemaField{}}, nil
	}
	var schema NodeSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("schema_loader: parse %s: %w", fileName, err)
	}
	if schema.Fields == nil {
		schema.Fields = []NodeSchemaField{}
	}
	return &schema, nil
}

// ListEmbeddedSchemas returns all node type names that have an embedded schema.
func ListEmbeddedSchemas() []string {
	entries, err := embeddedSchemas.ReadDir("schemas")
	if err != nil {
		return nil
	}
	types := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".json") {
			types = append(types, strings.TrimSuffix(name, ".json"))
		}
	}
	return types
}
