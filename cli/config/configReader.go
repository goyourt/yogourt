package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// Config Structure du fichier config
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

	Paths struct {
		ModelFolder string `yaml:"model_folder"`
		ProjectName string `yaml:"project_name"`
		MainFile    string `yaml:"main_file"`
		RouteFolder string `yaml:"route_folder"`
	} `yaml:"paths"`
}

// LoadConfig Lecture et parse du fichier config
func LoadConfig(path string) (*Config, error) {

	file, err := os.ReadFile(path)
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
