package database

import (
	"fmt"
	"github.com/goyourt/yogourt/cli/config"
	"log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB Instance globale de la base de données
var DB *gorm.DB

// InitDatabase Chargement de la base de données
func InitDatabase(configPathAdded string) {
	cfg, err := config.LoadConfig(configPathAdded)
	if err != nil {
		log.Fatal(err)
	}

	// Construction de l'url de connexion à la base de données (sslmode=disable --> SSL désactivé pour connexion en local)
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable", cfg.Database.Host, cfg.Database.User, cfg.Database.Password, cfg.Database.DBName, cfg.Database.Port)

	// Connexion à la base de données prostgreSQL
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Erreur de connexion à la base de données: %v", err)
	}

	// Stockage de la connexion à la bdd dans DB
	fmt.Println("✅ Connexion réussie à PostgreSQL")
	DB = db
}
