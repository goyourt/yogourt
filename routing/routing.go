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

	log.Print(bootBanner(mainConfig))

	r := gin.Default()
	if !corsEnabled(mainConfig) {
		log.Print("CORS is off (server.cors: false): no CORS header, and preflight requests are not answered")
		if corsSectionConfigured(mainConfig) {
			log.Print("warning: server.cors is false, so the whole cors section of the configuration file is ignored")
		}
	} else if corsConfig, enabled := buildCORSConfig(mainConfig); enabled {
		// cors.New panics on a configuration it rejects — an origin without
		// a scheme, typically. Validating first turns that panic into a
		// message that names the culprit.
		if err := corsConfig.Validate(); err != nil {
			log.Fatalf("Invalid cors section in the configuration file: %v", err)
		}
		if corsConfig.AllowAllOrigins && corsConfig.AllowCredentials {
			log.Print("warning: cors.allow_credentials has no effect while cors.allow_all_origins is set; browsers refuse credentialed responses sent with 'Access-Control-Allow-Origin: *'. List the origins explicitly.")
		}
		r.Use(cors.New(corsConfig))
	} else if mainConfig.Server.CORS != nil {
		// server.cors: true asked for CORS, and the cors section declares no
		// origin to allow: Gin would have nothing to answer with. An absent
		// key stays silent — it asked for nothing.
		log.Print("warning: server.cors is true but the cors section declares no origin; no CORS header is emitted")
	}

	if corsEnabled(mainConfig) {
		// Preflight answers come before the route tree: a browser sends
		// OPTIONS on paths that declare no OPTIONS handler. With CORS off
		// there is nothing to preflight, so the catch-all goes away too and
		// OPTIONS reaches the routes — or a 404.
		r.OPTIONS("/*path", func(c *gin.Context) {
			c.AbortWithStatus(204)
		})
	}

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

// corsEnabled reports whether CORS is handled at all: the middleware when the
// cors section declares something, and the preflight catch-all. An absent
// server.cors keeps both — the key was ignored until now, so a missing value
// must not turn CORS off under an application that relies on it. Only an
// explicit "cors: false" does.
func corsEnabled(mainConfig *providers.MainConfig) bool {
	return mainConfig.Server.CORS == nil || *mainConfig.Server.CORS
}

// corsSectionConfigured reports whether the cors section holds anything. With
// CORS switched off that section becomes dead configuration, and saying so at
// boot is cheaper than wondering why an allowed origin has no effect.
func corsSectionConfigured(mainConfig *providers.MainConfig) bool {
	config := mainConfig.CORS

	return config.AllowAllOrigins ||
		len(config.AllowedOrigins) > 0 ||
		len(config.AllowedMethods) > 0 ||
		len(config.AllowedHeaders) > 0 ||
		config.AllowCredentials ||
		config.MaxAge != 0
}

// bootBanner names the application the process serves. app_name and version
// were parsed and read by nothing; telling which build is running is the least
// a deployment can expect from them.
func bootBanner(mainConfig *providers.MainConfig) string {
	name := strings.TrimSpace(mainConfig.AppName)
	if name == "" {
		name = "yogourt application"
	}
	if version := strings.TrimSpace(mainConfig.Version); version != "" {
		name += " " + version
	}

	mode := strings.TrimSpace(mainConfig.Mode)
	if mode == "" {
		mode = "unset, Gin runs in debug"
	}

	return fmt.Sprintf("Starting %s (mode: %s)", name, mode)
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

// buildCORSConfig translates the cors section into a Gin CORS configuration,
// and reports whether the middleware should be installed at all.
//
// Gin's CORS middleware has no closed mode: cors.New panics on a
// configuration declaring neither an origin nor AllowAllOrigins. That panic
// used to be avoided by reading an empty section as AllowAllOrigins, so
// forgetting the section opened the API to every origin. An empty section now
// leaves the middleware out instead: no CORS header is emitted and browsers
// keep refusing cross-origin calls, which is what a missing configuration
// should mean.
func buildCORSConfig(mainConfig *providers.MainConfig) (cors.Config, bool) {
	config := mainConfig.CORS
	if !config.AllowAllOrigins && len(config.AllowedOrigins) == 0 {
		return cors.Config{}, false
	}

	allowedOrigins := config.AllowedOrigins
	if config.AllowAllOrigins {
		// cors.Config.Validate rejects an explicit list alongside
		// AllowAllOrigins, so the wider setting wins.
		allowedOrigins = nil
	}

	// Empty lists reached Gin as-is, and a preflight answered without
	// Access-Control-Allow-Methods/-Headers blocks every non-simple request.
	// Gin's own defaults are a working starting point; note that they do not
	// include Authorization, which an API behind JWT must list itself.
	defaults := cors.DefaultConfig()
	allowedMethods := config.AllowedMethods
	if len(allowedMethods) == 0 {
		allowedMethods = defaults.AllowMethods
	}
	allowedHeaders := config.AllowedHeaders
	if len(allowedHeaders) == 0 {
		allowedHeaders = defaults.AllowHeaders
	}

	return cors.Config{
		AllowAllOrigins:  config.AllowAllOrigins,
		AllowOrigins:     allowedOrigins,
		AllowMethods:     allowedMethods,
		AllowHeaders:     allowedHeaders,
		AllowCredentials: config.AllowCredentials,
		MaxAge:           config.MaxAge.Duration(),
	}, true
}

func listenAddress(host string, port int) string {
	if host == "" {
		host = defaultHost
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
