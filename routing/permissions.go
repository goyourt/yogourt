package routing

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/core"
)

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

// validateRoutePermissionGroup checks the Permissions declaration of one
// route against the handlers exported by ALL the files of its folder. The
// route permissions are declared in exactly one of those files, covering
// every exported method of the folder; several declarations for the same
// route refuse the boot. known is the optional list of permissions the
// engine knows about; when non-empty, any declared value other than
// authorization.Public must belong to it.
func validateRoutePermissionGroup(route string, files []routeFile, known []authorization.Action) []routeViolation {
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

	if len(declaring) == 0 {
		// A folder without any exported handler registers no route: it needs
		// no Permissions symbol (utility files stay valid).
		if len(methodFiles) == 0 {
			return nil
		}

		return []routeViolation{{
			File:    route,
			Method:  "Permissions",
			Problem: "missing required symbol: var Permissions map[string]string (declare the route permissions in one file of this folder)",
		}}
	}
	if len(declaring) > 1 {
		names := make([]string, len(declaring))
		for i, f := range declaring {
			names[i] = f.file
		}

		return []routeViolation{{
			File:    route,
			Method:  "Permissions",
			Problem: fmt.Sprintf("declared in %d files (%s): route permissions must be declared in a single file", len(declaring), strings.Join(names, ", ")),
		}}
	}

	declaration := declaring[0]
	knownSet := make(map[string]bool, len(known))
	for _, permission := range known {
		knownSet[string(permission)] = true
	}

	var violations []routeViolation

	methods := make([]string, 0, len(methodFiles))
	for method := range methodFiles {
		methods = append(methods, method)
	}
	sort.Strings(methods)
	for _, method := range methods {
		permission, declared := declaration.permissions[method]
		if !declared {
			violations = append(violations, routeViolation{
				File:    methodFiles[method],
				Method:  method,
				Problem: "no permission declared for this exported method",
			})

			continue
		}
		if permission == "" {
			// An empty permission is invalid unconditionally: it could never
			// be granted and Context.Authorize would answer 500 on every
			// request. Catch it at boot even without a strict list (D2).
			violations = append(violations, routeViolation{
				File:    declaration.file,
				Method:  method,
				Problem: "empty permission declared",
			})

			continue
		}
		if len(known) > 0 && permission != authorization.Public && !knownSet[permission] {
			violations = append(violations, routeViolation{
				File:    declaration.file,
				Method:  method,
				Problem: fmt.Sprintf("unknown permission %q", permission),
			})
		}
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

	return violations
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
