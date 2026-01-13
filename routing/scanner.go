package routing

import (
	"os"
	"path/filepath"
	"strings"
)

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func routePathFor(basePath, fullPath, fileName string) string {
	rel := fullPath[len(basePath) : len(fullPath)-len(fileName)]
	if rel == "" {
		return "/api"
	}
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	if strings.HasSuffix(rel, "/") {
		rel = rel[:len(rel)-1]
	}

	rel = strings.ReplaceAll(rel, "/_", "/:")

	return "/api" + rel
}

func walkGoFiles(basePath string) ([]string, error) {
	var files []string
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if isGoFile(info.Name()) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
