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
// validating a route file at startup (D2).
type routeViolation struct {
	File    string
	Method  string
	Problem string
}

func (v routeViolation) String() string {
	return fmt.Sprintf("%s: %s: %s", v.File, v.Method, v.Problem)
}

// validateRoutePermissions checks the Permissions declaration of one route
// file against its exported handler methods. hasSymbol reports whether the
// file exports a Permissions symbol at all. known is the optional list of
// permissions the engine knows about; when non-empty, any declared value
// other than authorization.Public must belong to it.
func validateRoutePermissions(file string, permissions map[string]string, hasSymbol bool, methods []string, known []authorization.Action) []routeViolation {
	if !hasSymbol {
		// A file without any exported handler registers no route: it needs no
		// Permissions symbol (utility files under the API folder stay valid).
		if len(methods) == 0 {
			return nil
		}

		return []routeViolation{{
			File:    file,
			Method:  "Permissions",
			Problem: "missing required symbol: var Permissions map[string]string",
		}}
	}

	knownSet := make(map[string]bool, len(known))
	for _, permission := range known {
		knownSet[string(permission)] = true
	}
	methodSet := make(map[string]bool, len(methods))
	for _, method := range methods {
		methodSet[method] = true
	}

	var violations []routeViolation

	sortedMethods := append([]string(nil), methods...)
	sort.Strings(sortedMethods)
	for _, method := range sortedMethods {
		permission, declared := permissions[method]
		if !declared {
			violations = append(violations, routeViolation{
				File:    file,
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
				File:    file,
				Method:  method,
				Problem: "empty permission declared",
			})

			continue
		}
		if len(known) > 0 && permission != authorization.Public && !knownSet[permission] {
			violations = append(violations, routeViolation{
				File:    file,
				Method:  method,
				Problem: fmt.Sprintf("unknown permission %q", permission),
			})
		}
	}

	entries := make([]string, 0, len(permissions))
	for entry := range permissions {
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	for _, entry := range entries {
		if !methodSet[entry] {
			violations = append(violations, routeViolation{
				File:    file,
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
