package providers

import (
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB global instance of the database
var (
	dbOnce    sync.Once
	cacheOnce sync.Once
	db        *gorm.DB
	cache     *redis.Client
)

func GetDB() *gorm.DB {
	dbOnce.Do(func() {
		db = InitDB()
	})
	return db
}

func GetCache() *redis.Client {
	cacheOnce.Do(func() {
		cache = InitCache()
	})
	return cache
}

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

func InitCache() *redis.Client {
	cfg := GetMainConfig().Cache

	return redis.NewClient(&redis.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
