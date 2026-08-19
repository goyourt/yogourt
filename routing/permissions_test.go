package routing

import (
	"strings"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

// assertEffective checks the effective permission map returned by the
// validation against the expected "<permission> (<origin>)" per method.
func assertEffective(t *testing.T, got map[string]methodPermission, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d effective permissions, got %d: %+v", len(want), len(got), got)
	}
	for method, expected := range want {
		permission, ok := got[method]
		if !ok {
			t.Errorf("no effective permission for %s: %+v", method, got)

			continue
		}
		if actual := permission.permission + " (" + permission.origin() + ")"; actual != expected {
			t.Errorf("effective permission for %s = %q, want %q", method, actual, expected)
		}
	}
}

func TestValidateGroupMissingSymbolDerivesEveryMethod(t *testing.T) {
	// A folder without any Permissions symbol is valid: the convention covers
	// every exported method.
	files := []routeFile{
		{file: "users/index.go", methods: []string{"GET", "POST"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a folder relying on the convention, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{
		"GET":  "users.read (derived)",
		"POST": "users.create (derived)",
	})
}

func TestValidateGroupNonDerivableMethodStillRequiresADeclaration(t *testing.T) {
	// The "/api" route has no resource segment: nothing can be derived and the
	// violation must name the method left without a permission.
	files := []routeFile{
		{file: "index.go", methods: []string{"GET"}},
	}

	_, violations := validateRoutePermissionGroup("/api", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "index.go" || violations[0].Method != "GET" {
		t.Errorf("unexpected violation location: %+v", violations[0])
	}
	if !strings.Contains(violations[0].Problem, "no permission can be derived") {
		t.Errorf("unexpected problem: %q", violations[0].Problem)
	}
}

func TestValidateGroupUnknownHTTPMethodIsNotDerivable(t *testing.T) {
	files := []routeFile{
		{file: "users/index.go", methods: []string{"GET", "HEAD"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "HEAD" || !strings.Contains(violations[0].Problem, "no permission can be derived") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
	assertEffective(t, effective, map[string]string{"GET": "users.read (derived)"})
}

func TestValidateGroupNonDerivableMethodCoveredByADeclaration(t *testing.T) {
	// An explicit declaration is the escape hatch for the non-derivable cases.
	files := []routeFile{
		{file: "index.go", methods: []string{"GET", "HEAD"}, hasSymbol: true, permissions: map[string]string{
			"GET":  "root.read",
			"HEAD": authorization.Public,
		}},
	}

	effective, violations := validateRoutePermissionGroup("/api", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{
		"GET":  "root.read (declared)",
		"HEAD": authorization.Public + " (declared)",
	})
}

func TestValidateGroupUtilityFolderWithoutHandlers(t *testing.T) {
	// A folder exporting no HTTP handler registers no route: it must not be
	// forced to declare a Permissions symbol.
	files := []routeFile{{file: "shared/helpers.go"}}

	effective, violations := validateRoutePermissionGroup("/api/shared", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a handler-less folder, got %v", violations)
	}
	if len(effective) != 0 {
		t.Fatalf("expected no effective permission for a handler-less folder, got %+v", effective)
	}
}

func TestValidateGroupSingleDeclarationCoversAllFiles(t *testing.T) {
	// Several files serve the same route (the URL comes from the folder): ONE
	// file may override the convention for every method of the folder.
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{
			"GET":  "user.read",
			"POST": "user.create",
		}},
		{file: "users/test.go", methods: []string{"POST"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a single covering declaration, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{
		"GET":  "user.read (declared)",
		"POST": "user.create (declared)",
	})
}

func TestValidateGroupMultipleDeclarationsRefused(t *testing.T) {
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "user.read"}},
		{file: "users/test.go", methods: []string{"POST"}, hasSymbol: true, permissions: map[string]string{"POST": "user.create"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	problem := violations[0].Problem
	if !strings.Contains(problem, "single file") || !strings.Contains(problem, "users/users.go") || !strings.Contains(problem, "users/test.go") {
		t.Errorf("the violation must name every declaring file: %+v", violations[0])
	}
	if len(effective) != 0 {
		t.Fatalf("an ambiguous route must resolve no permission, got %+v", effective)
	}
}

func TestValidateGroupPartialOverrideDerivesTheRest(t *testing.T) {
	// The map is an override, partial by nature: a method absent from it keeps
	// its derived permission instead of being a violation.
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "user.read"}},
		{file: "users/test.go", methods: []string{"POST"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a partial override, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{
		"GET":  "user.read (declared)",
		"POST": "users.create (derived)",
	})
}

func TestValidateGroupOverrideCanBePublic(t *testing.T) {
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET", "POST"}, hasSymbol: true, permissions: map[string]string{
			"GET": authorization.Public,
		}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, []authorization.Action{"users.create"})

	if len(violations) != 0 {
		t.Fatalf("expected no violation, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{
		"GET":  authorization.Public + " (declared)",
		"POST": "users.create (derived)",
	})
}

func TestValidateGroupOrphanEntry(t *testing.T) {
	files := []routeFile{
		{file: "users/index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{
			"GET":    "article.read",
			"DELETE": "article.delete",
		}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "users/index.go" || violations[0].Method != "DELETE" || !strings.Contains(violations[0].Problem, "no exported handler") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
	assertEffective(t, effective, map[string]string{"GET": "article.read (declared)"})
}

func TestValidateGroupEmptyPermission(t *testing.T) {
	// An empty permission can never be granted and would turn every request
	// into a 500: it must be a boot violation even without a strict list.
	files := []routeFile{
		{file: "users/index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": ""}},
	}

	effective, violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, "empty permission") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
	if len(effective) != 0 {
		t.Fatalf("an invalid method must resolve no permission, got %+v", effective)
	}
}

func TestValidateGroupUnknownDeclaredPermission(t *testing.T) {
	files := []routeFile{
		{file: "articles/index.go", methods: []string{"GET", "POST"}, hasSymbol: true, permissions: map[string]string{
			"GET":  "article.reed",
			"POST": authorization.Public,
		}},
	}
	known := []authorization.Action{"article.read"}

	_, violations := validateRoutePermissionGroup("/api/articles", files, known)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, `unknown permission "article.reed"`) {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateGroupUnknownDerivedPermission(t *testing.T) {
	// Strict mode applies to derived permissions too: a folder whose
	// convention lands outside the known list must not boot silently.
	files := []routeFile{
		{file: "articles/index.go", methods: []string{"GET", "POST"}},
	}
	known := []authorization.Action{"articles.read"}

	effective, violations := validateRoutePermissionGroup("/api/articles", files, known)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "articles/index.go" || violations[0].Method != "POST" {
		t.Errorf("unexpected violation location: %+v", violations[0])
	}
	if !strings.Contains(violations[0].Problem, `unknown permission "articles.create"`) || !strings.Contains(violations[0].Problem, "derived") {
		t.Errorf("the violation must name the derived permission: %+v", violations[0])
	}
	assertEffective(t, effective, map[string]string{"GET": "articles.read (derived)"})
}

func TestValidateGroupNoStrictModeWithoutKnownList(t *testing.T) {
	files := []routeFile{
		{file: "articles/index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "anything.goes"}},
	}

	effective, violations := validateRoutePermissionGroup("/api/articles", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation without a known permission list, got %v", violations)
	}
	assertEffective(t, effective, map[string]string{"GET": "anything.goes (declared)"})
}

func TestValidateGroupCollectsAllViolations(t *testing.T) {
	files := []routeFile{
		{file: "a.go", methods: []string{"GET", "POST", "HEAD"}, hasSymbol: true, permissions: map[string]string{
			"GET": "article.reed", // unknown override
			"PUT": "article.read", // orphan entry
		}},
	}
	known := []authorization.Action{"article.read"}

	effective, violations := validateRoutePermissionGroup("/api/articles", files, known)

	// GET has an unknown override, POST derives an unknown permission, HEAD
	// cannot be derived at all and PUT is an orphan entry.
	if len(violations) != 4 {
		t.Fatalf("expected 4 violations, got %d: %v", len(violations), violations)
	}
	if len(effective) != 0 {
		t.Fatalf("no method is valid here, got %+v", effective)
	}
}

func TestPermissionReportFormat(t *testing.T) {
	violations := []routeViolation{
		{File: "b.go", Method: "GET", Problem: "empty permission declared"},
		{File: "a.go", Method: "HEAD", Problem: "no permission can be derived for HEAD /api/users"},
	}

	report := permissionReport(violations)

	lines := strings.Split(report, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected a header and one line per violation, got %q", report)
	}
	// Lines are sorted for a deterministic report despite concurrent loading.
	if lines[1] != "a.go: HEAD: no permission can be derived for HEAD /api/users" {
		t.Errorf("unexpected first line: %q", lines[1])
	}
	if lines[2] != "b.go: GET: empty permission declared" {
		t.Errorf("unexpected second line: %q", lines[2])
	}
}
