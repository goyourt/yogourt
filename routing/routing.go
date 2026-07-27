package routing

import (
	"fmt"
	"log"
	"net"
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

	mainConfig := providers.GetMainConfig()
	r.Use(cors.New(buildCORSConfig(mainConfig)))

	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	if err := middleware.LoadMiddlewares(basePath); err != nil {
		log.Fatal("Error loading middlewares: ", err)
	}

	if err := loadAPIHandlers(r, apiFolder); err != nil {
		log.Fatal("Error loading handlers: ", err)
	}

	serverConfig := mainConfig.Server
	if err := r.Run(listenAddress(serverConfig.Host, serverConfig.Port)); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}

func buildCORSConfig(mainConfig *providers.MainConfig) cors.Config {
	config := mainConfig.CORS
	allowAllOrigins := config.AllowAllOrigins
	if len(config.AllowedOrigins) == 0 && !allowAllOrigins {
		allowAllOrigins = true
	}
	allowedOrigins := config.AllowedOrigins
	if allowAllOrigins {
		allowedOrigins = nil
	}

	return cors.Config{
		AllowAllOrigins:  allowAllOrigins,
		AllowOrigins:     allowedOrigins,
		AllowMethods:     config.AllowedMethods,
		AllowHeaders:     config.AllowedHeaders,
		AllowCredentials: config.AllowCredentials,
		MaxAge:           config.MaxAge * time.Hour,
	}
}

func listenAddress(host string, port int) string {
	if host == "" {
		host = defaultHost
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
