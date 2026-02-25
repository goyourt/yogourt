package routing

import (
	"testing"
)

func TestRoutePathForDynamicBracketSegment(t *testing.T) {
	base := "/project/api"
	full := "/project/api/users/[id]/index.go"
	got := routePathFor(base, full, "index.go")
	want := "/api/users/:id"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}

func TestRoutePathForStaticAndLegacyDynamicSegments(t *testing.T) {
	base := "/project/api"
	full := "/project/api/posts/_slug/comments/[commentId]/handler.go"
	got := routePathFor(base, full, "handler.go")
	want := "/api/posts/:slug/comments/:commentId"
	if got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}
}
