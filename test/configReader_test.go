package test

import (
	"testing"

	"github.com/goyourt/yogourt/services"
)

func TestConfigReader(t *testing.T) {
	cfg := services.GetConfig()
	if cfg == nil {
		t.Errorf("Config file not found")
		return
	}

	if cfg.AppName != "test-app" {
		t.Errorf("App name not found")
	}
	if cfg.Security.SecretKey != "secret_key" {
		t.Errorf("Security config not found")
	}
	if cfg.Database.Port != 1000 {
		t.Errorf("Database config not found")
	}
	if cfg.Paths.ModelFolder != "model_folder" {
		t.Errorf("Paths config not found")
	}
	if cfg.Server.CORS != true {
		t.Errorf("Server config not found")
	}
	if cfg.Cache.DB != 1000 {
		t.Errorf("Cache config not found")
	}
}
