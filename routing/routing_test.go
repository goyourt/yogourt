package routing

import (
	"strings"
	"testing"

	"github.com/goyourt/yogourt/services/providers"
)

func TestBuildCORSConfigDefaultsToAllowAllOrigins(t *testing.T) {
	mainConfig := &providers.MainConfig{}

	config := buildCORSConfig(mainConfig)

	if !config.AllowAllOrigins {
		t.Fatal("expected CORS to allow all origins when no origin is configured")
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("expected valid default CORS config, got %v", err)
	}
}

func TestBuildCORSConfigPreservesConfiguredOrigins(t *testing.T) {
	mainConfig := &providers.MainConfig{}
	mainConfig.CORS.AllowedOrigins = []string{"https://app.example.com"}

	config := buildCORSConfig(mainConfig)

	if config.AllowAllOrigins {
		t.Fatal("expected configured origins to disable the allow-all fallback")
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

	config := buildCORSConfig(mainConfig)

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
