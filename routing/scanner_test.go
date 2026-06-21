package routing

import (
	"path/filepath"
	"testing"
)

func TestRoutePathForDynamicBracketSegment(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "users", "[id]", "index.go")
	got := routePathFor(base, full, "index.go")
	want := "/api/users/:id"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestRoutePathForStaticAndLegacyDynamicSegments(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "posts", "_slug", "comments", "[commentId]", "handler.go")
	got := routePathFor(base, full, "handler.go")
	want := "/api/posts/:slug/comments/:commentId"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestRoutePathForRootAPIFile(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "index.go")
	got := routePathFor(base, full, "index.go")
	want := "/api"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}
