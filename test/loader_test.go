package test

import (
	"os"
	"testing"

	"github.com/goyourt/yogourt/binary"
)

func TestLoader(t *testing.T) {
	basePath, _ := os.Getwd()
	fileToCompile := basePath + "/TestFilesToCompile/FileToCompile.go"
	newPath, _ := binary.CompilePlugin(fileToCompile)

	functions, err := binary.LoadFunctions(newPath, []string{"Test"})
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
