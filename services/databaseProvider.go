package services

import (
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB Instance globale de la base de données
var DB *gorm.DB
var Cache *redis.Client

// InitDatabase Chargement de la base de données
func InitDB() {
	cfg := GetConfig()

	// Construction de l'url de connexion à la base de données (sslmode=disable --> SSL désactivé pour connexion en local)
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", cfg.Database.Host, cfg.Database.User, cfg.Database.Password, cfg.Database.DB, cfg.Database.Port)

	// Connexion à la base de données prostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Erreur de connexion à la base de données: %v", err)
	}

	// Stockage de la connexion à la bdd dans DB
	fmt.Println("✅ Connexion réussie à PostgreSQL")
	DB = db
}

func InitCache() {
	cfg := GetConfig().Cache

	Cache = redis.NewClient(&redis.Options{
		Addr:     cfg.Host + ":" + cfg.Port,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func GetDB() *gorm.DB {
	if DB == nil {
		InitDB()
	}
	return DB
}

func GetCache() *redis.Client {
	if Cache == nil {
		InitCache()
	}
	return Cache
}
