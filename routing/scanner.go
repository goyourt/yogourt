package routing

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/goyourt/yogourt/compiler"
)

func isGoFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func routePathFor(basePath, fullPath string) string {
	rel, err := filepath.Rel(basePath, fullPath)
	if err != nil {
		rel = strings.TrimPrefix(fullPath, basePath)
	}

	rel = filepath.Dir(rel)
	if rel == "." || rel == "" {
		return "/api"
	}

	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = compiler.SlugRouteFormater(part)
	}

	return "/api/" + strings.Join(parts, "/")
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
