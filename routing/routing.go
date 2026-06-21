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

const defaultHost = "0.0.0.0"

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

	corsConfig := providers.GetMainConfig().CORS

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

	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	if err := middleware.LoadMiddlewares(basePath); err != nil {
		log.Fatal("Error loading middlewares: ", err)
	}

	if err := loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers: ", err)
	}

	serverConfig := providers.GetMainConfig().Server
	//host := serverConfig.Host
	//if host == "" {
	//	host = defaultHost
	//}
	host := defaultHost

	r.Run(host + ":" + strconv.Itoa(serverConfig.Port))
}
