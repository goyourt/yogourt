package compiler

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const compiledRootFolder = ".yogourt"

func CompilePlugin(filePath string) (string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
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

	if upToDate(filePath, outPath, info.ModTime()) {
		return outPath, nil
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

func upToDate(src string, out string, srcMod time.Time) bool {
	outInfo, err := os.Stat(out)
	if err != nil {
		return false
	}

	if srcMod.After(outInfo.ModTime()) {
		return false
	}

	if isDir(src) {
		return !anyGoFileNewer(src, outInfo.ModTime())
	}

	return true
}

var errRebuild = errors.New("rebuild")

func anyGoFileNewer(dir string, soTime time.Time) bool {
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			if info, err := d.Info(); err == nil {
				if info.ModTime().After(soTime) {
					return errRebuild
				}
			}
		}
		return nil
	})

	return errors.Is(err, errRebuild)

}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
