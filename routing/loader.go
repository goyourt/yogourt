package routing

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/goyourt/yogourt/compiler"
	"github.com/goyourt/yogourt/middleware"
)

func loadAPIHandlers(r *gin.Engine, basePath string) error {
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
		wg       sync.WaitGroup
		mu       sync.Mutex
		errFirst error
		tasks    []routeTask
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
				if errFirst == nil {
					errFirst = fmt.Errorf("resolve plugin error %s: %w", f, cerr)
				}
				mu.Unlock()
				return
			}

			rp := routePathFor(basePath, f)

			routes, lerr := compiler.LoadRoutes(so)
			if lerr != nil {
				mu.Lock()
				if errFirst == nil {
					errFirst = fmt.Errorf("load error %s: %w", f, lerr)
				}
				mu.Unlock()
				return
			}

			baseMw := middleware.GetMiddleware(rp)
			for m, h := range routes {
				mws := make([]gin.HandlerFunc, len(baseMw), len(baseMw)+1)
				copy(mws, baseMw)
				mws = append(mws, h)
				mu.Lock()
				tasks = append(tasks, routeTask{
					protocol:  m,
					routePath: rp,
					handlers:  mws,
				})
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if errFirst != nil {
		return errFirst
	}

	for _, t := range tasks {
		r.Handle(t.protocol, t.routePath, t.handlers...)
	}

	return errFirst
}
