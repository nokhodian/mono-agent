package ainodes

import (
	"context"
	"testing"

	"github.com/nokhodian/mono-agent/internal/ai"
	"github.com/nokhodian/mono-agent/internal/workflow"
)

// mockClient is a test double that returns a fixed response.
type mockClient struct {
	response string
}

func (m *mockClient) Complete(_ context.Context, _ ai.CompletionRequest) (ai.CompletionResponse, error) {
	return ai.CompletionResponse{Content: m.response}, nil
}

func (m *mockClient) StreamComplete(_ context.Context, _ ai.CompletionRequest, _ func(ai.StreamChunk)) error {
	return nil
}

func TestChatNodeType(t *testing.T) {
	node := &ChatNode{}
	if got := node.Type(); got != "ai.chat" {
		t.Errorf("Type() = %q, want %q", got, "ai.chat")
	}
}

func TestChatNodeExecute(t *testing.T) {
	mock := &mockClient{response: "Hello from AI"}
	node := &ChatNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"name": "Alice", "question": "What is Go?"}},
			{JSON: map[string]interface{}{"name": "Bob", "question": "What is Rust?"}},
		},
	}

	config := map[string]interface{}{
		"prompt":        "Answer this: {{$json.question}}",
		"system_prompt": "You are helpful.",
		"output_key":    "answer",
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output handle, got %d", len(outputs))
	}
	if outputs[0].Handle != "main" {
		t.Errorf("expected handle \"main\", got %q", outputs[0].Handle)
	}
	if len(outputs[0].Items) != 2 {
		t.Fatalf("expected 2 output items, got %d", len(outputs[0].Items))
	}

	// Verify AI response is added under the configured output key.
	for i, item := range outputs[0].Items {
		answer, ok := item.JSON["answer"]
		if !ok {
			t.Errorf("item[%d]: missing \"answer\" key", i)
			continue
		}
		if answer != "Hello from AI" {
			t.Errorf("item[%d]: answer = %q, want %q", i, answer, "Hello from AI")
		}
		// Original fields should still be present.
		if _, ok := item.JSON["name"]; !ok {
			t.Errorf("item[%d]: original field \"name\" missing", i)
		}
	}
}

func TestChatNodeDefaultOutputKey(t *testing.T) {
	mock := &mockClient{response: "response text"}
	node := &ChatNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"text": "hello"}},
		},
	}

	config := map[string]interface{}{
		"prompt": "Process: {{$json.text}}",
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Default output key should be "ai_response".
	if _, ok := outputs[0].Items[0].JSON["ai_response"]; !ok {
		t.Error("expected default output key \"ai_response\"")
	}
}

func TestChatNodeMissingPrompt(t *testing.T) {
	mock := &mockClient{response: "ignored"}
	node := &ChatNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{{JSON: map[string]interface{}{}}},
	}

	_, err := node.Execute(context.Background(), input, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing prompt, got nil")
	}
}

func TestTransformNodeExecute(t *testing.T) {
	mock := &mockClient{response: "HELLO WORLD"}
	node := &TransformNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"text": "hello world"}},
		},
	}

	config := map[string]interface{}{
		"instruction": "Convert to uppercase",
		"input_field": "text",
		"output_key":  "upper",
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if len(outputs) != 1 || outputs[0].Handle != "main" {
		t.Fatalf("expected 1 output with handle \"main\"")
	}
	if len(outputs[0].Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(outputs[0].Items))
	}

	upper, ok := outputs[0].Items[0].JSON["upper"]
	if !ok {
		t.Fatal("missing \"upper\" key in output")
	}
	if upper != "HELLO WORLD" {
		t.Errorf("upper = %q, want %q", upper, "HELLO WORLD")
	}

	// Original field preserved.
	if outputs[0].Items[0].JSON["text"] != "hello world" {
		t.Error("original \"text\" field was modified")
	}
}

func TestTransformNodeType(t *testing.T) {
	node := &TransformNode{}
	if got := node.Type(); got != "ai.transform" {
		t.Errorf("Type() = %q, want %q", got, "ai.transform")
	}
}

