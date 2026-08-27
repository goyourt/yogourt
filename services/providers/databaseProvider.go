package providers

import (
	"context"
	"fmt"
	"log"
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

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", cfg.Database.Host, cfg.Database.User, cfg.Database.Password, cfg.Database.DB, cfg.Database.Port)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Error while connecting database: %v", err)
	}

	fmt.Println("✅ Connexion with PostgreSQL")
	return db
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
