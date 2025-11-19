package providers

import (
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB global instance of the database
var db *gorm.DB
var cache *redis.Client

func GetDB() *gorm.DB {
	if db == nil {
		db = InitDB()
	}
	return db
}

func GetCache() *redis.Client {
	if cache == nil {
		cache = InitCache()
	}
	return cache
}

func InitDB() *gorm.DB {
	cfg := GetConfig()

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", cfg.Database.Host, cfg.Database.User, cfg.Database.Password, cfg.Database.DB, cfg.Database.Port)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Error while connecting database: %v", err)
	}

	fmt.Println("✅ Connexion with PostgreSQL")
	return db
}

func InitCache() *redis.Client {
	cfg := GetConfig().Cache

	return redis.NewClient(&redis.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}
