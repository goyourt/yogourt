package routing

import (
	"strings"
	"testing"
	"time"

	"github.com/goyourt/yogourt/services/providers"
)

// A configuration file without a cors section used to be read as "allow
// every origin": the framework opened the API precisely when nobody had said
// anything about it. Nothing is installed now.
func TestBuildCORSConfigDisabledWhenNoOriginConfigured(t *testing.T) {
	mainConfig := &providers.MainConfig{}

	config, enabled := buildCORSConfig(mainConfig)

	if enabled {
		t.Fatalf("expected no CORS middleware without configuration, got %#v", config)
	}
	if config.AllowAllOrigins {
		t.Error("an unconfigured cors section must never enable all origins")
	}
}

func TestBuildCORSConfigPreservesConfiguredOrigins(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowedOrigins = []string{"https://app.example.com"}

	config, enabled := buildCORSConfig(mainConfig)

	if !enabled {
		t.Fatal("expected a configured origin to install the middleware")
	}
	if config.AllowAllOrigins {
		t.Fatal("expected configured origins to keep the allow-all mode off")
	}
	if len(config.AllowOrigins) != 1 || config.AllowOrigins[0] != "https://app.example.com" {
		t.Fatalf("unexpected allowed origins: %#v", config.AllowOrigins)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("expected valid configured CORS config, got %v", err)
	}
}

func TestBuildCORSConfigHonorsAllowAllOrigins(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowAllOrigins = true
	mainConfig.CORS.AllowedOrigins = []string{"https://ignored.example.com"}

	config, enabled := buildCORSConfig(mainConfig)

	if !enabled {
		t.Fatal("expected allow_all_origins to install the middleware")
	}
	if !config.AllowAllOrigins {
		t.Fatal("expected allow_all_origins to be forwarded to Gin")
	}
	if len(config.AllowOrigins) != 0 {
		t.Fatalf("expected allow-all mode to clear conflicting origins, got %#v", config.AllowOrigins)
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("expected valid allow-all CORS config, got %v", err)
	}
}

func TestListenAddressUsesConfiguredHost(t *testing.T) {
	got := listenAddress("127.0.0.1", 8080)
	if got != "127.0.0.1:8080" {
		t.Fatalf("listenAddress() = %q, want %q", got, "127.0.0.1:8080")
	}
}

func TestListenAddressUsesDefaultHost(t *testing.T) {
	got := listenAddress("", 8080)
	if got != "0.0.0.0:8080" {
		t.Fatalf("listenAddress() = %q, want %q", got, "0.0.0.0:8080")
	}
}

func TestListenAddressSupportsIPv6(t *testing.T) {
	got := listenAddress("::1", 8080)
	if got != "[::1]:8080" {
		t.Fatalf("listenAddress() = %q, want %q", got, "[::1]:8080")
	}
}

