package routing

import "testing"

func TestDerivePermissionResourceIsTheLastStaticSegment(t *testing.T) {
	cases := []struct {
		route  string
		method string
		want   string
	}{
		{"/api/users", "GET", "users.read"},
		{"/api/users", "POST", "users.create"},
		{"/api/users/:id", "PUT", "users.update"},
		{"/api/users/:id", "PATCH", "users.update"},
		{"/api/users/:id", "DELETE", "users.delete"},
		{"/api/users/:id/comments", "GET", "comments.read"},
		{"/api/users/:id/comments/:commentId/replies", "DELETE", "replies.delete"},
		// The resource is the last STATIC segment, whatever follows it.
		{"/api/users/:id/comments/:commentId", "PATCH", "comments.update"},
		// Case is normalized so the permission does not depend on the folder name.
		{"/api/Articles/:id", "GET", "articles.read"},
		// Gin catch-alls are dynamic segments too.
		{"/api/files/*path", "GET", "files.read"},
	}

	for _, c := range cases {
		got, ok := derivePermission(c.route, c.method)
		if !ok {
			t.Errorf("derivePermission(%q, %q) could not derive a permission", c.route, c.method)

			continue
		}
		if got != c.want {
			t.Errorf("derivePermission(%q, %q) = %q, want %q", c.route, c.method, got, c.want)
		}
	}
}

func TestDerivePermissionImpossibleCases(t *testing.T) {
	cases := []struct {
		name   string
		route  string
		method string
	}{
		{"root api route has no resource segment", "/api", "GET"},
		{"root api route with a trailing slash", "/api/", "POST"},
		{"a fully dynamic path has no resource segment", "/api/:id", "GET"},
		{"unknown http method has no verb", "/api/users", "HEAD"},
		{"options is not a business verb", "/api/users", "OPTIONS"},
		{"empty method", "/api/users", ""},
	}

	for _, c := range cases {
		if got, ok := derivePermission(c.route, c.method); ok {
			t.Errorf("%s: derivePermission(%q, %q) = %q, want no derivation", c.name, c.route, c.method, got)
		}
	}
}
