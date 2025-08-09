package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strings"
)

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
		} else if middlewares["^"+route] != nil { // clause to ignore previous middlewares
			middlewareList = append(make([]gin.HandlerFunc, 0), middlewares["^"+route])
		}
	}

	return middlewareList
}

func LoadMiddlewares(basePath string, compiledRootFolder string) error {
	filePath := basePath + "/middleware/middleware.go"

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Println("Middleware file not found at " + filePath)
		return nil
	}
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Error getting current working directory: %v", err)
	}

	relPath, err := filepath.Rel(pwd, filePath)
	if err != nil {
		return fmt.Errorf("Error getting relative path: %v", err)
	}
	newPath := compiledRootFolder + "/" + relPath + ".so"

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", newPath, filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("Error running command: %v", err)
	}

	plg, err := plugin.Open(newPath)
	if err != nil {
		return fmt.Errorf("Error opening plugin: %v", err)
	}

	callbacks, err := plg.Lookup("Callbacks")
	if err != nil {
		return fmt.Errorf("Error no Callbacks: %v", err)
	}

	middlewares = *callbacks.(*map[string]func(*gin.Context))

	return nil
}
