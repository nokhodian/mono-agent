package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

// preamble is prepended to every template so authors can use $json, $node,
// $workflow, $execution, and $env directly without referencing dot.
const preamble = `{{- $json := .json -}}` +
	`{{- $node := .node -}}` +
	`{{- $workflow := .workflow -}}` +
	`{{- $execution := .execution -}}` +
	`{{- $env := .env -}}`

// nodeJSONProxy is a map[string]interface{} with a single key "json" whose
// value is the node's first-item JSON map.  This lets templates use the syntax
// $node["Name"].json.someKey — the template looks up key "json" on the map
// and then key "someKey" on the inner map.
type nodeJSONProxy = map[string]interface{}

// ExpressionEngine evaluates Go text/template expressions against an
// ExpressionContext.  Parsed templates are cached in a sync.Map for
// performance.
type ExpressionEngine struct {
	cache sync.Map // key: template string → *template.Template
	funcs template.FuncMap
}

// NewExpressionEngine returns a fully configured ExpressionEngine.
func NewExpressionEngine() *ExpressionEngine {
	e := &ExpressionEngine{}
	e.funcs = e.buildFuncMap()
	return e
}

// buildFuncMap returns the safe function set exposed to all templates.
func (e *ExpressionEngine) buildFuncMap() template.FuncMap {
	return template.FuncMap{
		// Encoding
		"json": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(b), nil
		},
		"jsonParse": func(s string) (interface{}, error) {
			var out interface{}
			if err := json.Unmarshal([]byte(s), &out); err != nil {
				return nil, err
			}
			return out, nil
		},

		// Type conversions
		"toString": func(v interface{}) string {
			if v == nil {
				return ""
			}
			return fmt.Sprintf("%v", v)
		},
		"toInt": func(v interface{}) (int64, error) {
			switch t := v.(type) {
			case int:
				return int64(t), nil
			case int32:
				return int64(t), nil
			case int64:
				return t, nil
			case float32:
				return int64(t), nil
			case float64:
				return int64(t), nil
			case string:
				return strconv.ParseInt(strings.TrimSpace(t), 10, 64)
			case bool:
				if t {
					return 1, nil
				}
				return 0, nil
			default:
				return 0, fmt.Errorf("toInt: cannot convert %T", v)
			}
		},
		"toFloat": func(v interface{}) (float64, error) {
			switch t := v.(type) {
			case int:
				return float64(t), nil
			case int32:
				return float64(t), nil
			case int64:
				return float64(t), nil
			case float32:
				return float64(t), nil
			case float64:
				return t, nil
			case string:
				return strconv.ParseFloat(strings.TrimSpace(t), 64)
			case bool:
				if t {
					return 1, nil
				}
				return 0, nil
			default:
				return 0, fmt.Errorf("toFloat: cannot convert %T", v)
			}
		},
		"toBool": func(v interface{}) (bool, error) {
			switch t := v.(type) {
			case bool:
				return t, nil
			case int:
				return t != 0, nil
			case int64:
				return t != 0, nil
			case float64:
				return t != 0, nil
			case string:
				return strconv.ParseBool(strings.TrimSpace(t))
			default:
				return false, fmt.Errorf("toBool: cannot convert %T", v)
			}
		},

		// String operations
		"upper":     strings.ToUpper,
		"lower":     strings.ToLower,
		"trim":      strings.TrimSpace,
		"trimLeft":  func(cutset, s string) string { return strings.TrimLeft(s, cutset) },
		"trimRight": func(cutset, s string) string { return strings.TrimRight(s, cutset) },
		"split":     func(sep, s string) []string { return strings.Split(s, sep) },
		"join":      func(sep string, parts []string) string { return strings.Join(parts, sep) },

		// Map helpers
		"hasKey": func(m map[string]interface{}, key string) bool {
			if m == nil {
				return false
			}
			_, ok := m[key]
			return ok
		},
		"default": func(def interface{}, v interface{}) interface{} {
			if v == nil {
				return def
			}
			// Also treat zero-value string as absent.
			if s, ok := v.(string); ok && s == "" {
				return def
			}
			return v
		},

		// Time
		"now": func() string {
			return time.Now().UTC().Format(time.RFC3339)
		},

		// Arithmetic (all operate on float64)
		"add": func(a, b float64) float64 { return a + b },
		"sub": func(a, b float64) float64 { return a - b },
		"mul": func(a, b float64) float64 { return a * b },
		"div": func(a, b float64) (float64, error) {
			if b == 0 {
				return 0, fmt.Errorf("div: division by zero")
			}
			return a / b, nil
		},

		// Collection helpers
		"len": func(v interface{}) (int, error) {
			switch t := v.(type) {
			case string:
				return len(t), nil
			case []interface{}:
				return len(t), nil
			case map[string]interface{}:
				return len(t), nil
			default:
				return 0, fmt.Errorf("len: unsupported type %T", v)
			}
		},
		// index provides explicit slice/map indexing (supplements the built-in).
		"index": func(collection interface{}, key interface{}) (interface{}, error) {
			switch c := collection.(type) {
			case []interface{}:
				idx, err := toIndexInt(key)
				if err != nil {
					return nil, err
				}
				if idx < 0 || idx >= len(c) {
					return nil, nil
				}
				return c[idx], nil
			case map[string]interface{}:
				k, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("index: map key must be string, got %T", key)
				}
				return c[k], nil
			default:
				return nil, fmt.Errorf("index: unsupported collection type %T", collection)
			}
		},
	}
}

