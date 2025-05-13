package main

import (
	"cli/database"
	"cli/my_api/models"
	"log"
)

func main() {
	// 👇 C’est essentiel avant toute migration
	database.InitDatabase("../config.yaml")

	for name, model := range models.Models {
		if err := database.DB.AutoMigrate(model); err != nil {
			log.Printf("❌ Échec de la migration du modèle '%s': %v", name, err)
		} else {
			log.Printf("✅ Migration réussie pour le modèle '%s'", name)
		}
	}
}
