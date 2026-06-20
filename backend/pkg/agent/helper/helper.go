package helper

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

var envOnce sync.Once

// LoadAPIKey loads .env once, then reads the named environment variable.
func LoadAPIKey(envVarName string) (string, error) {
	envOnce.Do(func() {
		if err := godotenv.Load(); err != nil {
			slog.Warn("no .env file found, reading from environment")
		}
	})
	key := os.Getenv(envVarName)
	if key == "" {
		return "", fmt.Errorf("%s not set", envVarName)
	}
	return key, nil
}

// EnsureAPIKey verifies the named API key env var is present.
func EnsureAPIKey(envVarName string) error {
	_, err := LoadAPIKey(envVarName)
	return err
}

// ReadErrorBody reads an HTTP error response body for use in error messages.
func ReadErrorBody(r io.Reader) string {
	b, err := io.ReadAll(r)
	if err != nil {
		return "(could not read response body)"
	}
	return string(b)
}

// stdin is a single shared reader so repeated Input calls don't each create a
// new bufio.Reader. A fresh reader per call would discard any bytes the previous
// reader buffered past the newline (which breaks piped/scripted input entirely).
var stdin = bufio.NewReader(os.Stdin)

func Input(prompt string) (string, error) {
	fmt.Print(prompt)
	text, err := stdin.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

func ViewFiles(fileList []string) (string, error) {
	var result strings.Builder
	for _, filename := range fileList {
		content, err := os.ReadFile(filename)
		if err != nil {
			return "", fmt.Errorf("view_files: read %s: %v", filename, err)
		}
		result.WriteString(fmt.Sprintf("=== %s ===\n", filename))
		result.WriteString(string(content))
		result.WriteString("\n\n")
	}
	slog.Info("view_files called", "count", len(fileList))
	return result.String(), nil
}

// EnsureFile creates path with defaultContent if the file does not already exist.
func EnsureFile(path, defaultContent string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		slog.Info("creating missing file", "path", path)
		if err := os.WriteFile(path, []byte(defaultContent), 0644); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	return nil
}

func WriteToFile(path string, content string) (string, error) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write_file: write %s: %w", path, err)
	}
	return fmt.Sprintf("successfully wrote %d bytes to %s", len(content), path), nil
}

func AppendToFile(path string, content string) (string, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("append_file: open %s: %w", path, err)
	}
	defer f.Close()

	n, err := f.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("append_file: write %s: %w", path, err)
	}
	return fmt.Sprintf("successfully appended %d bytes to %s", n, path), nil
}

func EditFile(path, search, replace string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("editFile: empty file path")
	}
	if search == "" {
		return "", fmt.Errorf("editFile: search block cannot be empty")
	}

	original, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("editFile: file does not exist: %s", path)
		}
		return "", fmt.Errorf("editFile: read %s: %v", path, err)
	}
	content := string(original)

	count := strings.Count(content, search)
	if count == 0 {
		return "", fmt.Errorf("editFile: search block not found (exact match required) in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("editFile: ambiguous — search block appears %d times in %s; add more context lines", count, path)
	}

	newContent := strings.Replace(content, search, replace, 1)

	// Atomic write: write to a temp file then rename to avoid partial writes.
	tmpFile, err := os.CreateTemp(filepath.Dir(path), ".edit-*")
	if err != nil {
		return "", fmt.Errorf("editFile: create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(newContent); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("editFile: write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("editFile: close temp file: %v", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("editFile: rename: %v", err)
	}

	return fmt.Sprintf("SUCCESS: replaced 1 occurrence in %s", path), nil
}
