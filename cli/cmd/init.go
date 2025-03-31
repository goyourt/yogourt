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
	Short: "Initialise un nouveau projet yogourt",
	Long:  "Crée la structure de base pour un nouveau projet yogourt",
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
		createMiddlewareFile(ProjectName)
	},
}

// Mise à jour du fichier .env interne
func updateEnvFile(envPath, projectName string) error {

	// Charge le fichier .env existant
	envMap, err := godotenv.Read(envPath)
	if err != nil {
		return fmt.Errorf("Impossible de lire le fichier .env: %w", err)
	}

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
  port: 8080
  cors: true

database:
  type: "postgres"
  user: "admin"
  password: "password"
  host: "localhost"
  port: 5432
  dbname: "mydb"

paths:
  model_path: "my_api/models/"
  project_name: "my_api"
  main_file_path: "my_api/main.go"
  route_path: "my_api/routes/"

`

	file.WriteString(configFileContent) //Ecriture du contenu dans le fichier config
}

/* --- Fin création du fichier config --- */

/* --- Création du fichier middleware --- */
func createMiddlewareFile(ProjectName string) {

	/* Dossier middleware - présent dans le dossier principal */
	MiddlewareFolder := ProjectName + "/middleware/"

	middlewareFolderError := os.Mkdir(MiddlewareFolder, os.ModePerm)

	if middlewareFolderError != nil {
		fmt.Printf("Erreur lors de la création du dossier middleware: %v \n", middlewareFolderError)
		return
	}

	//Création du fichier middleware
	MiddlewareFile := MiddlewareFolder + "middleware.go"

	file, middlewareFileError := os.Create(MiddlewareFile)
	if middlewareFileError != nil {
		fmt.Printf("Erreur lors de la création du fichier middleware: %v \n", middlewareFileError)
		return
	}
	defer file.Close() //Fermeture du fichier config

	middlewareFileContent := `package middleware

import (
	"github.com/gin-gonic/gin"
)

var Callbacks = map[string]func(*gin.Context){
	"/":          base,
}

func base(c *gin.Context) {
	c.Next()
}

`

	file.WriteString(middlewareFileContent) //Ecriture du contenu dans le fichier middleware
}

/* --- Fin création du fichier middleware --- */

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

	/* Dossier model - présent dans le dossier principal */
	ModelFolder := ProjectName + "/models/"

	modelFolderError := os.Mkdir(ModelFolder, os.ModePerm)

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
	// Imports
)

func main() {
	/* Bienvenue sur yogourt ! */
}
`
	file.WriteString(mainFileContent) //Ecriture du contenu dans le fichier main.go

	fmt.Println("L'environnement a été initialisé avec succès.")

	// Sauvegarde dans un fichier .env
	err := godotenv.Write(map[string]string{
		"ROUTE_FOLDER": RouteFolder,
		"MAIN_FILE":    MainFile,
		"PROJECT_NAME": ProjectName,
		"MODEL_FOLDER": ModelFolder,
	}, ".env")
	if err != nil {
		fmt.Println("Erreur lors de la création du fichier .env:", err)
		return
	}
}
