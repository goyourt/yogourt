package middleware

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/binary"
)

const middlewaresPath = "/middleware/middleware.go"
const ignoreChar = "^"

var middlewares map[string]func(*gin.Context)

func GetMiddleware(path string) []gin.HandlerFunc {
	var middlewareList []gin.HandlerFunc
	subroutes := strings.Split(path, "/")
	for i := -1; i <= len(subroutes); i++ {
		route := "/"
		if i >= 0 {
			route = strings.Join(subroutes[:i], "/")
		}
		value, keyExists := middlewares[route]
		if route != "" && value != nil {
			middlewareList = append(middlewareList, value)
		} else if value == nil && keyExists { // delete all previous middlewares
			middlewareList = make([]gin.HandlerFunc, 0)
		} else if middlewares[ignoreChar+route] != nil { // clause to ignore previous middlewares
			middlewareList = append(make([]gin.HandlerFunc, 0), middlewares[ignoreChar+route])
		}
	}

	return middlewareList
}

func LoadMiddlewares(basePath string) error {
	filePath := basePath + middlewaresPath
	newPath, err := binary.CompilePlugin(filePath)
	if err != nil {
		return fmt.Errorf("Error compiling middleware plugin: %v", err)
	}

	callbacks, err := binary.LoadFunctions(newPath, []string{"Callbacks"})

	if err != nil {
		return err
	}

	middlewares = *callbacks["Callbacks"].(*map[string]func(*gin.Context))

	return nil
}
