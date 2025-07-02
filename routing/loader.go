package routing

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/middleware"
	"os"
	"path/filepath"
	"plugin"
	"strings"
)

func loadPackage(path string) (map[string]interface{}, error) {
	plg, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open plugin: %v", err)
	}

	routeHandler := make(map[string]interface{})
	for _, method := range []string{"GET", "PUT", "POST", "PATCH", "DELETE"} {
		if handler, err := plg.Lookup(method); err == nil {
			routeHandler[method] = handler
		}
	}

	return routeHandler, nil
}

func loadAPIHandlers(r *gin.Engine, basePath string) error {
	return filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".go") {
			routeHandler, err := compilePlugin(path)
			if err != nil {
				return fmt.Errorf("error loading or compiling package from %s: %v", path, err)
			}

			routePath := "/api" + path[len(basePath):len(path)-len(info.Name())]
			routeMiddlewares := middleware.GetMiddleware(routePath)
			for protocol, handlerFunc := range routeHandler {
				routeMiddlewares = append(routeMiddlewares, handlerFunc.(func(*gin.Context)))
				r.Handle(protocol, routePath, routeMiddlewares...)
			}
		}
		return nil
	})
}
