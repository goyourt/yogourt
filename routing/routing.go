package routing

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"os"
	"path/filepath"
)

func Initialize(apiFolder string, middlewares map[string]func(*gin.Context)) {
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

	if err = loadAPIHandlers(r, apiFolder, middlewares); err != nil {
		log.Fatal("Error loading handlers:", err)
		return
	}

	r.Run(":8080")
}
