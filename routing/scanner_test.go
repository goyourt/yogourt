package routing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoutePathForDynamicBracketSegment(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "users", "[id]", "index.go")
	got := routePathFor(base, full)
	want := "/api/users/:id"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestRoutePathForStaticAndLegacyDynamicSegments(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "posts", "_slug", "comments", "[commentId]", "handler.go")
	got := routePathFor(base, full)
	want := "/api/posts/:slug/comments/:commentId"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestRoutePathForRootAPIFile(t *testing.T) {
	base := filepath.Join("project", "api")
	full := filepath.Join(base, "index.go")
	got := routePathFor(base, full)
	want := "/api"
	if got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestIsGoFileExcludesTestFiles(t *testing.T) {
	cases := map[string]bool{
		"index.go":      true,
		"user.go":       true,
		"user_test.go":  false,
		"index_test.go": false,
		"readme.md":     false,
	}

	for name, want := range cases {
		if got := isGoFile(name); got != want {
			t.Errorf("isGoFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestWalkGoFilesExcludesTestFiles(t *testing.T) {
	root := t.TempDir()
	writeRouteFile(t, root, "index.go")
	writeRouteFile(t, root, "index_test.go")
	writeRouteFile(t, root, filepath.Join("users", "[id]", "user.go"))
	writeRouteFile(t, root, filepath.Join("users", "[id]", "user_test.go"))

	files, err := walkGoFiles(root)
	if err != nil {
		t.Fatalf("walkGoFiles returned error: %v", err)
	}

	want := map[string]bool{
		filepath.Join(root, "index.go"):                 false,
		filepath.Join(root, "users", "[id]", "user.go"): false,
	}
	if len(files) != len(want) {
		t.Fatalf("expected %d files, got %d: %#v", len(want), len(files), files)
	}
	for _, f := range files {
		if _, ok := want[f]; !ok {
			t.Fatalf("unexpected file returned: %s", f)
		}
		want[f] = true
	}
	for f, found := range want {
		if !found {
			t.Fatalf("expected file not found: %s", f)
		}
	}
}

func writeRouteFile(t *testing.T, root, rel string) {
	t.Helper()

	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
