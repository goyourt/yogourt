package routing

import (
	"strings"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

func TestValidateGroupMissingSymbol(t *testing.T) {
	files := []routeFile{
		{file: "users/index.go", methods: []string{"GET", "POST"}},
	}

	violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for a missing symbol, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "/api/users" || violations[0].Method != "Permissions" {
		t.Errorf("unexpected violation location: %+v", violations[0])
	}
	if !strings.Contains(violations[0].Problem, "missing required symbol") {
		t.Errorf("unexpected problem: %q", violations[0].Problem)
	}
}

func TestValidateGroupUtilityFolderWithoutHandlers(t *testing.T) {
	// A folder exporting no HTTP handler registers no route: it must not be
	// forced to declare a Permissions symbol.
	files := []routeFile{{file: "shared/helpers.go"}}

	violations := validateRoutePermissionGroup("/api/shared", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a handler-less folder, got %v", violations)
	}
}

func TestValidateGroupSingleDeclarationCoversAllFiles(t *testing.T) {
	// Several files serve the same route (the URL comes from the folder):
	// ONE file declares the permissions of every method of the folder.
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{
			"GET":  "user.read",
			"POST": "user.create",
		}},
		{file: "users/test.go", methods: []string{"POST"}},
	}

	violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a single covering declaration, got %v", violations)
	}
}

func TestValidateGroupMultipleDeclarationsRefused(t *testing.T) {
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "user.read"}},
		{file: "users/test.go", methods: []string{"POST"}, hasSymbol: true, permissions: map[string]string{"POST": "user.create"}},
	}

	violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	problem := violations[0].Problem
	if !strings.Contains(problem, "single file") || !strings.Contains(problem, "users/users.go") || !strings.Contains(problem, "users/test.go") {
		t.Errorf("the violation must name every declaring file: %+v", violations[0])
	}
}

func TestValidateGroupUndeclaredMethodNamesItsFile(t *testing.T) {
	files := []routeFile{
		{file: "users/users.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "user.read"}},
		{file: "users/test.go", methods: []string{"POST"}},
	}

	violations := validateRoutePermissionGroup("/api/users", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "users/test.go" || violations[0].Method != "POST" || !strings.Contains(violations[0].Problem, "no permission declared") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateGroupOrphanEntry(t *testing.T) {
	files := []routeFile{
		{file: "index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{
			"GET":    "article.read",
			"DELETE": "article.delete",
		}},
	}

	violations := validateRoutePermissionGroup("/api", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "index.go" || violations[0].Method != "DELETE" || !strings.Contains(violations[0].Problem, "no exported handler") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateGroupEmptyPermission(t *testing.T) {
	// An empty permission can never be granted and would turn every request
	// into a 500: it must be a boot violation even without a strict list.
	files := []routeFile{
		{file: "index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": ""}},
	}

	violations := validateRoutePermissionGroup("/api", files, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, "empty permission") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateGroupUnknownPermission(t *testing.T) {
	files := []routeFile{
		{file: "index.go", methods: []string{"GET", "POST"}, hasSymbol: true, permissions: map[string]string{
			"GET":  "article.reed",
			"POST": authorization.Public,
		}},
	}
	known := []authorization.Action{"article.read"}

	violations := validateRoutePermissionGroup("/api", files, known)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, `unknown permission "article.reed"`) {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateGroupNoStrictModeWithoutKnownList(t *testing.T) {
	files := []routeFile{
		{file: "index.go", methods: []string{"GET"}, hasSymbol: true, permissions: map[string]string{"GET": "anything.goes"}},
	}

	violations := validateRoutePermissionGroup("/api", files, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation without a known permission list, got %v", violations)
	}
}

func TestValidateGroupCollectsAllViolations(t *testing.T) {
	files := []routeFile{
		{file: "a.go", methods: []string{"GET", "POST"}, hasSymbol: true, permissions: map[string]string{
			"GET": "article.reed", // unknown
			"PUT": "article.read", // orphan entry
		}},
	}
	known := []authorization.Action{"article.read"}

	violations := validateRoutePermissionGroup("/api", files, known)

	// POST has no entry, GET has an unknown value, PUT is orphan.
	if len(violations) != 3 {
		t.Fatalf("expected 3 violations, got %d: %v", len(violations), violations)
	}
}

func TestPermissionReportFormat(t *testing.T) {
	violations := []routeViolation{
		{File: "b.go", Method: "GET", Problem: "no permission declared for this exported method"},
		{File: "a.go", Method: "Permissions", Problem: "missing required symbol: var Permissions map[string]string"},
	}

	report := permissionReport(violations)

	lines := strings.Split(report, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected a header and one line per violation, got %q", report)
	}
	// Lines are sorted for a deterministic report despite concurrent loading.
	if lines[1] != "a.go: Permissions: missing required symbol: var Permissions map[string]string" {
		t.Errorf("unexpected first line: %q", lines[1])
	}
	if lines[2] != "b.go: GET: no permission declared for this exported method" {
		t.Errorf("unexpected second line: %q", lines[2])
	}
}
