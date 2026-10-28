package services

import (
	"net"
	"strconv"
	"strings"

	"github.com/goyourt/yogourt/services/providers"
)

// defaultBaseURLScheme is the scheme given to a base URL rebuilt from
// server.host and server.port, and to a server.base_url written without one.
const defaultBaseURLScheme = "http"

// GetBaseUrl returns the URL the application is reachable at, scheme
// included.
//
// server.base_url wins when it is set: it is the only value that can describe
// a public address the process knows nothing about — a reverse proxy, a TLS
// terminator, a port mapped by a container. Without it the URL is rebuilt
// from server.host and server.port, which describe the listening socket.
func GetBaseUrl() string {
	cfg := providers.GetMainConfig()
	return baseURL(cfg.Server.BaseURL, cfg.Server.Host, cfg.Server.Port)
}

// baseURL builds the base URL from the configured public URL, or from the
// listening address when there is none.
func baseURL(configured, host string, port int) string {
	if configured = strings.TrimSpace(configured); configured != "" {
		return normalizeBaseURL(configured)
	}

	return defaultBaseURLScheme + "://" + authority(host, port)
}

// normalizeBaseURL completes a configured base URL: a value written without a
// scheme ("example.com", "example.com:8080") is served over HTTP, and a
// trailing slash is dropped so callers can concatenate a path directly.
func normalizeBaseURL(configured string) string {
	if !strings.Contains(configured, "://") {
		configured = defaultBaseURLScheme + "://" + configured
	}

	return strings.TrimRight(configured, "/")
}

// authority returns the "host:port" part of a URL rebuilt from the listening
// address. A host that is empty or unspecified — 0.0.0.0, :: — is an
// instruction to listen on every interface, not an address a client can dial,
// so it becomes localhost; an IPv6 literal is bracketed.
func authority(host string, port int) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

	if host == "" {
		host = "localhost"
	} else if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "localhost"
	}

	if port <= 0 {
		if strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}
