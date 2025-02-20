package routing

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"strings"
)

func Initialize(apiFolder string) {
	basePath, err := os.Getwd()

	if err != nil {
		fmt.Println(err)
		return
	}

	compiledApiFolder := basePath + "/.yogourt/"

	basePath += "/"

	if apiFolder[0] != '/' {
		compiledApiFolder += apiFolder
		apiFolder = basePath + apiFolder
	}

	if apiFolder == "" {
		apiFolder = basePath + "api"
		compiledApiFolder += "api"
	}

	if _, err := os.Stat(apiFolder); os.IsNotExist(err) {
		fmt.Println("API folder not found at " + apiFolder)
		return
	}

	r := gin.Default()

	if err = loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers:", err)
		return
	}

	r.Run(":8080")

}

func compilePlugin(filePath string) (map[string]interface{}, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("le fichier %s n'existe pas", filePath)
	}
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(pwd, filePath)
	if err != nil {
		return nil, err
	}
	newPath := ".yogourt/" + relPath + ".so"
	fmt.Println("Compiling plugin: ", newPath)

	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", newPath, filePath)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la compilation du plugin: %v", err)
	}

	fmt.Println("Compilation réussie du plugin:", newPath)

	routeHandlerMap, err := loadPackage(newPath)
	if err != nil {
		return nil, fmt.Errorf("erreur lors du chargement du plugin: %v", err)
	}
	return routeHandlerMap, nil
}

func loadPackage(path string) (map[string]interface{}, error) {
	plg, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open plugin: %v", err)
	}
	var routeHandler map[string]interface{}
	routeHandler = make(map[string]interface{})

	getHandler, err := plg.Lookup("GET")
	if err == nil {
		routeHandler["GET"] = getHandler
	}

	putHandler, err := plg.Lookup("PUT")
	if err == nil {
		routeHandler["PUT"] = putHandler
	}

	postHandler, err := plg.Lookup("POST")
	if err == nil {
		routeHandler["POST"] = postHandler
	}

	patchHandler, err := plg.Lookup("PATCH")
	if err == nil {
		routeHandler["PATCH"] = patchHandler
	}

	deleteHandler, err := plg.Lookup("DELETE")
	if err == nil {
		routeHandler["DELETE"] = deleteHandler
	}

	return routeHandler, nil
}

func loadAPIHandlers(r *gin.Engine, basePath string) error {
	return filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == ".yogourt" {
			fmt.Printf("skipping a dir without errors: %+v \n", info.Name())
			return filepath.SkipDir
		}

		if strings.HasSuffix(info.Name(), ".go") {
			pluginPath := path + ".so"
			routeHandler, err := compilePlugin(path)

			if err != nil {
				return fmt.Errorf("error loading or compiling package from %s: %v", pluginPath, err)
			}

			if routeHandler == nil {
				return nil
			}

			routePath := "/api" + path[len(basePath):len(path)-len(info.Name())]
			for protocol, handlerFunc := range routeHandler {
				r.Handle(protocol, routePath, handlerFunc.(func(*gin.Context)))
			}
		}
		return nil
	})
}
