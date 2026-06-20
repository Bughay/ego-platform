package deepseek

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}
type Function struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

func LoadToolsFromFile(filename string) ([]Tool, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return LoadToolsFromData(data)
}

func LoadToolsFromData(data []byte) ([]Tool, error) {
	var tools []Tool
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}
	return tools, nil
}

func (t Tool) ToLLMString() string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Tool: %s\n", t.Function.Name))
	b.WriteString(fmt.Sprintf("Description: %s\n", t.Function.Description))

	if len(t.Function.Parameters.Properties) > 0 {
		b.WriteString("Parameters:\n")
		for paramName, prop := range t.Function.Parameters.Properties {
			required := ""
			for _, r := range t.Function.Parameters.Required {
				if r == paramName {
					required = " (required)"
					break
				}
			}
			b.WriteString(fmt.Sprintf("  - %s: %s%s\n", paramName, prop.Type, required))
			if prop.Description != "" {
				b.WriteString(fmt.Sprintf("    %s\n", prop.Description))
			}
		}
	}

	return b.String()
}

func ToolsToLLMString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read tools.json: %w", err)
	}
	return ToolsToLLMStringFromData(data)
}

func ToolsToLLMStringFromData(data []byte) (string, error) {
	tools, err := LoadToolsFromData(data)
	if err != nil {
		return "", err
	}
	var results []string
	for _, t := range tools {
		results = append(results, t.ToLLMString())
	}
	return strings.Join(results, "\n---\n"), nil
}
