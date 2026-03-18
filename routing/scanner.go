package routing

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goyourt/yogourt/compiler"
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

	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = compiler.SlugRouteFormater(part)
	}
	rel = strings.Join(parts, "/")

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
