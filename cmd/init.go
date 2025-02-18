package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

/* Commande init */
var InitCmd = &cobra.Command{
	Use:   "init [projectName]",
	Short: "Initialise un nouveau projet GOyourt",
	Long:  "Crée la structure de base pour un nouveau projet GOyourt",
	Args:  cobra.ExactArgs(1), //Attends un seul argument
	Run: func(cmd *cobra.Command, args []string) {
		ProjectName := args[0]

		// Met à jour ton fichier .env interne avec le nom du projet
		err := updateEnvFile(".env", ProjectName)
		if err != nil {
			fmt.Printf("❌ Erreur lors de la mise à jour du fichier .env : %v\n", err)
			return
		}
		InitProject(ProjectName)
		CreateConfigFile(ProjectName)
	},
}

// Mise à jour du fichier .env interne
func updateEnvFile(envPath, projectName string) error {

	// Charge le fichier .env existant
	envMap, err := godotenv.Read(envPath)
	if err != nil {
		return fmt.Errorf("Impossible de lire le fichier .env: %w", err)
	}

	// Variables à ajouter au .env
	envMap["PROJECT_NAME"] = projectName
	envMap["HANDLER_FOLDER"] = projectName + "/handlers/"
	envMap["MAIN_FILE"] = projectName + "/main.go"
	envMap["ROUTE_FOLDER"] = projectName + "/routes/"

	// Construction du nouveau contenu du fichier .env
	var newEnvContent strings.Builder
	for key, value := range envMap {
		newEnvContent.WriteString(fmt.Sprintf("%s=\"%s\"\n", key, value))
	}

	// Écriture dans le fichier .env interne
	err = os.WriteFile(envPath, []byte(newEnvContent.String()), 0644) //Permet l'ecriture dans le fichier avec l'autorisation lecture + ecriture (0644)
	if err != nil {
		return fmt.Errorf("Impossible d'écrire dans le fichier .env: %w", err)
	}
	return nil
}

/* --- Création du fichier config --- */
func CreateConfigFile(ProjectName string) {
	//Création du fichier config
	ConfigFile := ProjectName + "/config.yaml"

	file, configFileError := os.Create(ConfigFile)
	if configFileError != nil {
		fmt.Printf("Erreur lors de la création du fichier config: %v \n", configFileError)
		return
	}
	defer file.Close() //Fermeture du fichier config

	configFileContent := `app_name: "` + ProjectName + `"
version: "1.0.0"
mode: "development"

server:
  host: "127.0.0.1"
  port: 8080
  cors: true

database:
  type: "postgres"
  user: "admin"
  password: "password"
  host: "localhost"
  port: 5432
  dbname: "mydb"

auth:
  jwt_secret: "supersecretkey"
  token_expiry: 3600

logs:
  level: "debug"
  file: "logs/app.log"
`

	file.WriteString(configFileContent) //Ecriture du contenu dans le fichier config
}

/* --- FFin création du fichier config --- */

/* --- Fonction pour la commande "init" du package --- */
func InitProject(ProjectName string) {

	/* Dossier principal du package */
	projectNameError := os.Mkdir(ProjectName, os.ModePerm) //Création du dossier principal + attribution des droits

	if projectNameError != nil {
		fmt.Printf("Erreur lors de la création de l'environnement de travail: %v \n", projectNameError)
		return
	}

	/* Dossier route - présent dans le dossier principal */
	RouteFolder := ProjectName + "/routes/"

	routeFolderError := os.Mkdir(RouteFolder, os.ModePerm)

	if routeFolderError != nil {
		fmt.Printf("Erreur lors de la création du dossier routes: %v \n", routeFolderError)
		return
	}

	/* Dossier handler - présent dans le dossier principal */
	HandlerFolder := ProjectName + "/handlers/"

	handlerFolderError := os.Mkdir(HandlerFolder, os.ModePerm)

	if handlerFolderError != nil {
		fmt.Printf("Erreur lors de la création du dossier handlers: %v \n", handlerFolderError)
		return
	}

	/* Dossier model - présent dans le dossier principal */
	modelFolder := ProjectName + "/models/"

	modelFolderError := os.Mkdir(modelFolder, os.ModePerm)

	if modelFolderError != nil {
		fmt.Printf("Erreur lors de la création du dossier models: %v \n", modelFolderError)
		return
	}

	/* Fichier main - présent dans le dossier principal */
	MainFile := ProjectName + "/main.go"

	file, mainFileError := os.Create(MainFile)
	if mainFileError != nil {
		fmt.Printf("Erreur lors de la création du fichier main: %v \n", mainFileError)
		return
	}
	defer file.Close() //Fermeture du fichier main

	mainFileContent := `package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	// Démarre le serveur HTTP
	port := "8080"
	fmt.Printf("Serveur démarré sur le port %s\n", port)
	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		fmt.Println("Erreur lors du démarrage du serveur :", err)
	}
}
`
	file.WriteString(mainFileContent) //Ecriture du contenu dans le fichier main.go

	fmt.Println("L'environnement a été initialisé avec succès.")

	// Sauvegarde dans un fichier .env
	err := godotenv.Write(map[string]string{
		"ROUTE_FOLDER":   RouteFolder,
		"HANDLER_FOLDER": HandlerFolder,
		"MAIN_FILE":      MainFile,
		"PROJECT_NAME":   ProjectName,
	}, ".env")
	if err != nil {
		fmt.Println("Erreur lors de la création du fichier .env:", err)
		return
	}
}
