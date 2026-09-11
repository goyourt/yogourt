package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const mainConfigPath = "./configs/yogourt.yaml"

const (
	// DefaultMaxRequestBodyBytes is large enough for ordinary JSON and file
	// uploads while still bounding multipart parsing when the key is absent.
	DefaultMaxRequestBodyBytes int64 = 32 << 20
	// MaxRequestBodyBytes prevents a configuration mistake from effectively
	// removing the backend limit. Larger transfers need a dedicated path.
	MaxRequestBodyBytes int64 = 1 << 30
)

var (
	configOnce sync.Once
	configData *MainConfig
	envOnce    sync.Once
	envErr     error
)

// MainConfig Structure of yogourt config file
type MainConfig struct {
	AppName  string   `yaml:"app_name"`
	Version  string   `yaml:"version"`
	Mode     string   `yaml:"mode"`
	EnvFiles EnvFiles `yaml:"env_files"`

	Server ServerConfig `yaml:"server"`

	Database DatabaseConfig `yaml:"database"`

	Cache CacheConfig `yaml:"cache"`

	// Paths carries the one path the runtime needs: RouteFolder, the folder
	// of the route tree. model_folder, project_name and main_file used to be
	// declared here and were read by nothing; they belong to the v0.5 CLI and
	// are now reported by removedMainConfigKeys instead of being parsed into
	// a field nobody reads.
	Paths struct {
		RouteFolder string `yaml:"route_folder"`
	} `yaml:"paths"`

	Security struct {
		SecretKey                   string `yaml:"secret_key"`
		TokenIssuer                 string `yaml:"token_issuer"`
		TokenAudience               string `yaml:"token_audience"`
		HashCost                    int    `yaml:"hash_cost"`
		TokenExpires                int    `yaml:"token_expires"`
		PasswordMinimumLength       int    `yaml:"password_minimum_length"`
		PasswordSpacialCharRequired bool   `yaml:"password_special_char_required"`
		PasswordNumberRequired      bool   `yaml:"password_number_required"`
		PasswordUpperCaseRequired   bool   `yaml:"password_upper_case_required"`
		PasswordLowerCaseRequired   bool   `yaml:"password_lower_case_required"`
	} `yaml:"security"`

	CORS struct {
		AllowedOrigins   []string `yaml:"allowed_origins"`
		AllowedMethods   []string `yaml:"allowed_methods"`
		AllowedHeaders   []string `yaml:"allowed_headers"`
		AllowCredentials bool     `yaml:"allow_credentials"`
		AllowAllOrigins  bool     `yaml:"allow_all_origins"`
		MaxAge           Duration `yaml:"max_age"`
	} `yaml:"cors"`
}

// ServerConfig controls the HTTP listener and the request budgets enforced by
// the framework. Zero values select bounded defaults; they never disable a
// timeout or a size limit.
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
	// CORS switches the CORS middleware and the preflight catch-all on
	// or off. A pointer, because the absent key and an explicit false
	// must not mean the same thing: an application that never wrote the
	// key keeps the middleware it has always had, and only "cors: false"
	// removes it.
	CORS *bool `yaml:"cors"`
	// BasePath is the HTTP prefix every route is published under. Empty
	// falls back to routing.DefaultPrefix ("/api").
	BasePath string `yaml:"base_path"`
	// BaseURL is the public URL the application is reachable at, used by
	// services.GetBaseUrl. Host is a listening address — "0.0.0.0" tells
	// the socket to accept every interface and says nothing about how a
	// client reaches the application — so a deployment behind a reverse
	// proxy, a TLS terminator or a container port mapping declares its
	// public URL here. Empty rebuilds it from Host and Port.
	BaseURL           string   `yaml:"base_url"`
	TrustedProxies    []string `yaml:"trusted_proxies"`
	ReadHeaderTimeout Duration `yaml:"read_header_timeout"`
	ReadTimeout       Duration `yaml:"read_timeout"`
	WriteTimeout      Duration `yaml:"write_timeout"`
	IdleTimeout       Duration `yaml:"idle_timeout"`
	MaxHeaderBytes    int      `yaml:"max_header_bytes"`
	MaxBodyBytes      int64    `yaml:"max_body_bytes"`
}

// CacheConfig controls the Redis connection. TLS uses Go's normal certificate
// and hostname verification and can add an application CA or a client
// certificate; no insecure verification switch is exposed.
type CacheConfig struct {
	Host     string         `yaml:"host"`
	Port     string         `yaml:"port"`
	Password string         `yaml:"password"`
	DB       int            `yaml:"db"`
	TLS      CacheTLSConfig `yaml:"tls"`
}

type CacheTLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ServerName string `yaml:"server_name"`
	CAFile     string `yaml:"ca_file"`
	CertFile   string `yaml:"cert_file"`
	KeyFile    string `yaml:"key_file"`
}

// RequestBodyLimit resolves server.max_body_bytes. Zero selects the bounded
// default; negative values and values above the framework ceiling are invalid.
func (c ServerConfig) RequestBodyLimit() (int64, error) {
	switch {
	case c.MaxBodyBytes == 0:
		return DefaultMaxRequestBodyBytes, nil
	case c.MaxBodyBytes < 0:
		return 0, fmt.Errorf("server.max_body_bytes must be positive")
	case c.MaxBodyBytes > MaxRequestBodyBytes:
		return 0, fmt.Errorf("server.max_body_bytes must not exceed %d", MaxRequestBodyBytes)
	default:
		return c.MaxBodyBytes, nil
	}
}

