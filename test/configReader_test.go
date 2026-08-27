package test

import (
	"fmt"
	"testing"

	"github.com/goyourt/yogourt/services/providers"
)

func TestConfigReader(t *testing.T) {
	cfg := providers.GetMainConfig()
	if cfg == nil {
		t.Errorf("Config file not found")
		return
	}

	if cfg.AppName != "test-app" {
		t.Errorf("App name not found")
	}
	if cfg.Security.SecretKey != "secret_key_at_least_32_bytes_long!!" {
		t.Errorf("Security config not found")
	}
	if len(cfg.EnvFiles) != 1 || cfg.EnvFiles[0] != "./configs/yogourt.env" {
		t.Errorf("Env files config not found")
	}
	if cfg.Database.Port != 1000 {
		t.Errorf("Database config not found")
	}
	if cfg.Paths.RouteFolder != "route_folder" {
		t.Errorf("Route folder config not found")
	}
	if cfg.Server.CORS == nil || !*cfg.Server.CORS {
		t.Errorf("Server config not found")
	}
	if cfg.Server.BasePath != "/base_path" {
		t.Errorf("Server base path not found")
	}
	if cfg.Server.BaseURL != "https://test-app.example.com" {
		t.Errorf("Server base url not found")
	}
	if cfg.Cache.DB != 1000 {
		t.Errorf("Cache config not found")
	}
}

func TestFileConfigReader(t *testing.T) {
	cfg := providers.GetFileConfig()
	fmt.Println(cfg)
	if cfg == nil || cfg.FileFolder == nil {
		t.Errorf("Config file not found")
		return
	}

	if *cfg.FileFolder != "./public/files/" {
		t.Errorf("FileFolder not found")
	}
	if *cfg.MaxFileSize != 5242880 {
		t.Errorf("MaxFileSize not found")
	}

	scripts := providers.GetConfigByFileType("scripts")
	images := providers.GetConfigByFileType("images")
	tests := providers.GetConfigByFileType("tests")
	null := providers.GetConfigByFileType("null")

	if *scripts.MaxFileSize != 2621440 || *scripts.FileFolder != "/var/tmp/" {
		t.Errorf("Incorect config for scripts")
	}
	if *images.MaxFileSize != 10485760 || *images.FileFolder != *cfg.FileFolder {
		t.Errorf("Incorect config for images")
	}
	if *tests.MaxFileSize != *cfg.MaxFileSize || *tests.FileFolder != "/test" {
		t.Errorf("Incorect config for tests")
	}
	if *null.MaxFileSize != *cfg.MaxFileSize || *null.FileFolder != *cfg.FileFolder {
		t.Errorf("Incorect config for null")
	}
}
