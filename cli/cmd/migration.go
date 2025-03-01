package cmd

import (
	"fmt"
	"os"

	"github.com/goyourt/yogourt/database"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

/* Commande Migration */
var MigrationCmd = &cobra.Command{
	Use:   "migrate [modelName]",
	Short: "Effectue une migration d'un modele",
	Long:  "Effectue une migration d'un modele vers la base de données configurée",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		modelName := args[0]
		migrate(modelName)
	},
}

func migrate(modelName string) {

	// Chargement du fichier .env
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("❌ Erreur de chargement du fichier .env")
		return
	}

	// Récupèration des variables d'environnement
	ProjectName := os.Getenv("PROJECT_NAME")
	ModelFolder := os.Getenv("MODEL_FOLDER")

	if _, err := os.Stat(ProjectName + "/config.yaml"); os.IsNotExist(err) {
		fmt.Println("❌ Fichier config.yaml non trouvé, veuillez entrer la commande suivante: yogourt init project_name")
		return
	} else {

		if _, err := os.Stat(ModelFolder + "/" + modelName + "Model.go"); os.IsNotExist(err) {
			fmt.Println("❌ Aucun modèle trouvé, veuillez créer un model avec la commande suivante: yogourt model model_name")
			return
		} else {
			// Initialissation de la base de données
			database.InitDatabase()

			// Migration
			err := database.DB.AutoMigrate("&" + ModelFolder + "." + modelName + "{}")
			if err != nil {
				fmt.Println("❌ Erreur de migration :", err)
			} else {
				fmt.Println("✅ Migration réussie")
			}
		}
	}
}
