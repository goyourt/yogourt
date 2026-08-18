package routing

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
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
// handlers. engine is nil when no authorizer is configured (D1); otherwise
// each route folder must declare its permissions in exactly one of its files
// and the RBAC middleware is inserted in front of each non-public handler.
// All loading errors and all permission violations are collected before
// failing, so a boot failure reports every problem at once (D2). It returns
// the deduplicated set of permissions declared by the routes, for the boot
// synchronization with the grant provider.
func loadAPIHandlers(r *gin.Engine, basePath string, engine *authorization.Engine) ([]authorization.Action, error) {
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
				routePath:   routePathFor(basePath, f),
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
			violations = append(violations, validateRoutePermissionGroup(routePath, groupRouteFiles, known)...)
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
		// After validation, at most one file of the group declares the route
		// permissions; its map covers every method of the folder.
		var permissions map[string]string
		if engine != nil {
			for _, lf := range groupFiles {
				if lf.hasSymbol && lf.symErr == nil {
					permissions = lf.permissions
					break
				}
			}
			for _, permission := range permissions {
				if permission != authorization.Public && permission != "" {
					declared[authorization.Action(permission)] = true
				}
			}
		}

		baseMw := middleware.GetMiddleware(routePath)
		for _, lf := range groupFiles {
			for m, h := range lf.routes {
				r.Handle(m, routePath, routeHandlerChain(engine, permissions[m], baseMw, h)...)
			}
		}
	}

	declaredList := make([]authorization.Action, 0, len(declared))
	for permission := range declared {
		declaredList = append(declaredList, permission)
	}

	return declaredList, nil
}

// routeHandlerChain assembles the final handler chain for one route method:
// inherited callbacks first, then — only when an authorizer is configured —
// the middleware recording the declared permission, then the RBAC middleware
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
