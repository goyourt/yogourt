package cmd

import (
	"cli/config"
	"cli/database"
	"fmt"
	"os"

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

	// Vérification et lecture du fichier config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf(`❌ Fichier config.yaml non trouvé, assurez vous que celui-ci se trouve à la racine de votre projet ou
   que vous avez entré la commande suivante: yogourt init project_name`)
		return
	} else {

		// Récupération de la variable d'environnement depuis le fichier config
		ModelFolder := cfg.Paths.ModelFolder

		if _, err := os.Stat(ModelFolder + "/" + modelName + "Model.go"); os.IsNotExist(err) {
			fmt.Println("❌ Aucun modèle trouvé, veuillez créer un modèle avec la commande suivante: yogourt model model_name")
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
