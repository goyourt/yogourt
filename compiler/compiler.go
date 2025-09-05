package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const compiledRootFolder = ".yogourt"

func CompilePlugin(filePath string) (string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file %s does not exist", filePath)
	}

	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(pwd, filePath)
	if err != nil {
		return "", err
	}
	newPath := compiledRootFolder + "/" + relPath + ".so"
	fmt.Println("Compiling plugin: ", newPath)

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", newPath, filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error compiling plugin: %v", err)
	}

	fmt.Println("Successfully compiled plugin:", newPath)
	return newPath, nil
}
