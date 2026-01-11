package services

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const ConfigPath = "./config.yaml"

var configData *Config

// Config Structure of config file
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
		DB       string `yaml:"db"`
	} `yaml:"database"`

	Cache struct {
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		Password string `yaml:"password"`
		DB       int    `yaml:"db"`
	} `yaml:"cache"`

	Paths struct {
		ModelFolder string `yaml:"model_folder"`
		ProjectName string `yaml:"project_name"`
		MainFile    string `yaml:"main_file"`
		RouteFolder string `yaml:"route_folder"`
	} `yaml:"paths"`

	Security struct {
		SecretKey    string `yaml:"secret_key"`
		TokenExpires int    `yaml:"token_expires"`
	} `yaml:"security"`

	CORS struct {
		AllowedOrigins   []string      `yaml:"allowed_origins"`
		AllowedMethods   []string      `yaml:"allowed_methods"`
		AllowedHeaders   []string      `yaml:"allowed_headers"`
		AllowCredentials bool          `yaml:"allow_credentials"`
		MaxAge           time.Duration `yaml:"max_age"`
	} `yaml:"cors"`
}

// read and parse the config.yaml file
func loadConfig() error {

	file, err := os.ReadFile(ConfigPath)
	if err != nil {
		return fmt.Errorf("❌ Impossible de lire config.yaml : %v", err)
	}

	var cfg Config
	err = yaml.Unmarshal(file, &cfg)
	if err != nil {
		return fmt.Errorf("❌ Erreur de parsing YAML : %v", err)
	}

	configData = &cfg
	return nil
}

func GetConfig() *Config {
	if configData == nil {
		err := loadConfig()
		if err != nil {
			panic(err)
		}
	}
	return configData
}
