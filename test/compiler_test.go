package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goyourt/yogourt/compiler"
)

func TestPluginPath(t *testing.T) {
	basePath, _ := os.Getwd()
	fileToCompile := basePath + "/TestFilesToCompile/FileToCompile.go"
	exceptedDestination := ".yogourt/TestFilesToCompile/FileToCompile.go.so"
	newPath, err := compiler.PluginPath(fileToCompile)

	if err != nil {
		t.Errorf("Error resolving plugin path: %v", err)
	}
	if newPath != exceptedDestination {
		t.Errorf("Plugin path is not as expected. Got %s, excepted %s", newPath, exceptedDestination)
	}
}

func TestResolvePlugin(t *testing.T) {
	basePath, _ := os.Getwd()
	fileToCompile := basePath + "/TestFilesToCompile/FileToCompile.go"
	pluginPath := filepath.Join(".yogourt", "TestFilesToCompile", "FileToCompile.go.so")

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		t.Fatalf("Error creating plugin folder: %v", err)
	}
	if err := os.WriteFile(pluginPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("Error creating placeholder plugin: %v", err)
	}
	defer os.RemoveAll("./.yogourt")

	resolvedPath, err := compiler.ResolvePlugin(fileToCompile)
	if err != nil {
		t.Errorf("Error resolving plugin: %v", err)
	}
	if resolvedPath != pluginPath {
		t.Errorf("Resolved path is not as expected. Got %s, expected %s", resolvedPath, pluginPath)
	}
}
