package routing

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/middleware"
	"log"
	"os"
	"path/filepath"
)

const compiledRootFolder = ".yogourt"

func Initialize(apiFolder string) {
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

	err = middleware.LoadMiddlewares(basePath, compiledRootFolder)
	if err != nil {
		log.Fatal("Error loading middlewares: ", err)
		return
	}
	if err = loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers: ", err)
		return
	}

	r.Run(":8080")
}
