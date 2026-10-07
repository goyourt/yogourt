package routing

import (
	"reflect"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

func TestAuthorizationSurfaceLinesSortedByRouteThenMethod(t *testing.T) {
	effective := map[string]map[string]methodPermission{
		"/api/users": {
			"POST": {permission: "user.create"},
			"GET":  {permission: "users.read", derived: true},
		},
		"/api/users/:id/comments": {
			"GET":    {permission: "comments.read", derived: true},
			"DELETE": {permission: authorization.Public},
		},
		"/api/articles": {
			"GET": {permission: "articles.read", derived: true},
		},
	}

	got := authorizationSurfaceLines(effective)

	want := []string{
		"authorization: GET /api/articles -> articles.read (derived)",
		"authorization: GET /api/users -> users.read (derived)",
		"authorization: POST /api/users -> user.create (declared)",
		"authorization: DELETE /api/users/:id/comments -> @public (declared)",
		"authorization: GET /api/users/:id/comments -> comments.read (derived)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unexpected surface lines:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestAuthorizationSurfaceLinesEmptyWithoutPermissions(t *testing.T) {
	if lines := authorizationSurfaceLines(nil); len(lines) != 0 {
		t.Errorf("expected no line for an empty surface, got %#v", lines)
	}
	// A route folder without handler contributes no line.
	if lines := authorizationSurfaceLines(map[string]map[string]methodPermission{"/api/shared": nil}); len(lines) != 0 {
		t.Errorf("expected no line for a handler-less folder, got %#v", lines)
	}
}
