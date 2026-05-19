package services

import (
	"strconv"

	"github.com/goyourt/yogourt/services/providers"
)

func GetBaseUrl() string {
	cfg := providers.GetMainConfig()
	host := cfg.Server.Host
	if host == "" {
		return "http://localhost:" + strconv.Itoa(cfg.Server.Port)
	}

	return host
}
