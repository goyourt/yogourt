package routing

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// withRestoredGinMode keeps the global Gin mode from leaking into the other
// tests of the package, which run in test mode.
func withRestoredGinMode(t *testing.T) {
	t.Helper()

	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })
}

// The config mode used to have no effect on Gin: a deployment declaring
// production kept serving with the debug logger and the route dump.
func TestApplyGinModeFollowsTheConfig(t *testing.T) {
	cases := map[string]string{
		"production":  gin.ReleaseMode,
		"PRODUCTION":  gin.ReleaseMode,
		" production": gin.ReleaseMode,
		"test":        gin.TestMode,
		"development": gin.DebugMode,
		"":            gin.DebugMode,
		"whatever":    gin.DebugMode,
	}

	for mode, want := range cases {
		withRestoredGinMode(t)
		t.Setenv(gin.EnvGinMode, "")

		applyGinMode(mode)

		if got := gin.Mode(); got != want {
			t.Errorf("mode %q gave Gin %q, want %q", mode, got, want)
		}
	}
}

// GIN_MODE is Gin's own documented lever and deployments already rely on it:
// the config must not silently override it.
func TestApplyGinModeLetsTheEnvironmentWin(t *testing.T) {
	withRestoredGinMode(t)
	t.Setenv(gin.EnvGinMode, gin.ReleaseMode)
	gin.SetMode(gin.ReleaseMode)

	applyGinMode("development")

	if got := gin.Mode(); got != gin.ReleaseMode {
		t.Errorf("an explicit GIN_MODE must win, got %q", got)
	}
}
