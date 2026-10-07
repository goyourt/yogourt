package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A caller may hand over a path whose parents are symlinked — on macOS
// t.TempDir() returns /var/folders/... whose real path is /private/var/... —
// while os.Getwd never contains a symlink. Comparing them lexically produced a
// relative path climbing out of the project, and the plugin was then reported
// as simply missing: a wrong answer instead of an error.
func TestPluginPathResolvesSymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "api", "widgets")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "route.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The working directory is the resolved root; the caller passes the
	// unresolved one.
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(resolved)

	got, err := PluginPath(file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(".yogourt", "api", "widgets", "route.go.so")
	if got != want {
		t.Errorf("PluginPath = %q, want %q", got, want)
	}
}

func TestPluginPathRefusesSourceOutsideWorkingDirectory(t *testing.T) {
	outside := t.TempDir()
	file := filepath.Join(outside, "route.go")
	if err := os.WriteFile(file, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())

	_, err := PluginPath(file)
	if err == nil {
		t.Fatal("a source outside the working directory must be refused, not turned into a climbing path")
	}
	if !strings.Contains(err.Error(), "outside the working directory") {
		t.Errorf("unexpected error: %v", err)
	}
}