// DatabaseConfig is the database section of the configuration. It is a named
// type, not an anonymous struct like its neighbours, so the DSN and pool
// helpers of the provider can take it as a parameter and be tested without
// the global config.
type DatabaseConfig struct {
	Type     string `yaml:"type"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	DB       string `yaml:"db"`
	// SSLMode is the libpq sslmode of the connection. Network connections
	// default to "verify-full" and cannot select a weaker mode. A local Unix
	// socket may use "disable" only with AllowInsecureLocalSocket.
	SSLMode string `yaml:"ssl_mode"`
	// SSLRootCert, SSLCert and SSLKey are the paths libpq needs beyond
	// sslmode: the CA bundle "verify-ca" and "verify-full" check the server
	// against, and the client certificate a server asking for one expects.
	// Empty fields stay out of the DSN, so libpq keeps its own defaults
	// (~/.postgresql/root.crt and friends).
	SSLRootCert string `yaml:"ssl_root_cert"`
	SSLCert     string `yaml:"ssl_cert"`
	SSLKey      string `yaml:"ssl_key"`
	// AllowInsecureLocalSocket is the explicit clear-text exception for a
	// explicitly configured local Unix-domain socket. TCP connections and an
	// absent host require verify-full.
	AllowInsecureLocalSocket bool `yaml:"allow_insecure_local_socket"`
	// SearchPath is the schema search path of every session opened by the
	// pool. Empty leaves the server default ("$user", public), so a
	// deployment holding its tables in a named schema no longer has to
	// qualify every model.
	SearchPath string `yaml:"search_path"`
	// Pool bounds the underlying database/sql pool.
	Pool DatabasePoolConfig `yaml:"pool"`
}

// DatabasePoolConfig bounds the database/sql pool behind GORM. A zero field
// keeps the default of database/sql — unlimited open connections, two idle
// ones, no expiry — which is what the provider has always used. The two
// durations accept a duration string ("30m") or a number of seconds.
type DatabasePoolConfig struct {
	MaxOpenConns    int      `yaml:"max_open_conns"`
	MaxIdleConns    int      `yaml:"max_idle_conns"`
	ConnMaxLifetime Duration `yaml:"conn_max_lifetime"`
	ConnMaxIdleTime Duration `yaml:"conn_max_idle_time"`
}

// read and parse the config.yaml file
//TODO take default values into account if there is one ${envVar:-defaultValue} actually only ${envVar} is supported

func loadConfig(filePath string, cfg any) error {

	file, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("Impossible to read config file %s : %v", filePath, err)
	}

	if err := loadConfiguredEnv(filePath, file); err != nil {
		return err
	}

	replaced := os.ExpandEnv(string(file))

	err = yaml.Unmarshal([]byte(replaced), cfg)
	if err != nil {
		return fmt.Errorf("Error parsing YAML : %v", err)
	}

	warnAboutDeadConfigKeys(filePath, []byte(replaced), cfg)

	return nil
}

// Duration is a duration read from YAML, written either as a Go duration
// string ("12h", "300ms") or as a bare number of seconds — the unit of the
// HTTP headers these values end up in.
//
// yaml.v3 decodes time.Duration from duration strings only, so a plain
// max_age: 3600 used to stop the boot on a decoding error, and the only way
// to obtain a usable value was to work around a conversion bug.
type Duration time.Duration

// Duration returns the value as a time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("a duration must be a number of seconds (3600) or a duration string (12h)")
	}

	raw := strings.TrimSpace(value.Value)
	if raw == "" {
		*d = 0
		return nil
	}

	// A bare number is seconds: it is what Access-Control-Max-Age and its
	// kind are expressed in, and what a reader writing 3600 means.
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		*d = Duration(seconds * float64(time.Second))
		return nil
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%q is not a duration: write a number of seconds (3600) or a duration string (12h, 300ms)", raw)
	}

	*d = Duration(parsed)
	return nil
}

type envFileConfig struct {
	EnvFiles EnvFiles `yaml:"env_files"`
}

type EnvFiles []string

func (e *EnvFiles) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		envFile := strings.TrimSpace(value.Value)
		if envFile == "" {
			*e = nil
			return nil
		}
		*e = []string{envFile}
	case yaml.SequenceNode:
		var envFiles []string
		for _, node := range value.Content {
			envFile := strings.TrimSpace(node.Value)
			if envFile != "" {
				envFiles = append(envFiles, envFile)
			}
		}
		*e = envFiles
	default:
		return fmt.Errorf("env_files must be a string or a list of strings")
	}

	return nil
}

func loadConfiguredEnv(configPath string, configContent []byte) error {
	envOnce.Do(func() {
		envFiles, err := envFilePaths(configPath, configContent)
		if err != nil {
			envErr = err
			return
		}
		if len(envFiles) == 0 && filepath.Clean(configPath) != filepath.Clean(mainConfigPath) {
			envFiles, err = envFilePathsFromMainConfig()
			if err != nil {
				envErr = err
				return
			}
		}
		envErr = loadEnvFiles(envFiles)
	})

	return envErr
}

func envFilePaths(configPath string, configContent []byte) ([]string, error) {
	cfg := &envFileConfig{}
	if err := yaml.Unmarshal(configContent, cfg); err != nil {
		return nil, fmt.Errorf("Error parsing YAML env file config from %s : %v", configPath, err)
	}

	if len(cfg.EnvFiles) > 0 {
		return cfg.EnvFiles, nil
	}

	return nil, nil
}

func envFilePathsFromMainConfig() ([]string, error) {
	file, err := os.ReadFile(mainConfigPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Impossible to read config file %s : %v", mainConfigPath, err)
	}

	return envFilePaths(mainConfigPath, file)
}

func loadEnvFiles(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}

	if err := godotenv.Load(filePaths...); err != nil {
		return fmt.Errorf("Impossible to load env files %v : %v", filePaths, err)
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
