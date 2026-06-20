package deepseek

import (
	"os"
	"strings"
	"testing"
)

func TestLoadToolsFromFile(t *testing.T) {
	schema := `[{"type":"function","function":{"name":"test_tool","description":"a test tool","parameters":{"type":"object","properties":{"arg1":{"type":"string","description":"first arg"}},"required":["arg1"]}}}]`
	path := tmpJSONFile(t, schema)

	tools, err := LoadToolsFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Function.Name != "test_tool" {
		t.Errorf("expected 'test_tool', got %q", tools[0].Function.Name)
	}
}

func TestLoadToolsFromFile_Missing(t *testing.T) {
	_, err := LoadToolsFromFile("/nonexistent/path/tools.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadToolsFromFile_InvalidJSON(t *testing.T) {
	path := tmpJSONFile(t, `not valid json`)
	_, err := LoadToolsFromFile(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestToolToLLMString(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: Function{
			Name:        "my_tool",
			Description: "does something useful",
			Parameters: Parameters{
				Type: "object",
				Properties: map[string]Property{
					"arg1": {Type: "string", Description: "first argument"},
				},
				Required: []string{"arg1"},
			},
		},
	}

	result := tool.ToLLMString()
	for _, want := range []string{"my_tool", "arg1", "required", "does something useful"} {
		if !strings.Contains(result, want) {
			t.Errorf("expected %q in LLM string output", want)
		}
	}
}

func TestToolsToLLMString(t *testing.T) {
	schema := `[
		{"type":"function","function":{"name":"tool_a","description":"tool a","parameters":{"type":"object","properties":{},"required":[]}}},
		{"type":"function","function":{"name":"tool_b","description":"tool b","parameters":{"type":"object","properties":{},"required":[]}}}
	]`
	path := tmpJSONFile(t, schema)

	result, err := ToolsToLLMString(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "tool_a") || !strings.Contains(result, "tool_b") {
		t.Error("expected both tool names in output")
	}
	if !strings.Contains(result, "---") {
		t.Error("expected separator between tools")
	}
}

func TestLoadToolsFromData(t *testing.T) {
	schema := `[{"type":"function","function":{"name":"data_tool","description":"loaded from bytes","parameters":{"type":"object","properties":{},"required":[]}}}]`
	tools, err := LoadToolsFromData([]byte(schema))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 || tools[0].Function.Name != "data_tool" {
		t.Errorf("unexpected tools: %+v", tools)
	}
}

func TestToolsToLLMStringFromData(t *testing.T) {
	schema := `[{"type":"function","function":{"name":"inline_tool","description":"inline desc","parameters":{"type":"object","properties":{},"required":[]}}}]`
	result, err := ToolsToLLMStringFromData([]byte(schema))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "inline_tool") {
		t.Error("expected tool name in output")
	}
}

func tmpJSONFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "tools-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
