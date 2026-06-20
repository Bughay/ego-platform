package tools

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed frontend_executer.json
var SchemaJSON []byte

// FileFunctions returns tool handlers that operate on files in the current directory.
func FileFunctions() map[string]func(string) (string, error) {
	return FileFunctionsWithPath(".")
}

// FileFunctionsWithPath returns tool handlers rooted at basePath, allowing the
// agent to operate on projects in a directory other than the working directory.
func FileFunctionsWithPath(basePath string) map[string]func(string) (string, error) {
	read := func(toolName, filename string) func(string) (string, error) {
		return func(_ string) (string, error) {
			path := filepath.Join(basePath, filename)
			content, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("%s: read %s: %v", toolName, path, err)
			}
			slog.Info("tool called", "tool", toolName)
			return string(content), nil
		}
	}

	write := func(toolName, filename string) func(string) (string, error) {
		return func(args string) (string, error) {
			path := filepath.Join(basePath, filename)
			if dir := filepath.Dir(path); dir != "." {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return "", fmt.Errorf("%s: mkdir %s: %v", toolName, dir, err)
				}
			}
			if err := os.WriteFile(path, []byte(args), 0644); err != nil {
				return "", fmt.Errorf("%s: write %s: %v", toolName, path, err)
			}
			slog.Info("tool called", "tool", toolName, "bytes", len(args))
			return fmt.Sprintf("successfully wrote %d bytes to %s", len(args), path), nil
		}
	}

	return map[string]func(string) (string, error){
		"analyze_plan": read("analyze_plan", "plan.md"),
		"analyze_html": read("analyze_html", "index.html"),
		"analyze_css":  read("analyze_css", "styles.css"),
		"analyze_js":   read("analyze_js", "script.js"),
		"update_html":  write("update_html", "index.html"),
		"update_css":   write("update_css", "styles.css"),
		"update_js":    write("update_js", "script.js"),
	}
}
