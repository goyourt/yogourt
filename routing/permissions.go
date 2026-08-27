package routing

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/core"
)

// derivedVerbs maps an HTTP method to the verb part of a permission derived by
// convention. A method absent from this table has no convention: the route
// must declare its permission explicitly.
var derivedVerbs = map[string]string{
	http.MethodGet:    "read",
	http.MethodPost:   "create",
	http.MethodPut:    "update",
	http.MethodPatch:  "update",
	http.MethodDelete: "delete",
}

// routeViolation is one route permission declaration problem found while
// validating a route at startup (D2).
type routeViolation struct {
	File    string
	Method  string
	Problem string
}

func (v routeViolation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.File, v.Method, v.Problem)
}

// routeFile is one loaded plugin file contributing handlers to a route. The
// URL being derived from the folder, several files may serve the same route.
type routeFile struct {
	file        string
	methods     []string
	permissions map[string]string
	hasSymbol   bool
}

// methodPermission is the effective permission of one route method: the value
// declared in the Permissions map when the method is overridden, the one
// derived from the folder and the HTTP method otherwise.
type methodPermission struct {
	permission string
	derived    bool
}

// origin labels the permission source in the boot log of the authorization
// surface.
func (p methodPermission) origin() string {
	if p.derived {
		return "derived"
	}

	return "declared"
}

// derivePermission returns the permission derived by convention from the Gin
// route path and the HTTP method, as "<resource>.<verb>": the resource is the
// last static segment of the path below prefix (lowercased), the verb comes
// from derivedVerbs. ok is false when no convention applies — the root route,
// the prefix itself, has no resource segment, and an HTTP method outside
// derivedVerbs has no verb — in which case the route must declare the
// permission explicitly.
//
// prefix is the configured HTTP prefix ("/api" by default): the segments it
// holds name a mount point, not a resource, so they never derive a permission.
func derivePermission(prefix, routePath, method string) (permission string, ok bool) {
	verb, known := derivedVerbs[strings.ToUpper(method)]
	if !known {
		return "", false
	}

	resource := ""
	trimmed := strings.TrimPrefix(routePath, prefix)
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "" {
			continue
		}
		// Gin parameters (":id") and catch-alls ("*path") carry no resource
		// name: only static segments can name one.
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			continue
		}
		resource = segment
	}
	if resource == "" {
		return "", false
	}

	return strings.ToLower(resource) + "." + verb, true
}

