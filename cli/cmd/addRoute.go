package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var RouteCmd = &cobra.Command{
	Use:   "route [routeName]",
	Short: "Crée une nouvelle route avec handler correspondant",
	Long:  "Crée un nouveau fichier route dans le dossier routes/ et un handler correspondant dans le dossier handlers/",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		routeName := args[0]
		CreateRoute(routeName)
	},
}

/* --- Vérifie si les dossiers requis existent ou non --- */
func DoesFolderExist(folderName string) bool {
	info, err := os.Stat(folderName)
	if os.IsNotExist(err) {
		return false // Le dossier n'existe pas
	}
	return info.IsDir() // Vérifie si c'est un dossier
}

/* --- Création de la route --- */
func CreateRoute(routeName string) {

	// Chargement des variables d'environnement depuis le fichier .env
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Printf("Erreur de chargement du fichier .env : %v \n", err)
		return
	}

	// Récupèration des variables d'environnement
	RouteFolder := os.Getenv("ROUTE_FOLDER")
	HandlerFolder := os.Getenv("HANDLER_FOLDER")
	MainFile := os.Getenv("MAIN_FILE")

	// fmt.Println(string(RouteFolder))
	// fmt.Println(string(HandlerFolder))
	if DoesFolderExist(RouteFolder) && DoesFolderExist(HandlerFolder) { //Si les dossier requis existent

		/* -- Création du fichier route -- */
		newRouteFile := RouteFolder + routeName + ".go"

		routeFile, routeFileError := os.Create(newRouteFile)
		if routeFileError != nil {
			fmt.Printf("Erreur lors de la création de la route: %v \n", routeFileError)
			return
		}
		defer routeFile.Close()

		//Contenu du fichier route
		routeFileContent := `package routes

import (
	"net/http"
	"myApi/handlers" // Importation du handler
)

//Définit la route et appelle le handler associé
func ` + routeName + `Route(mux *http.ServeMux) {
	mux.HandleFunc("/` + routeName + `", handlers.HelloHandler)
}
`
		routeFile.WriteString(routeFileContent)

		/* -- Création du fichier handler -- */
		newHandlerFile := HandlerFolder + routeName + "Handler.go"

		handlerFile, handlerFileError := os.Create(newHandlerFile)
		if handlerFileError != nil {
			fmt.Printf("Erreur lors de la création du handler: %v \n", handlerFileError)
			return
		}
		defer handlerFile.Close()

		//Contenu du fichier handler
		handlerFileContent := `package handlers

import (
	"fmt"
	"net/http"
)

//Gère les requêtes pour la route associée
func ` + routeName + `Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	_, err := w.Write([]byte(` + "`" + `{"message": "Bienvenue sur la route hello !"}` + "`" + `))
	if err != nil {
		fmt.Printf("Erreur lors de l'écriture de la réponse: %v \n", err)
	}
}
`
		handlerFile.WriteString(handlerFileContent)

		/* --- Modification du fichier main.go --- */
		mainFileContent, err := os.ReadFile(MainFile) //Lecture du fichier
		if err != nil {
			fmt.Printf("Erreur lors de la lecture du fichier main.go: %v \n", err)
		}

		// Convertion du contenu en chaîne de caractères
		contentStr := string(mainFileContent)

		/* Ajout des imports */
		importsToAdd := []string{ //Liste des imports à ajouter
			`"net/http"`,
			`"myApi/routes"`,
		}

		// Vérifie si chaque import existe déjà
		for _, imp := range importsToAdd {
			if !strings.Contains(contentStr, imp) {
				// Si l'import n'existe pas, l'ajouter après "import ("
				contentStr = strings.Replace(contentStr, "import (", "import (\n\t"+imp, 1)
			}
		}

		/* Ajout de l'import */
		if !strings.Contains(contentStr, "routes."+routeName+"Route(mux)") {
			mainFuncPos := strings.Index(contentStr, "func main()") + len("func main()")

			// Code à ajouter dans la fonction main
			codeToAdd := `
	routes.` + routeName + `Route(mux)
`
			// Ajout du code juste après la déclaration de la fonction main
			contentStr = contentStr[:mainFuncPos] + "\n" + codeToAdd + contentStr[mainFuncPos:]

			//Réécris le fichier avec le contenu modifié
			err = os.WriteFile(MainFile, []byte(contentStr), 0644)
			// fmt.Printf("Erreur lors de la modification du fichier main.go: %v\n", err)
		}
		/* --- Fin modification du fichier main.go --- */
	} else {
		fmt.Println("Les dossiers `routes` et `handlers` n'existent pas.")
		fmt.Println("Veuillez entrer la commande suivante: yogourt init projectName")
	}
}
