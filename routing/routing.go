package routing

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/middleware"
	"github.com/goyourt/yogourt/services/providers"
)

const (
	defaultHost = "0.0.0.0"
	// productionMode is the config mode where a weak JWT secret is fatal
	// rather than merely reported.
	productionMode = "production"
)

// config holds the settings applied by the functional options of Initialize.
type config struct {
	authorizer *authorization.Engine
}

// Option configures routing.Initialize.
type Option func(*config)

// WithAuthorizer enables route authorization: the engine is published
// process-wide before middlewares and routes are loaded, every route method
// gets a permission — derived from its folder and its HTTP method by
// convention, or overridden by the Permissions map of the folder (including
// with authorization.Public) — and the framework inserts the RBAC middleware
// in front of each protected handler. The resolved surface is logged at boot.
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
	validateSecretKeyAtBoot(mainConfig.Security.SecretKey, mainConfig.Mode)
	r.Use(cors.New(buildCORSConfig(mainConfig)))

	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	if err := middleware.LoadMiddlewares(basePath); err != nil {
		log.Fatal("Error loading middlewares: ", err)
	}

	declared, err := loadAPIHandlers(r, apiFolder, cfg.authorizer)
	if err != nil {
		log.Fatal("Error loading handlers: ", err)
	}

	if cfg.authorizer != nil {
		// Every permission declared by the routes (plus the known list) is
		// registered with the provider: applications never insert permission
		// rows by hand. Additive only, fail-fast on a store failure.
		perms := append(declared, cfg.authorizer.KnownPermissions()...)
		if err := cfg.authorizer.SyncPermissions(context.Background(), perms); err != nil {
			log.Fatal("Error syncing permissions: ", err)
		}
	}

	serverConfig := mainConfig.Server
	if err := r.Run(listenAddress(serverConfig.Host, serverConfig.Port)); err != nil {
		log.Fatal("Error starting server: ", err)
	}
}

// validateSecretKeyAtBoot surfaces a misconfigured JWT secret at startup
// instead of letting every token operation fail at request time (AUTHZ-012).
// Outside production the problem is only logged: a development or test
// application must keep booting with a throwaway secret, and many do not use
// the token service at all. In production a short secret is fatal — it is the
// one place where booting with a guessable signing key is worse than not
// booting. The length rule is kept in sync with services.ValidateSecretKey
// (routing cannot import services — import cycle).
func validateSecretKeyAtBoot(secret, mode string) {
	const minSecretKeyLength = 32

	problem := ""
	switch {
	case secret == "":
		problem = "security.secret_key is empty"
	case len(secret) < minSecretKeyLength:
		problem = fmt.Sprintf("security.secret_key is too short (%d bytes, minimum %d)", len(secret), minSecretKeyLength)
	default:
		return
	}

	if strings.EqualFold(mode, productionMode) {
		log.Fatalf("%s: refusing to start in production mode", problem)
	}
	log.Printf("warning: %s; JWT features stay unusable until it is fixed, and production mode would refuse to start", problem)
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
