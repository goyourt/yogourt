package routing

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/middleware"
	"github.com/goyourt/yogourt/services/providers"
)

const defaultHost = "0.0.0.0"

// config holds the settings applied by the functional options of Initialize.
type config struct {
	authorizer *authorization.Engine
}

// Option configures routing.Initialize.
type Option func(*config)

// WithAuthorizer enables route authorization: the engine is published
// process-wide before middlewares and routes are loaded, every route must
// then declare its permissions (or authorization.Public) and the framework
// inserts the RBAC middleware in front of each protected handler.
func WithAuthorizer(engine *authorization.Engine) Option {
	return func(cfg *config) {
		cfg.authorizer = engine
	}
}

func Initialize(apiFolder string, options ...Option) {
	cfg := &config{}
	for _, option := range options {
		option(cfg)
	}

	basePath, err := os.Getwd()
	if err != nil {
		log.Fatal("Error resolving working directory: ", err)
	}
	apiFolder = filepath.Join(basePath, apiFolder)

	if _, err := os.Stat(apiFolder); err != nil {
		log.Fatal("API folder not found at "+apiFolder+": ", err)
	}

	if cfg.authorizer != nil {
		// The engine must be visible process-wide before middlewares and
		// routes are loaded (D3).
		if err := authorization.Publish(cfg.authorizer); err != nil {
			log.Fatal("Error publishing authorizer: ", err)
		}
	}

	r := gin.Default()

	mainConfig := providers.GetMainConfig()
	validateSecretKeyAtBoot(mainConfig.Security.SecretKey)
	r.Use(cors.New(buildCORSConfig(mainConfig)))

	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	if err := middleware.LoadMiddlewares(basePath); err != nil {
		log.Fatal("Error loading middlewares: ", err)
	}

	if err := loadAPIHandlers(r, apiFolder, cfg.authorizer); err != nil {
		log.Fatal("Error loading handlers: ", err)
	}

	serverConfig := mainConfig.Server
	if err := r.Run(listenAddress(serverConfig.Host, serverConfig.Port)); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}

// validateSecretKeyAtBoot fails fast on a misconfigured JWT secret instead of
// letting every token operation fail at request time (AUTHZ-012). An empty
// secret only logs a warning: applications that do not use the token service
// must keep booting. The length rule is kept in sync with
// services.ValidateSecretKey (routing cannot import services — import cycle).
func validateSecretKeyAtBoot(secret string) {
	const minSecretKeyLength = 32
	if secret == "" {
		log.Printf("warning: security.secret_key is empty; JWT features are unusable until it is configured")
		return
	}
	if len(secret) < minSecretKeyLength {
		log.Fatalf("security.secret_key is too short (%d bytes): it must be at least %d bytes", len(secret), minSecretKeyLength)
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
