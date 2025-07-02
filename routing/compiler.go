package routing

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func compilePlugin(filePath string) (map[string]interface{}, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file %s does not exist", filePath)
	}

	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(pwd, filePath)
	if err != nil {
		return nil, err
	}
	newPath := ".yogourt/" + relPath + ".so"
	fmt.Println("Compiling plugin: ", newPath)

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", newPath, filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("error compiling plugin: %v", err)
	}

	fmt.Println("Successfully compiled plugin:", newPath)
	return loadPackage(newPath)
}
