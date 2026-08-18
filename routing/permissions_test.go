package routing

import (
	"strings"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

func TestValidateRoutePermissionsMissingSymbol(t *testing.T) {
	violations := validateRoutePermissions("users/index.go", nil, false, []string{"GET", "POST"}, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation for a missing symbol, got %d: %v", len(violations), violations)
	}
	if violations[0].File != "users/index.go" || violations[0].Method != "Permissions" {
		t.Errorf("unexpected violation location: %+v", violations[0])
	}
	if !strings.Contains(violations[0].Problem, "missing required symbol") {
		t.Errorf("unexpected problem: %q", violations[0].Problem)
	}
}

func TestValidateRoutePermissionsUtilityFileWithoutHandlers(t *testing.T) {
	// A file exporting no HTTP handler registers no route: it must not be
	// forced to declare a Permissions symbol.
	violations := validateRoutePermissions("shared/helpers.go", nil, false, nil, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation for a handler-less file, got %v", violations)
	}
}

func TestValidateRoutePermissionsEmptyPermission(t *testing.T) {
	// An empty permission can never be granted and would turn every request
	// into a 500: it must be a boot violation even without a strict list.
	permissions := map[string]string{"GET": ""}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET"}, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, "empty permission") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateRoutePermissionsUndeclaredMethod(t *testing.T) {
	permissions := map[string]string{"GET": "article.read"}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET", "POST"}, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "POST" || !strings.Contains(violations[0].Problem, "no permission declared") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateRoutePermissionsOrphanEntry(t *testing.T) {
	permissions := map[string]string{
		"GET":    "article.read",
		"DELETE": "article.delete",
	}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET"}, nil)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "DELETE" || !strings.Contains(violations[0].Problem, "no exported handler") {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateRoutePermissionsUnknownPermission(t *testing.T) {
	permissions := map[string]string{
		"GET":  "article.reed",
		"POST": authorization.Public,
	}
	known := []authorization.Action{"article.read"}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET", "POST"}, known)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Method != "GET" || !strings.Contains(violations[0].Problem, `unknown permission "article.reed"`) {
		t.Errorf("unexpected violation: %+v", violations[0])
	}
}

func TestValidateRoutePermissionsNoStrictModeWithoutKnownList(t *testing.T) {
	permissions := map[string]string{"GET": "anything.goes"}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET"}, nil)

	if len(violations) != 0 {
		t.Fatalf("expected no violation without a known permission list, got %v", violations)
	}
}

func TestValidateRoutePermissionsValidDeclaration(t *testing.T) {
	permissions := map[string]string{
		"GET":    "article.read",
		"POST":   authorization.Public,
		"DELETE": "article.delete",
	}
	known := []authorization.Action{"article.read", "article.delete"}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET", "POST", "DELETE"}, known)

	if len(violations) != 0 {
		t.Fatalf("expected no violation, got %v", violations)
	}
}

func TestValidateRoutePermissionsCollectsAllViolations(t *testing.T) {
	permissions := map[string]string{
		"GET": "article.reed", // unknown (d)
		"PUT": "article.read", // orphan entry (c)
	}
	known := []authorization.Action{"article.read"}

	violations := validateRoutePermissions("index.go", permissions, true, []string{"GET", "POST"}, known)

	// POST has no entry (b), GET has an unknown value (d), PUT is orphan (c).
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
