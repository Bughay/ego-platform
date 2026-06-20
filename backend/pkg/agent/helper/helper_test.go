package helper

import (
	"os"
	"testing"
)

func TestEditFile_Success(t *testing.T) {
	path := tmpFile(t, "hello world")

	result, err := EditFile(path, "hello", "goodbye")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty success message")
	}

	assertFileContent(t, path, "goodbye world")
}

func TestEditFile_NotFound(t *testing.T) {
	path := tmpFile(t, "hello world")
	_, err := EditFile(path, "nothere", "x")
	if err == nil {
		t.Error("expected error for missing search string")
	}
}

func TestEditFile_Ambiguous(t *testing.T) {
	path := tmpFile(t, "foo bar foo")
	_, err := EditFile(path, "foo", "baz")
	if err == nil {
		t.Error("expected error for ambiguous match")
	}
}

func TestEditFile_EmptyPath(t *testing.T) {
	_, err := EditFile("", "search", "replace")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestEditFile_EmptySearch(t *testing.T) {
	path := tmpFile(t, "hello")
	_, err := EditFile(path, "", "replace")
	if err == nil {
		t.Error("expected error for empty search string")
	}
}

func TestEnsureFile_CreatesWhenMissing(t *testing.T) {
	path := t.TempDir() + "/new.txt"
	if err := EnsureFile(path, "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFileContent(t, path, "default")
}

func TestEnsureFile_SkipsWhenExists(t *testing.T) {
	path := tmpFile(t, "existing content")
	if err := EnsureFile(path, "default"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// file should be unchanged
	assertFileContent(t, path, "existing content")
}

func TestWriteToFile(t *testing.T) {
	path := t.TempDir() + "/out.txt"
	result, err := WriteToFile(path, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	assertFileContent(t, path, "hello")
}

func TestAppendToFile(t *testing.T) {
	path := tmpFile(t, "hello")
	if _, err := AppendToFile(path, " world"); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "hello world")
}

func tmpFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != want {
		t.Errorf("file content: got %q, want %q", string(got), want)
	}
}
