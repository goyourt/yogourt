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

// routePathFor builds the Gin path of a route file: the prefix, then the
// folders between basePath and the file, each converted to its Gin form. The
// file name never appears in the URL.
//
// prefix is expected normalized (see normalizePrefix): either empty — routes
// are served at the root — or "/" followed by segments without a trailing
// slash.
func routePathFor(prefix, basePath, fullPath string) string {
	rel, err := filepath.Rel(basePath, fullPath)
	if err != nil {
		rel = strings.TrimPrefix(fullPath, basePath)
	}

	rel = filepath.Dir(rel)
	if rel == "." || rel == "" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}

	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		parts[i] = compiler.SlugRouteFormater(part)
	}

	return prefix + "/" + strings.Join(parts, "/")
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
