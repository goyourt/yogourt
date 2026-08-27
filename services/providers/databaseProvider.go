package providers

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// cacheDialTimeout bounds both the TCP dial and the readiness PING performed
// by InitCache. A cache host that is unreachable — rather than actively
// refusing the connection — must fail the caller, not hang it.
const cacheDialTimeout = 2 * time.Second

// DB global instance of the database
var (
	dbOnce  sync.Once
	cacheMu sync.Mutex
	db      *gorm.DB
	cache   *redis.Client
)

func GetDB() *gorm.DB {
	dbOnce.Do(func() {
		db = InitDB()
	})
	return db
}

// GetCache returns the shared Redis client, connecting on first use.
//
// The client is memoized only once it has answered a PING, so an outage at
// the first call does not poison the singleton for the lifetime of the
// process: the next caller retries the connection instead of being handed a
// client that will never work.
func GetCache() (*redis.Client, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cache != nil {
		return cache, nil
	}

	client, err := InitCache()
	if err != nil {
		return nil, err
	}

	cache = client
	return cache, nil
}

// InitDB opens a PostgreSQL connection from the database section of the
// configuration.
//
// Every failure — an unsupported driver, an sslmode libpq does not know, an
// unreachable server — is returned. It used to call log.Fatalf, which killed
// the process from inside whatever request first touched the database: an
// outage of a few seconds took the whole application down instead of failing
// the requests that needed it.
func InitDB() *gorm.DB {
	cfg := GetMainConfig()

	if err := validateDatabaseType(cfg.Database.Type); err != nil {
		log.Fatalf("❌ %v", err)
	}

	if err := validateSSLMode(cfg.Database.SSLMode); err != nil {
		log.Fatalf("❌ %v", err)
	}

	db, err := gorm.Open(postgres.Open(buildDSN(cfg.Database)), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Error while connecting database: %v", err)
	}

	if err := applyPoolSettings(db, cfg.Database.Pool); err != nil {
		log.Fatalf("❌ %v", err)
	}

	fmt.Println("✅ Connexion with PostgreSQL")
	return db
}

// defaultSSLMode keeps the connection in clear text when database.ssl_mode is
// empty. The DSN hard-coded it, and turning TLS on for every application that
// never wrote the key would break the ones talking to a server without it.
const defaultSSLMode = "disable"

// sslModes lists the libpq values database.ssl_mode accepts. An unknown mode
// is refused at boot: libpq would reject the DSN anyway, with an error that
// does not name the configuration key behind it.
var sslModes = map[string]bool{
	"disable":     true,
	"allow":       true,
	"prefer":      true,
	"require":     true,
	"verify-ca":   true,
	"verify-full": true,
}

// validateSSLMode checks database.ssl_mode against the libpq modes.
func validateSSLMode(sslMode string) error {
	mode := strings.ToLower(strings.TrimSpace(sslMode))
	if mode == "" || sslModes[mode] {
		return nil
	}

	return fmt.Errorf("unsupported database.ssl_mode %q: use one of disable, allow, prefer, require, verify-ca, verify-full", sslMode)
}

// buildDSN assembles the libpq keyword/value connection string.
//
// Every value is quoted and escaped: the DSN used to be concatenated as it
// came, so a password holding a space or a quote — the kind a generator
// produces — silently truncated the DSN into a connection to something else.
// Optional keywords are only written when they carry a value, which leaves
// libpq its own defaults for the rest.
func buildDSN(cfg DatabaseConfig) string {
	sslMode := strings.ToLower(strings.TrimSpace(cfg.SSLMode))
	if sslMode == "" {
		sslMode = defaultSSLMode
	}

	// A port of 0 is an undeclared port, not a port to dial: leaving the
	// keyword out lets libpq fall back to 5432.
	port := ""
	if cfg.Port > 0 {
		port = strconv.Itoa(cfg.Port)
	}

	pairs := []struct{ keyword, value string }{
		{"host", cfg.Host},
		{"user", cfg.User},
		{"password", cfg.Password},
		{"dbname", cfg.DB},
		{"port", port},
		{"sslmode", sslMode},
		{"sslrootcert", strings.TrimSpace(cfg.SSLRootCert)},
		{"sslcert", strings.TrimSpace(cfg.SSLCert)},
		{"sslkey", strings.TrimSpace(cfg.SSLKey)},
		{"search_path", strings.TrimSpace(cfg.SearchPath)},
	}

	var dsn strings.Builder
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		if dsn.Len() > 0 {
			dsn.WriteByte(' ')
		}
		dsn.WriteString(pair.keyword)
		dsn.WriteByte('=')
		dsn.WriteString(quoteDSNValue(pair.value))
	}

	return dsn.String()
}

// quoteDSNValue writes a value the way libpq reads one: single quotes around
// it, backslashes and single quotes escaped.
func quoteDSNValue(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value)
	return "'" + escaped + "'"
}

// applyPoolSettings bounds the database/sql pool behind GORM. A zero field
// changes nothing, so a configuration that never declared a pool keeps the
// defaults of database/sql.
func applyPoolSettings(db *gorm.DB, pool DatabasePoolConfig) error {
	if pool == (DatabasePoolConfig{}) {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("Error reading the connection pool: %v", err)
	}

	if pool.MaxOpenConns != 0 {
		sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	}
	if pool.MaxIdleConns != 0 {
		sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	}
	if pool.ConnMaxLifetime != 0 {
		sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime.Duration())
	}
	if pool.ConnMaxIdleTime != 0 {
		sqlDB.SetConnMaxIdleTime(pool.ConnMaxIdleTime.Duration())
	}

	return nil
}

// validateDatabaseType checks database.type against the only driver this
// provider can build. The field used to be parsed and read by nothing, so a
// config asking for "mysql" quietly opened a PostgreSQL connection instead;
// short of supporting a second driver, naming the mismatch at boot is the
// least surprising thing the field can do. An empty value stays valid: it is
// what a config that never cared about the driver holds.
func validateDatabaseType(databaseType string) error {
	switch strings.ToLower(strings.TrimSpace(databaseType)) {
	case "", "postgres", "postgresql":
		return nil
	}

	return fmt.Errorf("unsupported database.type %q: the provider only builds a PostgreSQL connection — use \"postgres\" or leave the field empty", databaseType)
}

// InitCache builds a Redis client and checks that it answers before handing
// it over. Unlike gorm.Open, redis.NewClient never touches the network: without
// this PING an unreachable or misconfigured instance would only surface at the
// first cache operation, inside a request and far from the configuration that
// caused it.
func InitCache() (*redis.Client, error) {
	cfg := GetMainConfig().Cache

	client := redis.NewClient(&redis.Options{
		Addr:        cfg.Host + ":" + cfg.Port,
		Password:    cfg.Password,
		DB:          cfg.DB,
		DialTimeout: cacheDialTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), cacheDialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("❌ Error while connecting cache at %s (db %d): %w", client.Options().Addr, cfg.DB, err)
	}

	fmt.Println("✅ Connexion with Redis")
	return client, nil
}
