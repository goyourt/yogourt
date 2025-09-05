package test

import (
	"os"
	"testing"

	"github.com/goyourt/yogourt/compiler"
)

func TestCompiler(t *testing.T) {
	basePath, _ := os.Getwd()
	fileToCompile := basePath + "/TestFilesToCompile/FileToCompile.go"
	exceptedDestination := ".yogourt/TestFilesToCompile/FileToCompile.go.so"
	newPath, err := compiler.CompilePlugin(fileToCompile)

	if err != nil {
		t.Errorf("Error compiling plugin: %v", err)
	}
	if newPath != exceptedDestination {
		t.Errorf("Compiled path is not as expected. Got %s, excepted %s", newPath, exceptedDestination)
	}
	os.RemoveAll("./.yogourt")
}
