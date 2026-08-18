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

// loadAPIHandlers loads every route plugin under basePath and registers its
// handlers. engine is nil when no authorizer is configured (D1); otherwise
// every route file must declare its permissions and the RBAC middleware is
// inserted in front of each non-public handler. All loading errors and all
// permission violations are collected before failing, so a boot failure
// reports every problem at once (D2).
func loadAPIHandlers(r *gin.Engine, basePath string, engine *authorization.Engine) error {
	files, err := walkGoFiles(basePath)
	if err != nil {
		return err
	}

	type routeTask struct {
		protocol  string
		routePath string
		handlers  []gin.HandlerFunc
	}

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		loadErrs   []error
		violations []routeViolation
		tasks      []routeTask
	)

	var known []authorization.Action
	if engine != nil {
		known = engine.KnownPermissions()
	}

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

			rp := routePathFor(basePath, f)

			routes, lerr := compiler.LoadRoutes(so)
			if lerr != nil {
				mu.Lock()
				loadErrs = append(loadErrs, fmt.Errorf("load error %s: %w", f, lerr))
				mu.Unlock()
				return
			}

			reportPath := reportFilePath(basePath, f)
			permissions, hasSymbol, symErr := loadRoutePermissions(so)

			if engine == nil {
				// D1: without an authorizer a Permissions symbol is ignored.
				if hasSymbol {
					log.Printf("warning: %s exports a Permissions symbol but no authorizer is configured; the symbol is ignored", reportPath)
				}
			} else {
				fileViolations := []routeViolation{}
				if symErr != nil {
					fileViolations = append(fileViolations, routeViolation{
						File:    reportPath,
						Method:  "Permissions",
						Problem: symErr.Error(),
					})
				} else {
					fileViolations = validateRoutePermissions(reportPath, permissions, hasSymbol, methodsOf(routes), known)
				}
				if len(fileViolations) > 0 {
					mu.Lock()
					violations = append(violations, fileViolations...)
					mu.Unlock()
					return
				}
			}

			baseMw := middleware.GetMiddleware(rp)
			for m, h := range routes {
				mu.Lock()
				tasks = append(tasks, routeTask{
					protocol:  m,
					routePath: rp,
					handlers:  routeHandlerChain(engine, permissions[m], baseMw, h),
				})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// One single exhaustive boot report (D2): plugin loading errors and
	// permission violations are joined so no category masks the other.
	if len(violations) > 0 {
		loadErrs = append(loadErrs, errors.New(permissionReport(violations)))
	}
	if len(loadErrs) > 0 {
		return errors.Join(loadErrs...)
	}

	for _, t := range tasks {
		r.Handle(t.protocol, t.routePath, t.handlers...)
	}

	return nil
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
