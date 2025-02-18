package cmd

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

/* Commande serve */
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Démarre le serveur http",
	Long:  "Démarre le serveur http",
	Run: func(cmd *cobra.Command, args []string) {
		RunServer()
	},
}

// Structure pour récupérer la configuration
type Config struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// Fonction pour charger le fichier YAML config
func LoadConfig() (*Config, error) {

	// Chargement du fichier .env
	err := godotenv.Load(".env")
	if err != nil {
		return nil, fmt.Errorf("❌ Erreur de chargement du fichier .env: %v", err)
	}

	// Récupération de la variable depuis le fichier .env
	ProjectName := os.Getenv("PROJECT_NAME")

	file, err := os.ReadFile(ProjectName + "/config.yaml")
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(file, &config) //Convertion du contenu yaml en go
	if err != nil {
		return nil, err
	}

	fmt.Println("Configuration chargée :", config)
	return &config, nil
}

func RunServer() {

	// Chargement du fichier .env
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("❌ Erreur de chargement du fichier .env")
		return
	}

	// Récupération de la variable depuis le fichier .env
	ProjectName := os.Getenv("PROJECT_NAME")

	// Vérifie si config.yaml existe
	if _, err := os.Stat(ProjectName + "/config.yaml"); os.IsNotExist(err) {
		fmt.Println("❌ Fichier config.yaml non trouvé, veuillez entrer la commande suivante: goyourt init project_name")
		return
	} else {

		// Charge le fichier config
		config, err := LoadConfig()
		if err != nil {
			fmt.Println("❌ Erreur lors du chargement du fichier config.yaml:", err)
			return
		}

		// Récupère les valeurs du fichier de config
		host := config.Host
		port := config.Port
		address := fmt.Sprintf("%s:%d", host, port)

		// Création du ServeMux
		mux := http.NewServeMux()

		fmt.Printf("🚀 Serveur démarré sur http://%s\n", address)
		if err := http.ListenAndServe(address, mux); err != nil {
			log.Fatalf("Erreur de démarrage du serveur: %v", err)
		}
	}
}
