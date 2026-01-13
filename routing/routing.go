package routing

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/middleware"
	"github.com/goyourt/yogourt/services/providers"
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

	corsConfig := providers.GetConfig().CORS

	if len(corsConfig.AllowedOrigins) == 0 && !corsConfig.AllowAllOrigins {
		corsConfig.AllowAllOrigins = true
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     corsConfig.AllowedOrigins,
		AllowMethods:     corsConfig.AllowedMethods,
		AllowHeaders:     corsConfig.AllowedHeaders,
		AllowCredentials: corsConfig.AllowCredentials,
		MaxAge:           corsConfig.MaxAge * time.Hour,
	}))

	err = middleware.LoadMiddlewares(basePath)
	if err != nil {
		log.Fatal("Error loading middlewares: ", err)
		return
	}
	if err = loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers: ", err)
		return
	}

	r.Run("0.0.0.0:" + strconv.Itoa(providers.GetConfig().Server.Port))
}
