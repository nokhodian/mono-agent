package action

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// VariableResolver handles {{variable.path}} template resolution within
// action step definitions.
type VariableResolver struct {
	context *ExecutionContext
}

// NewVariableResolver creates a resolver bound to the given execution context.
func NewVariableResolver(ctx *ExecutionContext) *VariableResolver {
	return &VariableResolver{context: ctx}
}

// Resolve replaces all {{variable.path}} occurrences in template with their
// resolved string values. If the template contains only a single placeholder
// and nothing else, the raw value is stringified; otherwise each placeholder
// is interpolated into the surrounding text.
func (vr *VariableResolver) Resolve(template string) string {
	if template == "" {
		return template
	}

	return templatePattern.ReplaceAllStringFunc(template, func(match string) string {
		// Strip the {{ and }} delimiters.
		path := strings.TrimSpace(match[2 : len(match)-2])
		val := vr.ResolvePath(path)
		if val == nil {
			return ""
		}
		return fmt.Sprintf("%v", val)
	})
}

// ResolveValue preserves the underlying Go type when the value is a string
// containing a single template reference (e.g. "{{extract_post_urls.data}}").
// If the string contains mixed content or is not a string at all, it is
// returned as-is (with string templates resolved).
func (vr *VariableResolver) ResolveValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		matches := templatePattern.FindAllStringIndex(v, -1)
		if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(v) {
			// The entire string is a single template reference — preserve type.
			path := strings.TrimSpace(v[2 : len(v)-2])
			resolved := vr.ResolvePath(path)
			if resolved != nil {
				return resolved
			}
			return ""
		}
		return vr.Resolve(v)

	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, val := range v {
			result[key] = vr.ResolveValue(val)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = vr.ResolveValue(val)
		}
		return result

	default:
		return value
	}
}

// ResolvePath navigates a dot-separated path through the execution context
// to retrieve the target value. It supports several access patterns:
//
//   - Simple variable:          "keyword"         → Variables["keyword"]
//   - Dot path:                 "item.url"        → Variables["item"]["url"]
//   - Step result data:         "step_id.data"    → StepResults["step_id"].Data
//   - Step result count:        "step_id.count"   → len(StepResults["step_id"].Data)
//   - Step result success:      "step_id.success" → StepResults["step_id"].Success
//   - Array index:              "items[0]"        → Variables["items"][0]
//   - Data map:                 "data.key"        → Data["key"]
func (vr *VariableResolver) ResolvePath(path string) interface{} {
	if path == "" {
		return nil
	}

	// Support "a or b or c" — return first non-nil, non-empty value.
	if strings.Contains(path, " or ") {
		for _, part := range strings.Split(path, " or ") {
			part = strings.TrimSpace(part)
			resolved := vr.ResolvePath(part)
			if resolved != nil {
				if s, ok := resolved.(string); !ok || s != "" {
					return resolved
				}
			}
			// Treat as a literal if it looks like a number, boolean, or quoted string.
			if f, err := strconv.ParseFloat(part, 64); err == nil {
				return f
			}
			if part == "true" {
				return true
			}
			if part == "false" {
				return false
			}
			if len(part) >= 2 && ((part[0] == '"' && part[len(part)-1] == '"') || (part[0] == '\'' && part[len(part)-1] == '\'')) {
				return part[1 : len(part)-1]
			}
		}
		return nil
	}

	parts := splitPath(path)
	if len(parts) == 0 {
		return nil
	}

	root := parts[0]

	// Parse array access from the root segment (e.g. "searches[0]").
	rootName, rootIndex, rootHasIndex := parseArrayAccess(root)

	// Check step results first for dotted paths like "step_id.data".
	if len(parts) >= 2 && !rootHasIndex {
		if sr := vr.context.GetStepResult(root); sr != nil {
			return vr.resolveStepResultField(sr, parts[1:])
		}
	}

	// Look up base name in Variables.
	lookupKey := rootName
	vr.context.mu.Lock()
	val, exists := vr.context.Variables[lookupKey]
	vr.context.mu.Unlock()

	if !exists {
		// Check Data map.
		vr.context.mu.Lock()
		val, exists = vr.context.Data[lookupKey]
		vr.context.mu.Unlock()
		if !exists {
			return nil
		}
	}

	// Apply array index on root if present (e.g. searches[0]).
	if rootHasIndex {
		val = vr.accessSlice(val, rootIndex)
	}

	// Navigate remaining path segments.
	if len(parts) > 1 {
		return vr.navigatePath(val, parts[1:])
	}

	return val
}

