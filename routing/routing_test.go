package routing

import (
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
