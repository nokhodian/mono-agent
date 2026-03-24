package workflow

import (
	"strings"
	"testing"
)

func TestExpressionEvaluateString_JSONName(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"name": "Alice"},
	}
	got, err := engine.EvaluateString(`{{$json.name}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Alice" {
		t.Errorf("got %q, want %q", got, "Alice")
	}
}

func TestExpressionEvaluateString_JSONCount(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"count": 42},
	}
	got, err := engine.EvaluateString(`{{$json.count}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

func TestExpressionEvaluateString_UpperFunc(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"name": "Alice"},
	}
	got, err := engine.EvaluateString(`{{upper $json.name}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ALICE" {
		t.Errorf("got %q, want %q", got, "ALICE")
	}
}

func TestExpressionEvaluateString_DefaultFunc(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{},
	}
	// $json.missing is zero (empty string with missingkey=zero)
	got, err := engine.EvaluateString(`{{default "unknown" $json.missing}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "unknown" {
		t.Errorf("got %q, want %q", got, "unknown")
	}
}

func TestExpressionEvaluateString_Passthrough(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{}
	got, err := engine.EvaluateString("hello world", ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExpressionEvaluateString_NodeOutput(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"name": "Alice"},
		Node: map[string][]Item{
			"MyNode": {
				{JSON: map[string]interface{}{"result": "ok"}},
			},
		},
	}
	// Access the first item's JSON of "MyNode" via $node["MyNode"].json.result
	got, err := engine.EvaluateString(`{{(index $node "MyNode").json.result}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("got %q, want %q", got, "ok")
	}
}

func TestExpressionResolveConfig(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"name": "Alice", "score": 99},
	}
	config := map[string]interface{}{
		"greeting": "Hello, {{$json.name}}!",
		"score":    `{{$json.score}}`,
		"nested": map[string]interface{}{
			"label": "Name: {{$json.name}}",
		},
		"literal": 42, // non-string passes through
	}

	resolved, err := engine.ResolveConfig(config, ctx)
	if err != nil {
		t.Fatalf("ResolveConfig error: %v", err)
	}

	if v, ok := resolved["greeting"].(string); !ok || v != "Hello, Alice!" {
		t.Errorf("greeting: got %v, want %q", resolved["greeting"], "Hello, Alice!")
	}
	if v, ok := resolved["score"].(string); !ok || v != "99" {
		t.Errorf("score: got %v, want %q", resolved["score"], "99")
	}
	nested, ok := resolved["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("nested: expected map, got %T", resolved["nested"])
	}
	if v, ok := nested["label"].(string); !ok || v != "Name: Alice" {
		t.Errorf("nested.label: got %v, want %q", nested["label"], "Name: Alice")
	}
	if v, ok := resolved["literal"].(int); !ok || v != 42 {
		t.Errorf("literal: got %v, want 42", resolved["literal"])
	}
}

func TestExpressionEvaluateBool(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"active": "true"},
	}
	got, err := engine.EvaluateBool(`{{$json.active}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
}

func TestExpressionEvaluateString_LowerFunc(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"city": "LONDON"},
	}
	got, err := engine.EvaluateString(`{{lower $json.city}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "london" {
		t.Errorf("got %q, want %q", got, "london")
	}
}

func TestExpressionEvaluateString_WorkflowID(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		WorkflowID: "wf-123",
	}
	got, err := engine.EvaluateString(`{{$workflow.id}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "wf-123" {
		t.Errorf("got %q, want %q", got, "wf-123")
	}
}

func TestExpressionEvaluateString_EnvVar(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		Env: map[string]string{"MY_VAR": "hello_env"},
	}
	got, err := engine.EvaluateString(`{{$env.MY_VAR}}`, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello_env" {
		t.Errorf("got %q, want %q", got, "hello_env")
	}
}

func TestExpressionTemplateCache(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"x": "cached"},
	}
	tmpl := `{{$json.x}}`
	// Call twice to exercise the cache path
	for i := 0; i < 2; i++ {
		got, err := engine.EvaluateString(tmpl, ctx)
		if err != nil {
			t.Fatalf("iter %d: unexpected error: %v", i, err)
		}
		if got != "cached" {
			t.Errorf("iter %d: got %q, want %q", i, got, "cached")
		}
	}
}

func TestExpressionEvaluateString_ParseError(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{}
	_, err := engine.EvaluateString(`{{.unclosed`, ctx)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
	if !strings.Contains(err.Error(), "expression:") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

func TestExpressionResolveItem(t *testing.T) {
	engine := NewExpressionEngine()
	ctx := ExpressionContext{
		JSON: map[string]interface{}{"user": "Bob"},
	}
	item := Item{
		JSON: map[string]interface{}{
			"greeting": "Hi {{$json.user}}",
			"count":    7,
		},
	}
	resolved, err := engine.ResolveItem(item, ctx)
	if err != nil {
		t.Fatalf("ResolveItem error: %v", err)
	}
	if v, ok := resolved.JSON["greeting"].(string); !ok || v != "Hi Bob" {
		t.Errorf("greeting: got %v, want %q", resolved.JSON["greeting"], "Hi Bob")
	}
	if v, ok := resolved.JSON["count"].(int); !ok || v != 7 {
		t.Errorf("count: got %v, want 7", resolved.JSON["count"])
	}
}