// validateRoutePermissionGroup resolves the effective permission of every
// exported method of one route — the folder of a route may spread its handlers
// over several files — and reports the declaration problems found.
//
// Permissions are derived by convention (see derivePermission); the optional
// Permissions map of the folder is a partial override, so a method absent from
// it simply keeps its derived permission. Remaining violations: the map
// declared by several files of the same folder, an entry naming no exported
// handler (typo), an empty permission, a method no convention can cover, and —
// in strict mode, when known is non-empty — any unknown permission, be it
// overridden or derived (authorization.Public is always exempt).
//
// The returned map holds the effective permission of each valid method, so the
// loader builds the handler chains and the boot synchronization without
// recomputing anything.
func validateRoutePermissionGroup(prefix, route string, files []routeFile, known []authorization.Action) (map[string]methodPermission, []routeViolation) {
	sortedFiles := append([]routeFile(nil), files...)
	sort.Slice(sortedFiles, func(i, j int) bool { return sortedFiles[i].file < sortedFiles[j].file })

	// Union of the exported methods of the folder, with the first exporting
	// file kept for reporting.
	methodFiles := make(map[string]string)
	var declaring []routeFile
	for _, f := range sortedFiles {
		for _, method := range f.methods {
			if _, seen := methodFiles[method]; !seen {
				methodFiles[method] = f.file
			}
		}
		if f.hasSymbol {
			declaring = append(declaring, f)
		}
	}

	if len(methodFiles) == 0 && len(declaring) == 0 {
		// A folder without any exported handler registers no route and
		// declares nothing: utility files stay valid.
		return nil, nil
	}
	if len(declaring) > 1 {
		names := make([]string, len(declaring))
		for i, f := range declaring {
			names[i] = f.file
		}

		return nil, []routeViolation{{
			File:    route,
			Method:  "Permissions",
			Problem: fmt.Sprintf("declared in %d files (%s): route permissions must be declared in a single file", len(declaring), strings.Join(names, ", ")),
		}}
	}

	// At most one file overrides the convention for this route; without any
	// declaration every method is derived.
	var declaration routeFile
	if len(declaring) == 1 {
		declaration = declaring[0]
	}

	knownSet := make(map[string]bool, len(known))
	for _, permission := range known {
		knownSet[string(permission)] = true
	}

	var violations []routeViolation
	effective := make(map[string]methodPermission, len(methodFiles))

	methods := make([]string, 0, len(methodFiles))
	for method := range methodFiles {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	for _, method := range methods {
		permission, overridden := declaration.permissions[method]
		file := declaration.file
		if !overridden {
			file = methodFiles[method]

			derived, ok := derivePermission(prefix, route, method)
			if !ok {
				violations = append(violations, routeViolation{
					File:    file,
					Method:  method,
					Problem: fmt.Sprintf("no permission can be derived for %s %s: declare it in var Permissions map[string]string", method, route),
				})

				continue
			}
			permission = derived
		}
		if permission == "" {
			// An empty permission is invalid unconditionally: it could never
			// be granted and Context.Authorize would answer 500 on every
			// request. Catch it at boot even without a strict list (D2).
			violations = append(violations, routeViolation{
				File:    file,
				Method:  method,
				Problem: "empty permission declared",
			})

			continue
		}
		if len(known) > 0 && permission != authorization.Public && !knownSet[permission] {
			problem := fmt.Sprintf("unknown permission %q", permission)
			if !overridden {
				problem += " derived by convention: declare a known permission for this method"
			}
			violations = append(violations, routeViolation{
				File:    file,
				Method:  method,
				Problem: problem,
			})

			continue
		}

		effective[method] = methodPermission{permission: permission, derived: !overridden}
	}

	entries := make([]string, 0, len(declaration.permissions))
	for entry := range declaration.permissions {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	for _, entry := range entries {
		if _, exported := methodFiles[entry]; !exported {
			violations = append(violations, routeViolation{
				File:    declaration.file,
				Method:  entry,
				Problem: "permission declared but no exported handler with this name",
			})
		}
	}

	return effective, violations
}

// permissionReport formats all collected violations as a single exhaustive
// report, one "file: method: problem" line per violation (D2).
func permissionReport(violations []routeViolation) string {
	lines := make([]string, len(violations))
	for i, violation := range violations {
		lines[i] = violation.String()
	}
	sort.Strings(lines)

	return fmt.Sprintf("route permission validation failed (%d violation(s)):\n%s", len(lines), strings.Join(lines, "\n"))
}

// loadRoutePermissions extracts the optional Permissions symbol from a route
// plugin. Any symbol lookup failure means the symbol is absent (AUTHZ-401);
// a symbol present with an unexpected type is reported through err.
func loadRoutePermissions(soPath string) (permissions map[string]string, hasSymbol bool, err error) {
	loaded, err := compiler.LoadSymbol[map[string]string](soPath, "Permissions")
	if err != nil {
		if errors.Is(err, compiler.ErrSymbolNotFound) {
			return nil, false, nil
		}

		return nil, true, err
	}

	return *loaded, true, nil
}

// routePermissionMiddleware records the permission declared for the route
// method, making it available to Context.Authorize.
func routePermissionMiddleware(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		core.SetRoutePermission(c, permission)
	}
}

func methodsOf(routes map[string]gin.HandlerFunc) []string {
	methods := make([]string, 0, len(routes))
	for method := range routes {
		methods = append(methods, method)
	}

	return methods
}
