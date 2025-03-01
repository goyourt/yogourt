package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v2"
)

// Structure du fichier config
type Config struct {
	AppName string `yaml:"app_name"`
	Version string `yaml:"version"`
	Mode    string `yaml:"mode"`

	Server struct {
		Port int  `yaml:"port"`
		CORS bool `yaml:"cors"`
	} `yaml:"server"`

	Database struct {
		Type     string `yaml:"type"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		DBName   string `yaml:"dbname"`
	} `yaml:"database"`
}

// Lecture et parse du fichier config
func LoadConfig() (*Config, error) {

	// Chargement du fichier .env
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("❌ Erreur de chargement du fichier .env")
		return nil, err
	}

	// Récupèration des variables d'environnement
	ProjectName := os.Getenv("PROJECT_NAME")

	file, err := os.ReadFile(ProjectName + "/config.yaml")
	if err != nil {
		return nil, fmt.Errorf("❌ Impossible de lire config.yaml : %v", err)
	}

	var cfg Config
	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return nil, fmt.Errorf("❌ Erreur de parsing YAML : %v", err)
	}

	return &cfg, nil
}
