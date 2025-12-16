package routing

import (
	"fmt"
	"path/filepath"
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

	var (
		compileWg sync.WaitGroup
		loadWg    sync.WaitGroup
		mu        sync.Mutex
		errFirst  error
	)

	// ensure at least 1 worker
	maxWorkers := runtime.NumCPU() / 2
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	// semaphore to limit concurrent compilations
	sem := make(chan struct{}, maxWorkers)

	// buffer tasks to avoid backpressure during compilation
	tasks := make(chan struct {
		protocol  string
		routePath string
		handlers  []gin.HandlerFunc
	}, len(files))

	regDone := make(chan struct{})
	go func() {
		for t := range tasks {
			r.Handle(t.protocol, t.routePath, t.handlers...)
		}
		close(regDone)
	}()

	for _, f := range files {
		compileWg.Add(1)
		f := f

		// acquire semaphore before starting the compile goroutine
		sem <- struct{}{}

		go func() {
			defer compileWg.Done()
			defer func() { <-sem }()

			so, cerr := compiler.CompileCached(f)
			if cerr != nil {
				mu.Lock()
				if errFirst == nil {
					errFirst = fmt.Errorf("compile error %s: %w", f, cerr)
				}
				mu.Unlock()
				return
			}

			rp := routePathFor(basePath, f, filepath.Base(f))

			routesCh := make(chan map[string]gin.HandlerFunc, 1)
			mwCh := make(chan []gin.HandlerFunc, 1)

			go func(src, soPath string) {
				routes, lerr := compiler.LoadRoutes(soPath)
				if lerr != nil {
					mu.Lock()
					if errFirst == nil {
						errFirst = fmt.Errorf("load error %s: %w", src, lerr)
					}
					mu.Unlock()
					routesCh <- nil
					return
				}
				routesCh <- routes
			}(f, so)

			go func(rpath string) {
				baseMw := middleware.GetMiddleware(rpath)
				mwCh <- baseMw
			}(rp)

			loadWg.Add(1)
			go func(src string) {
				defer loadWg.Done()
				routes := <-routesCh
				baseMw := <-mwCh
				if routes == nil {
					return
				}
				for m, h := range routes {
					mws := make([]gin.HandlerFunc, len(baseMw), len(baseMw)+1)
					copy(mws, baseMw)
					mws = append(mws, h)
					tasks <- struct {
						protocol  string
						routePath string
						handlers  []gin.HandlerFunc
					}{protocol: m, routePath: rp, handlers: mws}
				}
			}(f)
		}()
	}

	compileWg.Wait()
	loadWg.Wait()
	close(tasks)
	<-regDone

	return errFirst
}