func TestClassifyNodeExecute(t *testing.T) {
	mock := &mockClient{response: "tech"}
	node := &ClassifyNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"title": "New Go release", "body": "Go 1.22 is out"}},
			{JSON: map[string]interface{}{"title": "Recipe ideas", "body": "Best pasta recipes"}},
		},
	}

	config := map[string]interface{}{
		"categories": []interface{}{"tech", "food", "sports"},
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	// Should have "main" handle + "tech" handle (since mock always returns "tech").
	if len(outputs) < 2 {
		t.Fatalf("expected at least 2 output handles, got %d", len(outputs))
	}

	// Check main handle.
	var mainOutput *workflow.NodeOutput
	var techOutput *workflow.NodeOutput
	for i := range outputs {
		switch outputs[i].Handle {
		case "main":
			mainOutput = &outputs[i]
		case "tech":
			techOutput = &outputs[i]
		}
	}

	if mainOutput == nil {
		t.Fatal("missing \"main\" output handle")
	}
	if techOutput == nil {
		t.Fatal("missing \"tech\" output handle")
	}

	// All items should have category "tech".
	for i, item := range mainOutput.Items {
		cat, ok := item.JSON["category"]
		if !ok {
			t.Errorf("main item[%d]: missing \"category\"", i)
			continue
		}
		if cat != "tech" {
			t.Errorf("main item[%d]: category = %q, want %q", i, cat, "tech")
		}
		conf, ok := item.JSON["confidence"]
		if !ok {
			t.Errorf("main item[%d]: missing \"confidence\"", i)
		} else if conf != 1.0 {
			t.Errorf("main item[%d]: confidence = %v, want 1.0", i, conf)
		}
	}

	// Tech handle should have all 2 items.
	if len(techOutput.Items) != 2 {
		t.Errorf("tech handle: expected 2 items, got %d", len(techOutput.Items))
	}
}

func TestClassifyNodeType(t *testing.T) {
	node := &ClassifyNode{}
	if got := node.Type(); got != "ai.classify" {
		t.Errorf("Type() = %q, want %q", got, "ai.classify")
	}
}

func TestClassifyNodeMissingCategories(t *testing.T) {
	mock := &mockClient{response: "ignored"}
	node := &ClassifyNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{{JSON: map[string]interface{}{}}},
	}

	_, err := node.Execute(context.Background(), input, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing categories, got nil")
	}
}

func TestExtractNodeType(t *testing.T) {
	node := &ExtractNode{}
	if got := node.Type(); got != "ai.extract" {
		t.Errorf("Type() = %q, want %q", got, "ai.extract")
	}
}

func TestExtractNodeExecute(t *testing.T) {
	mock := &mockClient{response: `{"name": "Alice", "age": 30}`}
	node := &ExtractNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"text": "Alice is 30 years old"}},
		},
	}

	config := map[string]interface{}{
		"prompt":     "Extract name and age from: {{$json.text}}",
		"output_key": "data",
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if len(outputs) != 1 || outputs[0].Handle != "main" {
		t.Fatalf("expected 1 output with handle \"main\"")
	}

	data, ok := outputs[0].Items[0].JSON["data"]
	if !ok {
		t.Fatal("missing \"data\" key in output")
	}

	// The parsed JSON should be a map.
	dataMap, ok := data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", data)
	}
	if dataMap["name"] != "Alice" {
		t.Errorf("data.name = %v, want \"Alice\"", dataMap["name"])
	}
}

func TestEmbedNodeType(t *testing.T) {
	node := &EmbedNode{}
	if got := node.Type(); got != "ai.embed" {
		t.Errorf("Type() = %q, want %q", got, "ai.embed")
	}
}

func TestEmbedNodeExecute(t *testing.T) {
	mock := &mockClient{response: "[0.1, 0.2, 0.3]"}
	node := &EmbedNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"text": "hello world"}},
		},
	}

	config := map[string]interface{}{
		"input_field": "text",
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	embedding, ok := outputs[0].Items[0].JSON["embedding"]
	if !ok {
		t.Fatal("missing \"embedding\" key in output")
	}

	emb, ok := embedding.([]float64)
	if !ok {
		t.Fatalf("expected []float64, got %T", embedding)
	}
	if len(emb) != 3 {
		t.Fatalf("expected 3 floats, got %d", len(emb))
	}
	if emb[0] != 0.1 || emb[1] != 0.2 || emb[2] != 0.3 {
		t.Errorf("embedding = %v, want [0.1, 0.2, 0.3]", emb)
	}
}