// resolveStepResultField extracts a named field from a StepResult.
func (vr *VariableResolver) resolveStepResultField(sr *StepResult, parts []string) interface{} {
	if len(parts) == 0 {
		return nil
	}

	field := parts[0]
	remaining := parts[1:]

	var val interface{}
	switch field {
	case "data":
		val = sr.Data
	case "count":
		switch d := sr.Data.(type) {
		case []interface{}:
			return len(d)
		case []map[string]interface{}:
			return len(d)
		case []string:
			return len(d)
		default:
			return 0
		}
	case "success":
		return sr.Success
	case "error":
		if sr.Error != nil {
			return sr.Error.Error()
		}
		return nil
	case "element":
		val = sr.Element
	default:
		// Try navigating into sr.Data.
		val = sr.Data
		remaining = parts
	}

	if len(remaining) > 0 && val != nil {
		return vr.navigatePath(val, remaining)
	}
	return val
}

// navigatePath walks through nested maps and slices using the given path
// segments.
func (vr *VariableResolver) navigatePath(current interface{}, parts []string) interface{} {
	for _, part := range parts {
		if current == nil {
			return nil
		}

		// Check for array index notation: "items[0]".
		name, index, hasIndex := parseArrayAccess(part)
		if hasIndex {
			current = vr.accessMap(current, name)
			current = vr.accessSlice(current, index)
			continue
		}

		current = vr.accessMap(current, part)
	}
	return current
}

// accessMap extracts a keyed value from a map or returns nil.
func (vr *VariableResolver) accessMap(current interface{}, key string) interface{} {
	switch m := current.(type) {
	case map[string]interface{}:
		return m[key]
	case map[string]string:
		return m[key]
	default:
		return nil
	}
}

// accessSlice extracts an indexed value from a slice or returns nil.
func (vr *VariableResolver) accessSlice(current interface{}, index int) interface{} {
	switch s := current.(type) {
	case []interface{}:
		if index >= 0 && index < len(s) {
			return s[index]
		}
	case []string:
		if index >= 0 && index < len(s) {
			return s[index]
		}
	case []map[string]interface{}:
		if index >= 0 && index < len(s) {
			return s[index]
		}
	}
	return nil
}

// splitPath splits a dotted path string into segments, handling array bracket
// notation as part of the segment.
func splitPath(path string) []string {
	return strings.Split(path, ".")
}

// parseArrayAccess parses "name[index]" into its components. Returns the name,
// index, and whether array access notation was present.
func parseArrayAccess(part string) (string, int, bool) {
	bracketIdx := strings.Index(part, "[")
	if bracketIdx < 0 {
		return part, 0, false
	}
	closeBracket := strings.Index(part, "]")
	if closeBracket < 0 || closeBracket <= bracketIdx+1 {
		return part, 0, false
	}

	name := part[:bracketIdx]
	indexStr := part[bracketIdx+1 : closeBracket]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return part, 0, false
	}
	return name, index, true
}

// ResolveStepDef creates a deep copy of the given StepDef with all template
// references resolved to their current values.
func (vr *VariableResolver) ResolveStepDef(step StepDef) StepDef {
	resolved := step

	resolved.URL = vr.Resolve(step.URL)
	resolved.Selector = vr.Resolve(step.Selector)
	resolved.ConfigKey = vr.Resolve(step.ConfigKey)
	resolved.ElementRef = vr.Resolve(step.ElementRef)
	resolved.Attribute = vr.Resolve(step.Attribute)
	resolved.Direction = vr.Resolve(step.Direction)
	resolved.MethodName = vr.Resolve(step.MethodName)
	resolved.Variable = vr.Resolve(step.Variable)
	resolved.WaitFor = vr.Resolve(step.WaitFor)
	resolved.WaitAfter = vr.Resolve(step.WaitAfter)
	if step.Duration != nil {
		resolved.Duration = vr.ResolveValue(step.Duration)
	}

	if step.Value != nil {
		resolved.Value = vr.ResolveValue(step.Value)
	}

	// Resolve alternatives.
	if len(step.Alternatives) > 0 {
		alts := make([]string, len(step.Alternatives))
		for i, alt := range step.Alternatives {
			alts[i] = vr.Resolve(alt)
		}
		resolved.Alternatives = alts
	}

	// Resolve args.
	if len(step.Args) > 0 {
		args := make([]interface{}, len(step.Args))
		for i, arg := range step.Args {
			args[i] = vr.ResolveValue(arg)
		}
		resolved.Args = args
	}

	// Resolve race selectors.
	if len(step.RaceSelectors) > 0 {
		race := make(map[string]string, len(step.RaceSelectors))
		for k, v := range step.RaceSelectors {
			race[k] = vr.Resolve(v)
		}
		resolved.RaceSelectors = race
	}

	// Resolve Set map (for update_progress steps).
	if len(step.Set) > 0 {
		setMap := make(map[string]interface{}, len(step.Set))
		for k, v := range step.Set {
			setMap[k] = vr.ResolveValue(v)
		}
		resolved.Set = setMap
	}

	return resolved
}
