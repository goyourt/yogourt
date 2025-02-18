package cmd

import (
	"fmt"
	"os"

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
		InitProject(ProjectName)
	},
}

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

import "fmt"

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

	/* Fichier config - présent dans le dossier principal */
	configFile := ProjectName + "/go.mod"

	file, configFileError := os.Create(configFile)
	if configFileError != nil {
		fmt.Printf("Erreur lors de la création du fichier config: %v \n", configFileError)
		return
	}
	defer file.Close() //Fermeture du fichier config

	fmt.Println("L'environnement a été initialisé avec succès.")

	// Sauvegarde dans un fichier .env
	err := godotenv.Write(map[string]string{
		"ROUTE_FOLDER":   RouteFolder,
		"HANDLER_FOLDER": HandlerFolder,
		"MAIN_FILE":      MainFile,
	}, ".env")
	if err != nil {
		fmt.Println("Erreur lors de la création du fichier .env:", err)
		return
	}
}