func TestResolveAPIPrefixDefaultsToDefaultPrefix(t *testing.T) {
	got, err := resolveAPIPrefix("", &providers.MainConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != DefaultPrefix {
		t.Fatalf("resolveAPIPrefix() = %q, want %q", got, DefaultPrefix)
	}
}

func TestResolveAPIPrefixPrefersTheOptionOverTheConfig(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.Server.BasePath = "/from-config"

	got, err := resolveAPIPrefix("/from-option/", mainConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/from-option" {
		t.Fatalf("resolveAPIPrefix() = %q, want %q", got, "/from-option")
	}
}

func TestResolveAPIPrefixReadsBasePath(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.Server.BasePath = "v1"

	got, err := resolveAPIPrefix("", mainConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/v1" {
		t.Fatalf("resolveAPIPrefix() = %q, want %q", got, "/v1")
	}
}

func TestResolveAPIPrefixAcceptsTheRoot(t *testing.T) {
	got, err := resolveAPIPrefix("/", &providers.MainConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("resolveAPIPrefix(\"/\") = %q, want the empty root prefix", got)
	}
	if displayPrefix(got) != "/" {
		t.Fatalf("displayPrefix() = %q, want %q", displayPrefix(got), "/")
	}
}

func TestResolveAPIPrefixNamesTheFaultySource(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.Server.BasePath = "/api/:version"

	_, err := resolveAPIPrefix("", mainConfig)
	if err == nil {
		t.Fatal("expected a Gin parameter in the prefix to be refused")
	}
	if !strings.Contains(err.Error(), "server.base_path") {
		t.Errorf("the error must name the source of the prefix, got %v", err)
	}

	if _, err = resolveAPIPrefix("/api/*catch", mainConfig); err == nil {
		t.Fatal("expected a catch-all in the prefix to be refused")
	} else if !strings.Contains(err.Error(), "routing.WithPrefix") {
		t.Errorf("the error must name the option, got %v", err)
	}
}

func TestNormalizePrefix(t *testing.T) {
	valid := map[string]string{
		"/api":    "/api",
		"api":     "/api",
		"/v1/":    "/v1",
		"  /v1  ": "/v1",
		"/api/v2": "/api/v2",
		"/":       "",
		"":        "",
		"   ":     "",
	}
	for raw, want := range valid {
		got, err := normalizePrefix(raw)
		if err != nil {
			t.Errorf("normalizePrefix(%q) returned %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("normalizePrefix(%q) = %q, want %q", raw, got, want)
		}
	}

	for _, raw := range []string{"/api//v2", "/api/:id", "/api/*path", "/two words"} {
		if _, err := normalizePrefix(raw); err == nil {
			t.Errorf("normalizePrefix(%q) = nil error, want a refusal", raw)
		}
	}
}

// Empty method and header lists reached Gin untouched, and a preflight
// answered without Access-Control-Allow-Methods blocks every non-simple
// request.
func TestBuildCORSConfigFillsEmptyMethodsAndHeaders(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowedOrigins = []string{"https://app.example.com"}

	config, enabled := buildCORSConfig(mainConfig)

	if !enabled {
		t.Fatal("expected the middleware to be installed")
	}
	if len(config.AllowMethods) == 0 {
		t.Error("expected default methods, got none")
	}
	if len(config.AllowHeaders) == 0 {
		t.Error("expected default headers, got none")
	}
}

func TestBuildCORSConfigKeepsConfiguredMethodsAndHeaders(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowedOrigins = []string{"https://app.example.com"}
	mainConfig.CORS.AllowedMethods = []string{"GET"}
	mainConfig.CORS.AllowedHeaders = []string{"Authorization"}

	config, _ := buildCORSConfig(mainConfig)

	if len(config.AllowMethods) != 1 || config.AllowMethods[0] != "GET" {
		t.Errorf("configured methods must win, got %#v", config.AllowMethods)
	}
	if len(config.AllowHeaders) != 1 || config.AllowHeaders[0] != "Authorization" {
		t.Errorf("configured headers must win, got %#v", config.AllowHeaders)
	}
}

// max_age was multiplied by time.Hour on its way to Gin, which turned 12h
// into 477807h and made the field unusable.
func TestBuildCORSConfigForwardsMaxAgeUnchanged(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowAllOrigins = true
	mainConfig.CORS.MaxAge = providers.Duration(12 * time.Hour)

	config, _ := buildCORSConfig(mainConfig)

	if config.MaxAge != 12*time.Hour {
		t.Errorf("MaxAge = %v, want %v", config.MaxAge, 12*time.Hour)
	}
}

func TestCORSEnabledOnlyFalseDisablesIt(t *testing.T) {
	enabled, disabled := true, false

	cases := map[string]struct {
		value *bool
		want  bool
	}{
		"absent key keeps CORS":  {nil, true},
		"cors: true keeps CORS":  {&enabled, true},
		"cors: false drops CORS": {&disabled, false},
	}

	for name, c := range cases {
		mainConfig := &providers.MainConfig{}
		mainConfig.Server.CORS = c.value
		if got := corsEnabled(mainConfig); got != c.want {
			t.Errorf("%s: corsEnabled() = %v, want %v", name, got, c.want)
		}
	}
}

func TestCORSSectionConfiguredDetectsADeadSection(t *testing.T) {
	if corsSectionConfigured(&providers.MainConfig{}) {
		t.Error("an empty cors section must not be reported as configured")
	}

	withOrigins := &providers.MainConfig{}
	withOrigins.CORS.AllowedOrigins = []string{"https://app.example.com"}
	if !corsSectionConfigured(withOrigins) {
		t.Error("a section declaring an origin must be reported as configured")
	}

	withMaxAge := &providers.MainConfig{}
	withMaxAge.CORS.MaxAge = providers.Duration(12 * time.Hour)
	if !corsSectionConfigured(withMaxAge) {
		t.Error("a section declaring only max_age must be reported as configured")
	}
}

func TestBootBannerUsesAppNameAndVersion(t *testing.T) {
	mainConfig := &providers.MainConfig{AppName: "demo", Version: "1.2.3", Mode: "production"}
	if got, want := bootBanner(mainConfig), "Starting demo 1.2.3 (mode: production)"; got != want {
		t.Errorf("bootBanner() = %q, want %q", got, want)
	}

	empty := bootBanner(&providers.MainConfig{})
	if !strings.Contains(empty, "yogourt application") || !strings.Contains(empty, "debug") {
		t.Errorf("bootBanner() of an empty config = %q, want a fallback name and the effective Gin mode", empty)
	}
}

// The route folder comes from paths.route_folder, and from nowhere else:
// Initialize takes no folder argument to contradict it with.
func TestResolveAPIFolderReadsRouteFolder(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.Paths.RouteFolder = "  ./routes/  "

	got, err := resolveAPIFolder(mainConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "./routes/" {
		t.Fatalf("resolveAPIFolder() = %q, want %q", got, "./routes/")
	}
}

func TestResolveAPIFolderFailsWhenRouteFolderIsMissing(t *testing.T) {
	err := resolveAPIFolderError(t, &providers.MainConfig{})
	if !strings.Contains(err.Error(), "paths.route_folder") {
		t.Fatalf("error %q does not name the configuration key", err)
	}
}

// resolveAPIFolderError returns the error of a resolution expected to fail.
func resolveAPIFolderError(t *testing.T, mainConfig *providers.MainConfig) error {
	t.Helper()

	folder, err := resolveAPIFolder(mainConfig)
	if err == nil {
		t.Fatalf("resolveAPIFolder() = %q, want an error when route_folder is unset", folder)
	}
	return err
}
