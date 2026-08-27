package services

import "testing"

// The base URL must always be dialable: server.host is a listening address,
// so a wildcard host has no meaning for a client and the rebuilt URL carries
// a scheme and the port in every case.
func TestBaseURLFromListeningAddress(t *testing.T) {
	cases := map[string]struct {
		host string
		port int
		want string
	}{
		"empty host":           {host: "", port: 8080, want: "http://localhost:8080"},
		"wildcard IPv4":        {host: "0.0.0.0", port: 8080, want: "http://localhost:8080"},
		"wildcard IPv6":        {host: "::", port: 8080, want: "http://localhost:8080"},
		"bracketed wildcard":   {host: "[::]", port: 8080, want: "http://localhost:8080"},
		"loopback":             {host: "127.0.0.1", port: 8080, want: "http://127.0.0.1:8080"},
		"host name":            {host: "example.com", port: 8080, want: "http://example.com:8080"},
		"IPv6 literal":         {host: "::1", port: 8080, want: "http://[::1]:8080"},
		"bracketed IPv6":       {host: "[::1]", port: 8080, want: "http://[::1]:8080"},
		"surrounding spaces":   {host: "  127.0.0.1  ", port: 8080, want: "http://127.0.0.1:8080"},
		"no port":              {host: "example.com", port: 0, want: "http://example.com"},
		"IPv6 literal no port": {host: "::1", port: 0, want: "http://[::1]"},
	}

	for name, tc := range cases {
		got := baseURL("", tc.host, tc.port)
		if got != tc.want {
			t.Errorf("%s: baseURL(\"\", %q, %d) = %q, want %q", name, tc.host, tc.port, got, tc.want)
		}
	}
}

// A configured server.base_url describes an address the process cannot infer
// — a reverse proxy, a TLS terminator, a mapped container port — so it wins
// over the listening address.
func TestBaseURLPrefersConfiguredURL(t *testing.T) {
	cases := map[string]struct {
		configured string
		want       string
	}{
		"full URL":        {configured: "https://api.example.com", want: "https://api.example.com"},
		"URL with port":   {configured: "https://api.example.com:8443", want: "https://api.example.com:8443"},
		"trailing slash":  {configured: "https://api.example.com/", want: "https://api.example.com"},
		"missing scheme":  {configured: "api.example.com", want: "http://api.example.com"},
		"scheme and path": {configured: "https://example.com/api/", want: "https://example.com/api"},
		"padded value":    {configured: "  https://api.example.com  ", want: "https://api.example.com"},
	}

	for name, tc := range cases {
		got := baseURL(tc.configured, "0.0.0.0", 8080)
		if got != tc.want {
			t.Errorf("%s: baseURL(%q, ...) = %q, want %q", name, tc.configured, got, tc.want)
		}
	}
}

// A blank server.base_url is the absent key, not a base URL of "".
func TestBaseURLIgnoresBlankConfiguredURL(t *testing.T) {
	got := baseURL("   ", "127.0.0.1", 8080)
	if want := "http://127.0.0.1:8080"; got != want {
		t.Fatalf("baseURL(\"   \", ...) = %q, want %q", got, want)
	}
}
