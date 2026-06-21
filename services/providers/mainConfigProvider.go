package providers

import (
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const mainConfigPath = "./configs/yogourt.yaml"

var (
	configOnce sync.Once
	configData *MainConfig
)

// MainConfig Structure of yogourt config file
type MainConfig struct {
	AppName string `yaml:"app_name"`
	Version string `yaml:"version"`
	Mode    string `yaml:"mode"`

	Server struct {
		Port int    `yaml:"port"`
		CORS bool   `yaml:"cors"`
		Host string `yaml:"host"`
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
		SecretKey                   string `yaml:"secret_key"`
		HashCost                    int    `yaml:"hash_cost"`
		TokenExpires                int    `yaml:"token_expires"`
		PasswordMinimumLength       int    `yaml:"password_minimum_length"`
		PasswordSpacialCharRequired bool   `yaml:"password_special_char_required"`
		PasswordNumberRequired      bool   `yaml:"password_number_required"`
		PasswordUpperCaseRequired   bool   `yaml:"password_upper_case_required"`
		PasswordLowerCaseRequired   bool   `yaml:"password_lower_case_required"`
	} `yaml:"security"`

	CORS struct {
		AllowedOrigins   []string      `yaml:"allowed_origins"`
		AllowedMethods   []string      `yaml:"allowed_methods"`
		AllowedHeaders   []string      `yaml:"allowed_headers"`
		AllowCredentials bool          `yaml:"allow_credentials"`
		AllowAllOrigins  bool          `yaml:"allow_all_origins"`
		MaxAge           time.Duration `yaml:"max_age"`
	} `yaml:"cors"`
}

// read and parse the config.yaml file
//TODO take default values into account if there is one ${envVar:-defaultValue} actually only ${envVar} is supported

func loadConfig(filePath string, cfg any) error {

	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Impossible to read config file %s : %v", filePath, err)
	}

	replaced := os.ExpandEnv(string(file))

	err = yaml.Unmarshal([]byte(replaced), cfg)
	if err != nil {
		return fmt.Errorf("Error parsing YAML : %v", err)
	}

	return nil
}

func GetMainConfig() *MainConfig {
	configOnce.Do(func() {
		cfg := &MainConfig{}
		err := loadConfig(mainConfigPath, cfg)
		if err != nil {
			panic(err)
		}
		configData = cfg
	})
	return configData
}
