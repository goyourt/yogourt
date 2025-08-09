package routing

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/binary"
	"github.com/goyourt/yogourt/middleware"
)

const compiledRootFolder = ".yogourt"

func Initialize(apiFolder string, port string) {
	basePath, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
		return
	}
	apiFolder = filepath.Join(basePath, apiFolder)

	if _, err := os.Stat(apiFolder); os.IsNotExist(err) {
		fmt.Println("API folder not found at " + apiFolder)
		return
	}

	r := gin.Default()

	err = middleware.LoadMiddlewares(basePath)
	if err != nil {
		log.Fatal("Error loading middlewares: ", err)
		return
	}
	if err = loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers: ", err)
		return
	}

	r.Run(port)
}

func loadAPIHandlers(r *gin.Engine, basePath string) error {
	return filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(info.Name(), ".go") {
			newPath, err := binary.CompilePlugin(path)
			if err != nil {
				return fmt.Errorf("error loading or compiling package from %s: %v", path, err)
			}

			routeHandler, err := binary.LoadFunctions(newPath, []string{"GET", "PUT", "POST", "PATCH", "DELETE"})
			if err != nil {
				return err
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
