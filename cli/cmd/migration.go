package cmd

import (
	"fmt"
	"github.com/goyourt/yogourt/cli/config"
	"github.com/goyourt/yogourt/cli/database"
	"log"
	"os"
	"os/exec"

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

// Cette fonction sert à exécuter le fichier migrate.go du dossier cmd
func executeMigration() {

	// Initialisation du fichier de logs
	InitLogsFile()

	// Vérification et lecture du fichier config
	cfg, err := config.LoadConfig(ConfigPath)
	if err != nil {
		fmt.Printf(`❌ Fichier config.yaml non trouvé, assurez vous que celui-ci se trouve à la racine de votre projet ou
   que vous avez entré la commande suivante: yogourt init project_name`)
		log.Printf("ERROR: %s\n", err) // Ecriture des logs
		return
	} else {

		// Définir le chemin du projet de l'utilisateur (chemin vers le projet de l'utilisateur)
		ProjectName := cfg.Paths.ProjectName
		pathToUserProject := "./" + ProjectName

		// Crée une commande pour exécuter `go run cmd/migrate/main.go`
		cmd := exec.Command("go", "run", "./cmd/migrate.go")
		cmd.Dir = pathToUserProject // Définit le répertoire de travail du projet
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		// Exécution de la commande
		cmd.Run()
	}
}

func migrate(modelName string) {

	// Initialisation du fichier de logs
	InitLogsFile()

	// Vérification et lecture du fichier config
	cfg, err := config.LoadConfig(ConfigPath)
	if err != nil {
		fmt.Printf(`❌ Fichier config.yaml non trouvé, assurez vous que celui-ci se trouve à la racine de votre projet ou
   que vous avez entré la commande suivante: yogourt init project_name`)
		log.Printf("ERROR: %s\n", err) // Ecriture des logs
		return
	} else {

		// Récupération de la variable d'environnement depuis le fichier config
		ModelFolder := cfg.Paths.ModelFolder

		if _, err := os.Stat(ModelFolder + "/" + modelName + "Model.go"); os.IsNotExist(err) {
			fmt.Println("❌ Aucun modèle trouvé, veuillez créer un modèle avec la commande suivante: yogourt model model_name")
			log.Printf("ERROR: %s\n", err) // Ecriture des logs
			return
		} else {
			// Initialisation BDD
			database.InitDatabase(ConfigPath)

			// Migration via le fichier cmd/migrate/main.go
			executeMigration()
		}
	}
}

/* --- Ajout de la commande migration à la commande root --- */
func init() {
	rootCmd.AddCommand(MigrationCmd)
}
