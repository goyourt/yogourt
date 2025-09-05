package test

import (
	"os"
	"testing"

	"github.com/goyourt/yogourt/compiler"
)

func TestLoader(t *testing.T) {
	basePath, _ := os.Getwd()
	fileToCompile := basePath + "/TestFilesToCompile/FileToCompile.go"
	newPath, _ := compiler.CompilePlugin(fileToCompile)

	functions, err := compiler.LoadFunctions(newPath, []string{"Test"})
	if err != nil {
		t.Errorf("Error loading functions: %v", err)
	}
	if functions["Test"] == nil {
		t.Errorf("Function 'Test' not found")
		return
	}
	function := functions["Test"].(func() string)
	if function() != "success" {
		t.Errorf("Loaded function should return 'success', returned  %v", function())
	}

	os.RemoveAll("./.yogourt")
}
