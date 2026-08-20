package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const compiledRootFolder = ".yogourt"

// PluginPath returns the path of the compiled plugin expected for a source
// file: the source path, relative to the working directory, under
// compiledRootFolder and suffixed with ".so".
//
// Both paths have their symbolic links resolved before being compared.
// os.Getwd never returns a path containing a symlink, so a caller passing one
// — /var/... on macOS, where the real path is /private/var/... — used to get a
// relative path climbing out of the project ("../../../var/...") and a plugin
// silently reported as missing, instead of an error. routing.Initialize is
// unaffected, since it derives its folder from os.Getwd itself; any other
// caller, a CLI or a tool passing a path from elsewhere, is.
func PluginPath(filePath string) (string, error) {
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("file %s does not exist", filePath)
	}

	resolvedFile, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return "", err
	}

	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	resolvedPwd, err := filepath.EvalSymlinks(pwd)
	if err != nil {
		return "", err
	}

	relPath, err := filepath.Rel(resolvedPwd, resolvedFile)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relPath, "..") {
		// Outside the project: the plugin path would be meaningless, and a
		// silently wrong one is worse than a refusal.
		return "", fmt.Errorf("source file %s is outside the working directory %s", resolvedFile, resolvedPwd)
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
