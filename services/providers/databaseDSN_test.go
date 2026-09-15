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

// A loopback host without the exception stays a network host: verify-full by
// default, weaker modes refused — but the refusal names the key that unblocks
// local development.
func TestLoopbackWithoutExceptionBehavesLikeANetworkHost(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = "localhost"

	mode, err := resolveDatabaseSSLMode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "verify-full" {
		t.Fatalf("loopback default mode = %q, want verify-full", mode)
	}

	cfg.SSLMode = "disable"
	_, err = resolveDatabaseSSLMode(cfg)
	if err == nil {
		t.Fatal("loopback host without allow_insecure_loopback accepted disable")
	}
	if !strings.Contains(err.Error(), "database.allow_insecure_loopback") {
		t.Errorf("error %q does not name database.allow_insecure_loopback as the way out", err)
	}
}

func TestLoopbackExceptionDefaultsToDisable(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = "localhost"
	cfg.AllowInsecureLoopback = true

	mode, err := resolveDatabaseSSLMode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "disable" {
		t.Fatalf("loopback exception default mode = %q, want disable", mode)
	}
}

// With the exception, ssl_mode keeps its say: an explicit mode is used as
// declared, verify-full included.
func TestLoopbackExceptionHonorsExplicitMode(t *testing.T) {
	for _, want := range []string{"disable", "allow", "prefer", "require", "verify-ca", "verify-full"} {
		cfg := baseDatabaseConfig()
		cfg.Host = "localhost"
		cfg.AllowInsecureLoopback = true
		cfg.SSLMode = want

		mode, err := resolveDatabaseSSLMode(cfg)
		if err != nil {
			t.Errorf("resolveDatabaseSSLMode(%q) = %v, want no error", want, err)
			continue
		}
		if mode != want {
			t.Errorf("loopback exception mode = %q, want %q", mode, want)
		}
	}
}

// The exception recognizes the literals a developer types, IPv6 and bracketed
// forms included.
func TestLoopbackExceptionCoversLoopbackLiterals(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "127.0.0.2", "::1", "[::1]"} {
		cfg := baseDatabaseConfig()
		cfg.Host = host
		cfg.AllowInsecureLoopback = true

		mode, err := resolveDatabaseSSLMode(cfg)
		if err != nil {
			t.Errorf("resolveDatabaseSSLMode(host %q) = %v, want no error", host, err)
			continue
		}
		if mode != "disable" {
			t.Errorf("host %q mode = %q, want disable", host, mode)
		}
	}
}

// The exception never reaches past the machine: a name is not resolved, so a
// DNS entry answering 127.0.0.1 stays a network host.
func TestLoopbackExceptionCannotWeakenNetworkTransport(t *testing.T) {
	for _, host := range []string{"db.internal", "loopback.example.com", "10.0.0.1"} {
		cfg := baseDatabaseConfig()
		cfg.Host = host
		cfg.AllowInsecureLoopback = true

		_, err := resolveDatabaseSSLMode(cfg)
		if err == nil {
			t.Errorf("host %q accepted allow_insecure_loopback", host)
			continue
		}
		if !strings.Contains(err.Error(), "database.allow_insecure_loopback") {
			t.Errorf("error %q does not name the configuration key", err)
		}
	}
}

func TestLoopbackExceptionDoesNotApplyToUnixSockets(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = "/private/tmp/postgres"
	cfg.AllowInsecureLoopback = true

	if _, err := resolveDatabaseSSLMode(cfg); err == nil {
		t.Fatal("Unix socket host accepted allow_insecure_loopback")
	}
}

// An empty host is the default socket: it keeps today's verify-full and the
// loopback exception has nothing to apply to.
func TestEmptyDatabaseHostIgnoresTheLoopbackException(t *testing.T) {
	cfg := baseDatabaseConfig()
	cfg.Host = ""
	cfg.AllowInsecureLoopback = true

	mode, err := resolveDatabaseSSLMode(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if mode != "verify-full" {
		t.Fatalf("empty host mode = %q, want verify-full", mode)
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