func TestAgentNodeType(t *testing.T) {
	node := &AgentNode{}
	if got := node.Type(); got != "ai.agent" {
		t.Errorf("Type() = %q, want %q", got, "ai.agent")
	}
}

func TestAgentNodeExecute(t *testing.T) {
	mock := &mockClient{response: "I analyzed the data and found 3 key insights."}
	node := &AgentNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{
			{JSON: map[string]interface{}{"topic": "market trends"}},
		},
	}

	config := map[string]interface{}{
		"goal":      "Analyze {{$json.topic}} and provide insights",
		"max_steps": 3,
	}

	outputs, err := node.Execute(context.Background(), input, config)
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}

	if len(outputs) != 1 || outputs[0].Handle != "main" {
		t.Fatalf("expected 1 output with handle \"main\"")
	}

	item := outputs[0].Items[0]

	result, ok := item.JSON["agent_result"]
	if !ok {
		t.Fatal("missing \"agent_result\" key")
	}
	if result != "I analyzed the data and found 3 key insights." {
		t.Errorf("agent_result = %q", result)
	}

	steps, ok := item.JSON["steps_taken"]
	if !ok {
		t.Fatal("missing \"steps_taken\" key")
	}
	if steps != 1 {
		t.Errorf("steps_taken = %v, want 1", steps)
	}

	maxSteps, ok := item.JSON["max_steps"]
	if !ok {
		t.Fatal("missing \"max_steps\" key")
	}
	if maxSteps != 3 {
		t.Errorf("max_steps = %v, want 3", maxSteps)
	}

	// Original field should be preserved.
	if item.JSON["topic"] != "market trends" {
		t.Error("original \"topic\" field was modified")
	}
}

func TestAgentNodeMissingGoal(t *testing.T) {
	mock := &mockClient{response: "ignored"}
	node := &AgentNode{ClientOverride: mock}

	input := workflow.NodeInput{
		Items: []workflow.Item{{JSON: map[string]interface{}{}}},
	}

	_, err := node.Execute(context.Background(), input, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected error for missing goal, got nil")
	}
}

func TestExpandTemplate(t *testing.T) {
	item := workflow.Item{
		JSON: map[string]interface{}{
			"name":  "Alice",
			"age":   30,
			"email": "alice@example.com",
		},
	}

	tests := []struct {
		template string
		want     string
	}{
		{"Hello {{$json.name}}", "Hello Alice"},
		{"{{$json.name}} is {{$json.age}}", "Alice is 30"},
		{"No placeholders here", "No placeholders here"},
		{"Missing: {{$json.unknown}}", "Missing: {{$json.unknown}}"},
		{"Email: {{$json.email}}", "Email: alice@example.com"},
	}

	for _, tt := range tests {
		got := expandTemplate(tt.template, item)
		if got != tt.want {
			t.Errorf("expandTemplate(%q) = %q, want %q", tt.template, got, tt.want)
		}
	}
}

func TestConfigHelpers(t *testing.T) {
	config := map[string]interface{}{
		"str_val":   "hello",
		"float_val": 3.14,
		"int_val":   float64(42), // JSON numbers decode as float64
		"empty_str": "",
	}

	if got := configString(config, "str_val", "default"); got != "hello" {
		t.Errorf("configString = %q, want %q", got, "hello")
	}
	if got := configString(config, "missing", "default"); got != "default" {
		t.Errorf("configString missing = %q, want %q", got, "default")
	}
	if got := configString(config, "empty_str", "default"); got != "default" {
		t.Errorf("configString empty = %q, want %q", got, "default")
	}

	if got := configFloat(config, "float_val", 0.0); got != 3.14 {
		t.Errorf("configFloat = %v, want 3.14", got)
	}
	if got := configFloat(config, "missing", 1.0); got != 1.0 {
		t.Errorf("configFloat missing = %v, want 1.0", got)
	}

	if got := configInt(config, "int_val", 0); got != 42 {
		t.Errorf("configInt = %v, want 42", got)
	}
	if got := configInt(config, "missing", 10); got != 10 {
		t.Errorf("configInt missing = %v, want 10", got)
	}
}