// toIndexInt converts an arbitrary key value to an integer slice index.
func toIndexInt(key interface{}) (int, error) {
	switch k := key.(type) {
	case int:
		return k, nil
	case int64:
		return int(k), nil
	case float64:
		return int(k), nil
	case string:
		n, err := strconv.Atoi(k)
		if err != nil {
			return 0, fmt.Errorf("index: invalid index %q", k)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("index: unsupported key type %T", key)
	}
}

// parse returns a cached *template.Template for the given source string.
// The preamble is always prepended so $json/$node/... variables are available.
func (e *ExpressionEngine) parse(src string) (*template.Template, error) {
	if v, ok := e.cache.Load(src); ok {
		return v.(*template.Template), nil
	}
	full := preamble + src
	t, err := template.New("").
		Option("missingkey=zero").
		Funcs(e.funcs).
		Parse(full)
	if err != nil {
		return nil, fmt.Errorf("expression: parse error in %q: %w", src, err)
	}
	e.cache.Store(src, t)
	return t, nil
}

// buildData converts an ExpressionContext into the map passed as dot to templates.
func buildData(ctx ExpressionContext) map[string]interface{} {
	// .json
	jsonMap := ctx.JSON
	if jsonMap == nil {
		jsonMap = make(map[string]interface{})
	}

	// .node — map of node name → nodeJSONProxy so templates can do
	// $node["Name"].json.someKey
	// nodeJSONProxy is map[string]interface{}{"json": firstItemJSON}.
	nodeMap := make(map[string]interface{}, len(ctx.Node))
	for name, items := range ctx.Node {
		var firstJSON map[string]interface{}
		if len(items) > 0 {
			firstJSON = items[0].JSON
		}
		if firstJSON == nil {
			firstJSON = make(map[string]interface{})
		}
		nodeMap[name] = nodeJSONProxy{"json": firstJSON}
	}

	// .workflow
	workflowMap := map[string]interface{}{
		"id": ctx.WorkflowID,
	}

	// .execution
	executionMap := map[string]interface{}{
		"id": ctx.ExecutionID,
	}

	// .env — merge OS environment with explicit Env overrides
	envMap := make(map[string]interface{})
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}
	for k, v := range ctx.Env {
		envMap[k] = v
	}

	return map[string]interface{}{
		"json":      jsonMap,
		"node":      nodeMap,
		"workflow":  workflowMap,
		"execution": executionMap,
		"env":       envMap,
	}
}

// EvaluateString evaluates a template string against the given context.
// Strings that contain no {{ }} delimiters are returned as-is without parsing.
func (e *ExpressionEngine) EvaluateString(tmpl string, ctx ExpressionContext) (string, error) {
	if !strings.Contains(tmpl, "{{") {
		return tmpl, nil
	}

	t, err := e.parse(tmpl)
	if err != nil {
		return "", err
	}

	data := buildData(ctx)

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("expression: execute error: %w", err)
	}
	return buf.String(), nil
}

// EvaluateBool evaluates a template string and converts the result to bool.
func (e *ExpressionEngine) EvaluateBool(tmpl string, ctx ExpressionContext) (bool, error) {
	s, err := e.EvaluateString(tmpl, ctx)
	if err != nil {
		return false, err
	}
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no", "":
		return false, nil
	}
	return false, fmt.Errorf("expression: cannot convert %q to bool", s)
}

// EvaluateValue evaluates a template and returns the raw interface{} result.
// The output is parsed as JSON when possible; otherwise the raw string is returned.
func (e *ExpressionEngine) EvaluateValue(tmpl string, ctx ExpressionContext) (interface{}, error) {
	s, err := e.EvaluateString(tmpl, ctx)
	if err != nil {
		return nil, err
	}
	s = strings.TrimSpace(s)
	var out interface{}
	if jsonErr := json.Unmarshal([]byte(s), &out); jsonErr == nil {
		return out, nil
	}
	return s, nil
}

// ResolveConfig walks a config map and evaluates all string values as
// templates.  Non-string values pass through unchanged.  Nested maps and
// slices are recursively resolved.
func (e *ExpressionEngine) ResolveConfig(config map[string]interface{}, ctx ExpressionContext) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		resolved, err := e.resolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("expression: resolving key %q: %w", k, err)
		}
		out[k] = resolved
	}
	return out, nil
}

// resolveValue recursively resolves a single value.
func (e *ExpressionEngine) resolveValue(v interface{}, ctx ExpressionContext) (interface{}, error) {
	switch t := v.(type) {
	case string:
		return e.EvaluateString(t, ctx)
	case map[string]interface{}:
		return e.ResolveConfig(t, ctx)
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, elem := range t {
			resolved, err := e.resolveValue(elem, ctx)
			if err != nil {
				return nil, fmt.Errorf("expression: resolving slice index %d: %w", i, err)
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}

// ResolveItem walks an Item's JSON map and evaluates all string values as
// templates, returning a new Item with resolved values.
func (e *ExpressionEngine) ResolveItem(item Item, ctx ExpressionContext) (Item, error) {
	resolved, err := e.ResolveConfig(item.JSON, ctx)
	if err != nil {
		return Item{}, err
	}
	return Item{
		JSON:   resolved,
		Binary: item.Binary,
	}, nil
}
