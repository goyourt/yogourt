package compiler

import (
	"fmt"
	"os"
	"path/filepath"
)

const compiledRootFolder = ".yogourt"

func PluginPath(filePath string) (string, error) {
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

	return filepath.Join(compiledRootFolder, relPath+".so"), nil
}

func ResolvePlugin(filePath string) (string, error) {
	outPath, err := PluginPath(filePath)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	return "", fmt.Errorf("compiled plugin %s does not exist", outPath)
}
