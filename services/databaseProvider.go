package services

import (
	"fmt"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB Instance globale de la base de données
var DB *gorm.DB

// InitDatabase Chargement de la base de données
func Init() {
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

func GetDB() *gorm.DB {
	if DB == nil {
		Init()
	}
	return DB
}
