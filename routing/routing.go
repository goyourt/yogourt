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

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/middleware"
	"github.com/goyourt/yogourt/services/providers"
)

const (
	defaultHost = "0.0.0.0"
	// productionMode is the config mode where a weak JWT secret is fatal
	// rather than merely reported, and where Gin runs in release mode.
	productionMode = "production"

	// testConfigMode maps the config mode to Gin's own test mode.
	testConfigMode = "test"

	// DefaultPrefix is the HTTP prefix every route is published under when
	// neither WithPrefix nor server.base_path names another one.
	DefaultPrefix = "/api"
)

// config holds the settings applied by the functional options of Initialize.
type config struct {
	authorizer *authorization.Engine
	// prefix is the raw value given to WithPrefix. Empty means the option was
	// not used, so server.base_path — then DefaultPrefix — decides.
	prefix string
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

// WithPrefix publishes every route under prefix instead of the default
// "/api". It wins over server.base_path of the configuration, so a program can
// force its own mount point. WithPrefix("/") serves the tree at the root:
// api/users/route.go then answers on "/users".
//
// A prefix names a mount point, never a resource: it is excluded from the
// permissions derived by convention, exactly as "/api" was.
func WithPrefix(prefix string) Option {
	return func(cfg *config) {
		cfg.prefix = prefix
	}
}

// Initialize loads the route tree of apiFolder, wires the middlewares and
// serves. The path is relative to the working directory and never reaches the
// HTTP prefix: the URL of a route only comes from its position in the tree.
func Initialize(apiFolder string, options ...Option) {
	cfg := &config{}
	for _, option := range options {
		option(cfg)
	}

	basePath, err := os.Getwd()
	if err != nil {
		log.Fatal("Error resolving working directory: ", err)
	}

	mainConfig := providers.GetMainConfig()

	prefix, err := resolveAPIPrefix(cfg.prefix, mainConfig)
	if err != nil {
		log.Fatal(err)
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

	validateSecretKeyAtBoot(mainConfig.Security.SecretKey, mainConfig.Mode)

	// Before gin.Default(): Gin logs its mode as it builds the engine, so
	// setting it afterwards would leave a log line contradicting reality.
	applyGinMode(mainConfig.Mode)

	r := gin.Default()
	r.Use(cors.New(buildCORSConfig(mainConfig)))

	log.Printf("Serving the routes of %s under %s", apiFolder, displayPrefix(prefix))

	if err := middleware.LoadMiddlewares(basePath); err != nil {
		log.Fatal("Error loading middlewares: ", err)
	}

	declared, err := loadAPIHandlers(r, prefix, apiFolder, cfg.authorizer)
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

// resolveAPIPrefix picks the HTTP prefix of the whole route tree: WithPrefix
// wins, then server.base_path of the configuration, then DefaultPrefix. The
// result is normalized — "" for the root, "/segment…" otherwise — so no caller
// has to deal with a trailing slash.
func resolveAPIPrefix(option string, mainConfig *providers.MainConfig) (string, error) {
	raw, source := strings.TrimSpace(option), "routing.WithPrefix"
	if raw == "" {
		raw, source = strings.TrimSpace(mainConfig.Server.BasePath), "server.base_path"
	}
	if raw == "" {
		return DefaultPrefix, nil
	}

	prefix, err := normalizePrefix(raw)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", source, err)
	}

	return prefix, nil
}

// normalizePrefix turns a written prefix into the form the route builder
// expects: a leading slash, no trailing one, and "" for the root. A prefix is
// a static mount point, so a Gin parameter or a space in it is a mistake worth
// a boot failure instead of a route tree nobody can reach.
func normalizePrefix(raw string) (string, error) {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", nil
	}

	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if segment == "" {
			return "", fmt.Errorf("prefix %q holds an empty segment", raw)
		}
		if strings.ContainsAny(segment, ":*") || strings.ContainsAny(segment, " \t") {
			return "", fmt.Errorf("prefix %q: the segment %q cannot hold a Gin parameter or a space", raw, segment)
		}
	}

	return "/" + strings.Join(segments, "/"), nil
}

// displayPrefix renders a normalized prefix for a human: the root prefix is ""
// internally, which reads as nothing at all in a log line.
func displayPrefix(prefix string) string {
	if prefix == "" {
		return "/"
	}

	return prefix
}

// applyGinMode aligns Gin's own mode with the application mode of the config:
// mode "production" runs Gin in release mode, "test" in test mode, anything
// else — including an empty value — in debug mode. Without this, mode had no
// effect on Gin and a production deployment kept serving with the debug logger
// and the route dump.
//
// An explicit GIN_MODE environment variable wins. It is Gin's own documented
// lever, deployments already rely on it, and honouring the config over it
// would silently break them.
func applyGinMode(mode string) {
	if os.Getenv(gin.EnvGinMode) != "" {
		return
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case productionMode:
		gin.SetMode(gin.ReleaseMode)
	case testConfigMode:
		gin.SetMode(gin.TestMode)
	default:
		gin.SetMode(gin.DebugMode)
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
		MaxAge:           config.MaxAge.Duration(),
	}
}

func listenAddress(host string, port int) string {
	if host == "" {
		host = defaultHost
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
