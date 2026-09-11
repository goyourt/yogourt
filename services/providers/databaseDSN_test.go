package providers

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// baseDatabaseConfig is the section a configuration declaring nothing beyond
// the connection holds.
func baseDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Host:     "db.internal",
		User:     "app",
		Password: "secret",
		DB:       "app_db",
		Port:     5432,
	}
}

// Network connections verify the server identity even when ssl_mode is absent.
func TestBuildDSNDefaultsToVerifiedTLSOnNetwork(t *testing.T) {
	got, err := buildDSN(baseDatabaseConfig())
	if err != nil {
		t.Fatal(err)
	}
	want := "host='db.internal' user='app' password='secret' dbname='app_db' port='5432' sslmode='verify-full'"

	if got != want {
		t.Fatalf("buildDSN() = %q, want %q", got, want)
	}
}

// The TLS, schema and port keywords are only written when they carry a value,
// so libpq keeps its own defaults for the rest.
func TestBuildDSNWritesOnlyDeclaredKeywords(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.SSLMode = "verify-full"
	cfg.SSLRootCert = "/etc/ssl/root.crt"
	cfg.SearchPath = "app,public"

	got, err := buildDSN(cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"sslmode='verify-full'", "sslrootcert='/etc/ssl/root.crt'", "search_path='app,public'"} {
		if !strings.Contains(got, want) {
			t.Errorf("buildDSN() = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"sslcert=", "sslkey="} {
		if strings.Contains(got, unwanted) {
			t.Errorf("buildDSN() = %q, want no %q for an empty field", got, unwanted)
		}
	}
}

// A port of 0 is an undeclared port, not a port to dial.
func TestBuildDSNOmitsUnsetPort(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Port = 0

	got, err := buildDSN(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "port=") {
		t.Fatalf("buildDSN() = %q, want no port keyword", got)
	}
}

// The DSN used to be concatenated as it came: a password holding a space cut
// it short, and everything after the space was read as another keyword.
func TestBuildDSNQuotesValuesWithSpacesAndQuotes(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Password = `p ss'w\rd`

	got, err := buildDSN(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := `password='p ss\'w\\rd'`

	if !strings.Contains(got, want) {
		t.Fatalf("buildDSN() = %q, want it to contain %q", got, want)
	}
	if !strings.Contains(got, "sslmode='verify-full'") {
		t.Fatalf("buildDSN() = %q, want the keywords after the password to survive it", got)
	}
}

func TestNetworkDatabaseRejectsModesWithoutIdentityVerification(t *testing.T) {
	for _, mode := range []string{"disable", "allow", "prefer", "require", "verify-ca"} {
		cfg := baseDatabaseConfig()
		cfg.SSLMode = mode
		if _, err := resolveDatabaseSSLMode(cfg); err == nil {
			t.Errorf("resolveDatabaseSSLMode(%q) = nil error, want a refusal", mode)
		}
	}
}

func TestResolveDatabaseSSLModeRejectsUnknownMode(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.SSLMode = "verify"
	_, err := resolveDatabaseSSLMode(cfg)
	if err == nil {
		t.Fatal("resolveDatabaseSSLMode() = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "database.ssl_mode") {
		t.Errorf("error %q does not name the configuration key", err)
	}
}

func TestLocalSocketRequiresExplicitClearTextException(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = "/private/tmp/postgres"
	cfg.SSLMode = "disable"

	if _, err := resolveDatabaseSSLMode(cfg); err == nil {
		t.Fatal("local socket without allow_insecure_local_socket was accepted")
	}
	cfg.AllowInsecureLocalSocket = true
	mode, err := resolveDatabaseSSLMode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "disable" {
		t.Fatalf("local socket mode = %q, want disable", mode)
	}
}

func TestLocalSocketExceptionCannotWeakenNetworkTransport(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.AllowInsecureLocalSocket = true
	if _, err := resolveDatabaseSSLMode(cfg); err == nil {
		t.Fatal("network host accepted allow_insecure_local_socket")
	}
}

func TestEmptyDatabaseHostCannotEnableTheLocalSocketException(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = ""

	mode, err := resolveDatabaseSSLMode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "verify-full" {
		t.Fatalf("empty host mode = %q, want verify-full", mode)
	}

	cfg.AllowInsecureLocalSocket = true
	if _, err := resolveDatabaseSSLMode(cfg); err == nil {
		t.Fatal("empty host accepted the local socket exception")
	}
}

// The pool section reads a duration string as well as a number of seconds.
func TestDatabasePoolDurations(t *testing.T) {
	cfg := &MainConfig{}
	yamlContent := []byte(`
database:
  pool:
    max_open_conns: 25
    max_idle_conns: 5
    conn_max_lifetime: 30m
    conn_max_idle_time: 90
`)

	if err := yaml.Unmarshal(yamlContent, cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pool := cfg.Database.Pool
	if pool.MaxOpenConns != 25 || pool.MaxIdleConns != 5 {
		t.Errorf("pool = %+v, want 25 open and 5 idle connections", pool)
	}
	if pool.ConnMaxLifetime.Duration() != 30*time.Minute {
		t.Errorf("conn_max_lifetime = %v, want 30m", pool.ConnMaxLifetime.Duration())
	}
	if pool.ConnMaxIdleTime.Duration() != 90*time.Second {
		t.Errorf("conn_max_idle_time = %v, want 90s", pool.ConnMaxIdleTime.Duration())
	}
}
