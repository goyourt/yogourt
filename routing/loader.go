package routing

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/ginmw"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/middleware"
)

// loadedRouteFile is the result of loading one route plugin: its handlers
// and its optional Permissions declaration.
type loadedRouteFile struct {
	file        string
	routePath   string
	routes      map[string]gin.HandlerFunc
	permissions map[string]string
	hasSymbol   bool
	symErr      error
}

// loadAPIHandlers loads every route plugin under basePath and registers its
// handlers under prefix — the configured HTTP prefix, "/api" unless the
// application changed it. engine is nil when no authorizer is configured (D1); otherwise
// every method of every route gets an effective permission — derived from the
// folder and the HTTP method by convention, or overridden by the Permissions
// map of the folder — and the RBAC middleware is inserted in front of each
// non-public handler. All loading errors and all permission violations are
// collected before failing, so a boot failure reports every problem at once
// (D2). It returns the deduplicated set of effective permissions of the
// routes, for the boot synchronization with the grant provider.
func loadAPIHandlers(r *gin.Engine, prefix, basePath string, engine *authorization.Engine) ([]authorization.Action, error) {
	files, err := walkGoFiles(basePath)
	if err != nil {
		return nil, err
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		loadErrs []error
		loaded   []loadedRouteFile
	)

	// semaphore to limit concurrent plugin loading
	sem := make(chan struct{}, runtime.NumCPU())

	for _, f := range files {
		wg.Add(1)

		// acquire semaphore before starting the plugin loading goroutine
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			so, cerr := compiler.ResolvePlugin(f)
			if cerr != nil {
				mu.Lock()
				loadErrs = append(loadErrs, fmt.Errorf("resolve plugin error %s: %w", f, cerr))
				mu.Unlock()
				return
			}

			routes, lerr := compiler.LoadRoutes(so)
			if lerr != nil {
				mu.Lock()
				loadErrs = append(loadErrs, fmt.Errorf("load error %s: %w", f, lerr))
				mu.Unlock()
				return
			}

			permissions, hasSymbol, symErr := loadRoutePermissions(so)

			mu.Lock()
			loaded = append(loaded, loadedRouteFile{
				file:        reportFilePath(basePath, f),
				routePath:   routePathFor(prefix, basePath, f),
				routes:      routes,
				permissions: permissions,
				hasSymbol:   hasSymbol,
				symErr:      symErr,
			})
			mu.Unlock()
		}()
	}

	wg.Wait()

	// The URL is derived from the folder: several files may serve the same
	// route, so declarations are validated per route, not per file.
	groups := make(map[string][]loadedRouteFile)
	for _, lf := range loaded {
		groups[lf.routePath] = append(groups[lf.routePath], lf)
	}

	var violations []routeViolation
	// Effective permission of every method of every route, empty when no
	// authorizer is configured (the handler chain then ignores permissions).
	effective := make(map[string]map[string]methodPermission, len(groups))
	if engine == nil {
		// D1: without an authorizer a Permissions symbol is ignored.
		for _, lf := range loaded {
			if lf.hasSymbol {
				log.Printf("warning: %s exports a Permissions symbol but no authorizer is configured; the symbol is ignored", lf.file)
			}
		}
	} else {
		known := engine.KnownPermissions()
		for routePath, groupFiles := range groups {
			groupRouteFiles := make([]routeFile, 0, len(groupFiles))
			for _, lf := range groupFiles {
				if lf.symErr != nil {
					violations = append(violations, routeViolation{
						File:    lf.file,
						Method:  "Permissions",
						Problem: lf.symErr.Error(),
					})
				}
				groupRouteFiles = append(groupRouteFiles, routeFile{
					file:        lf.file,
					methods:     methodsOf(lf.routes),
					permissions: lf.permissions,
					hasSymbol:   lf.hasSymbol && lf.symErr == nil,
				})
			}
			routePermissions, groupViolations := validateRoutePermissionGroup(prefix, routePath, groupRouteFiles, known)
			violations = append(violations, groupViolations...)
			effective[routePath] = routePermissions
		}
	}

	// One single exhaustive boot report (D2): plugin loading errors and
	// permission violations are joined so no category masks the other.
	if len(violations) > 0 {
		loadErrs = append(loadErrs, errors.New(permissionReport(violations)))
	}
	if len(loadErrs) > 0 {
		return nil, errors.Join(loadErrs...)
	}

	declared := make(map[authorization.Action]bool)
	for routePath, groupFiles := range groups {
		// Validation resolved the effective permission of every method of the
		// route: overridden by the Permissions map, or derived by convention.
		routePermissions := effective[routePath]
		for _, permission := range routePermissions {
			if permission.permission != authorization.Public {
				declared[authorization.Action(permission.permission)] = true
			}
		}

		baseMw := middleware.GetMiddleware(routePath)
		for _, lf := range groupFiles {
			for m, h := range lf.routes {
				r.Handle(m, routePath, routeHandlerChain(engine, routePermissions[m].permission, baseMw, h)...)
			}
		}
	}

	if engine != nil {
		// The convention removed the fail-fast on a missing declaration: the
		// operator must still be able to read the whole authorization surface
		// at every boot.
		for _, line := range authorizationSurfaceLines(effective) {
			log.Print(line)
		}
	}

	declaredList := make([]authorization.Action, 0, len(declared))
	for permission := range declared {
		declaredList = append(declaredList, permission)
	}

	return declaredList, nil
}

// authorizationSurfaceLines formats the authorization surface as one compact
// line per route method, sorted by route then method, each naming the
// effective permission and where it comes from.
func authorizationSurfaceLines(effective map[string]map[string]methodPermission) []string {
	type entry struct {
		route  string
		method string
		line   string
	}

	entries := make([]entry, 0, len(effective))
	for route, permissions := range effective {
		for method, permission := range permissions {
			entries = append(entries, entry{
				route:  route,
				method: method,
				line:   fmt.Sprintf("authorization: %s %s -> %s (%s)", method, route, permission.permission, permission.origin()),
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].route != entries[j].route {
			return entries[i].route < entries[j].route
		}

		return entries[i].method < entries[j].method
	})

	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.line
	}

	return lines
}

// routeHandlerChain assembles the final handler chain for one route method:
// inherited callbacks first, then — only when an authorizer is configured —
// the middleware recording the effective permission, then the RBAC middleware
// (last, right before the handler) unless the method is authorization.Public.
// Without an authorizer the chain is strictly identical to pre-v2 (D1).
func routeHandlerChain(engine *authorization.Engine, permission string, baseMw []gin.HandlerFunc, handler gin.HandlerFunc) []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, len(baseMw)+3)
	handlers = append(handlers, baseMw...)
	if engine != nil {
		handlers = append(handlers, routePermissionMiddleware(permission))
		if permission != authorization.Public {
			handlers = append(handlers, ginmw.MiddlewareFor(engine, permission))
		}
	}

	return append(handlers, handler)
}

// reportFilePath returns the path used to identify a route file in warnings
// and violation reports, relative to the API folder when possible.
func reportFilePath(basePath, fullPath string) string {
	rel, err := filepath.Rel(basePath, fullPath)
	if err != nil {
		return fullPath
	}

	return filepath.ToSlash(rel)
}
