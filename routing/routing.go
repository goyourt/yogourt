package routing

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/middleware"
	"github.com/goyourt/yogourt/services/providers"
)

const compiledRootFolder = ".yogourt"

func Initialize(apiFolder string) {
	basePath, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
		return
	}
	apiFolder = filepath.Join(basePath, apiFolder)

	if _, err := os.Stat(apiFolder); os.IsNotExist(err) {
		fmt.Println("API folder not found at " + apiFolder)
		return
	}

	r := gin.Default()

	corsConfig := providers.GetConfig().CORS

	if len(corsConfig.AllowedOrigins) == 0 && !corsConfig.AllowAllOrigins {
		corsConfig.AllowAllOrigins = true
	}

	r.Use(cors.New(cors.Config{
		AllowOrigins:     corsConfig.AllowedOrigins,
		AllowMethods:     corsConfig.AllowedMethods,
		AllowHeaders:     corsConfig.AllowedHeaders,
		AllowCredentials: corsConfig.AllowCredentials,
		MaxAge:           corsConfig.MaxAge * time.Hour,
	}))

	r.OPTIONS("/*path", func(c *gin.Context) {
		c.AbortWithStatus(204)
	})

	err = middleware.LoadMiddlewares(basePath)
	if err != nil {
		if tryRestartOnStalePlugin(err, basePath) {
			return
		}
		log.Fatal("Error loading middlewares: ", err)
		return
	}
	if err = loadAPIHandlers(r, apiFolder); err != nil {
		if tryRestartOnStalePlugin(err, basePath) {
			return
		}
		log.Fatal("Error loading handlers: ", err)
		return
	}

	r.Run("0.0.0.0:" + strconv.Itoa(providers.GetConfig().Server.Port))
}

const stalePluginRestartCountEnv = "YOGOURT_STALE_PLUGIN_RESTART_COUNT"
const stalePluginRestartMax = 10
const stalePluginRestartDelay = 500 * time.Millisecond

func tryRestartOnStalePlugin(err error, basePath string) bool {
	if !compiler.IsStalePluginVersionError(err) {
		return false
	}

	restartCount, _ := strconv.Atoi(os.Getenv(stalePluginRestartCountEnv))
	if restartCount >= stalePluginRestartMax {
		log.Printf(
			"Stale plugin detected but restart limit reached (%d/%d): %v",
			restartCount,
			stalePluginRestartMax,
			err,
		)
		return false
	}

	if soPath := compiler.ExtractPluginPath(err); soPath != "" {
		targets := make([]string, 0, 4)
		targets = append(targets, soPath)
		if filepath.Ext(soPath) != ".so" {
			targets = append(targets, soPath+".so")
		}
		if !filepath.IsAbs(soPath) {
			abs := filepath.Join(basePath, soPath)
			targets = append(targets, abs)
			if filepath.Ext(abs) != ".so" {
				targets = append(targets, abs+".so")
			}
		}

		removed := false
		for _, target := range targets {
			if rmErr := os.Remove(target); rmErr == nil {
				log.Printf("Removed stale plugin before restart: %s", target)
				removed = true
				break
			} else if !os.IsNotExist(rmErr) {
				log.Printf("Failed to remove stale plugin %s before restart: %v", target, rmErr)
			}
		}
		if !removed {
			log.Printf("No stale plugin file removed before restart (extracted path: %s)", soPath)
		}
	}

	execPath, pathErr := os.Executable()
	if pathErr != nil {
		log.Printf("Unable to resolve executable path for restart: %v", pathErr)
		return false
	}

	argv := append([]string{execPath}, os.Args[1:]...)
	env := setEnvVar(os.Environ(), stalePluginRestartCountEnv, strconv.Itoa(restartCount+1))

	log.Printf(
		"Stale plugin detected, restarting process in-place (%d/%d) after %s",
		restartCount+1,
		stalePluginRestartMax,
		stalePluginRestartDelay,
	)
	time.Sleep(stalePluginRestartDelay)

	if execErr := syscall.Exec(execPath, argv, env); execErr != nil {
		log.Printf("Unable to exec process restart after stale plugin error: %v", execErr)
		return false
	}
	return true
}

func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(env)+1)
	replaced := false

	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			if !replaced {
				updated = append(updated, prefix+value)
				replaced = true
			}
			continue
		}
		updated = append(updated, kv)
	}

	if !replaced {
		updated = append(updated, prefix+value)
	}

	return updated
}
