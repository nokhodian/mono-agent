package config

import "fmt"

// ConfigHelper provides convenience methods for working with Config objects.
type ConfigHelper struct{}

// GetXPath searches the Config's field tree for a field with the given name
// and returns its XPath value. The search is a recursive depth-first
// traversal. Returns an error if the field is not found.
func (ch *ConfigHelper) GetXPath(config *Config, fieldName string) (string, error) {
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}
	xpath, err := findXPathRecursive(config.Fields, fieldName)
	if err != nil {
		return "", fmt.Errorf("field %q not found in config %q: %w", fieldName, config.ConfigName, err)
	}
	return xpath, nil
}

// findXPathRecursive performs a depth-first search through the ConfigField
// tree looking for a node whose Name matches fieldName. It returns the XPath
// of the first match.
func findXPathRecursive(node ConfigField, fieldName string) (string, error) {
	if node.Name == fieldName {
		if node.XPath != "" {
			return node.XPath, nil
		}
		return "", fmt.Errorf("field %q found but has no XPath", fieldName)
	}

	for _, child := range node.Data {
		xpath, err := findXPathRecursive(child, fieldName)
		if err == nil {
			return xpath, nil
		}
	}

	return "", fmt.Errorf("field %q not found", fieldName)
}
