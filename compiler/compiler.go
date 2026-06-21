package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const compiledRootFolder = ".yogourt"

func CompilePlugin(filePath string) (string, error) {
	if _, err := os.Stat(filePath); err != nil {
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

	outPath := filepath.Join(compiledRootFolder, relPath+".so")

	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}

	fmt.Println("🔨 Compiling plugin:", outPath)
	cmd := exec.Command(
		"go", "build",
		"-buildmode=plugin",
		"-o", outPath,
		filePath,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("error compiling plugin: %w", err)
	}

	fmt.Println("✅ Successfully compiled:", outPath)
	return outPath, nil
}
