package ginmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
)

// countingProvider counts the resolutions the store is asked for, around any
// provider. It is concurrency-safe: a handler may authorize from several
// goroutines.
type countingProvider struct {
	inner authorization.GrantProvider

	mu    sync.Mutex
	calls int
}

func (p *countingProvider) Resolve(ctx context.Context, subject authorization.Subject, scope authorization.Scope) (authorization.Grants, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	return p.inner.Resolve(ctx, subject, scope)
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls
}

func (p *countingProvider) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = 0
}

// newCountedEngine returns an engine over a counting provider, plus the store
// so a test can revoke at runtime.
func newCountedEngine(t *testing.T) (*authorization.Engine, *memory.Provider, *countingProvider) {
	t.Helper()

	store := memory.NewProvider()
	ctx := context.Background()
	if err := store.CreateRole(ctx, "editor"); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantPermissions(ctx, "editor", "article.read"); err != nil {
		t.Fatal(err)
	}
	if err := store.BindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatal(err)
	}

	counter := &countingProvider{inner: store}

	return authorization.NewEngine(authorization.WithProvider(counter)), store, counter
}

// serveWithHandler runs one request through the RBAC middleware and a handler
// re-checking the same permission, as Context.Authorize does after loading
// the resource.
func serveWithHandler(engine *authorization.Engine, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/articles", MiddlewareFor(engine, "article.read"), handler)

	request := httptest.NewRequest(http.MethodGet, "/articles", nil)
	ctx := authorization.WithSubject(request.Context(), authorization.Subject{ID: "subject-1"})
	ctx = authorization.WithScope(ctx, "tenant-1")
	request = request.WithContext(ctx)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

// TestMiddlewareCacheSharedWithHandler covers AUTHZ-601 across the HTTP
// boundary: the cache created by the middleware must be visible from the
// handler, which only works because the middleware rewrites c.Request.
func TestMiddlewareCacheSharedWithHandler(t *testing.T) {
	engine, _, counter := newCountedEngine(t)

	recorder := serveWithHandler(engine, func(c *gin.Context) {
		ctx := c.Request.Context()
		subject, _ := authorization.SubjectFromContext(ctx)
		for range 3 {
			decision := engine.Decide(ctx, authorization.Request{
				Subject: subject,
				Action:  "article.read",
				Scope:   authorization.ScopeFromContext(ctx),
			})
			if !decision.Allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})

				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	// Two resolutions for the whole request: tenant-1 and ScopeGlobal, each
	// asked once, for the middleware check plus three handler decisions.
	if got := counter.count(); got != 2 {
		t.Errorf("provider calls = %d, want 2 for the whole request", got)
	}
}

// TestGrantCacheDoesNotSurviveTheRequest covers AUTHZ-602 over HTTP: a
// revocation between two requests is visible on the second one.
func TestGrantCacheDoesNotSurviveTheRequest(t *testing.T) {
	engine, store, counter := newCountedEngine(t)

	handler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": "ok"}) }

	if recorder := serveWithHandler(engine, handler); recorder.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", recorder.Code)
	}

	if err := store.UnbindRoles(context.Background(), "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatal(err)
	}
	counter.reset()

	recorder := serveWithHandler(engine, handler)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("second request status = %d, want 403: the revocation must be visible right away", recorder.Code)
	}
	if got := counter.count(); got == 0 {
		t.Error("the second request must resolve the grants again, not reuse the first request's cache")
	}
	assertGenericBody(t, recorder)
}

// TestHandlerGoroutinesShareTheCache covers the concurrency of the
// per-request cache through the HTTP path: a handler fanning out into
// goroutines authorizes from all of them. Run under -race.
func TestHandlerGoroutinesShareTheCache(t *testing.T) {
	engine, _, counter := newCountedEngine(t)

	recorder := serveWithHandler(engine, func(c *gin.Context) {
		ctx := c.Request.Context()
		subject, _ := authorization.SubjectFromContext(ctx)

		var (
			wg      sync.WaitGroup
			denials = make([]bool, 32)
		)
		for i := range denials {
			wg.Add(1)
			go func() {
				defer wg.Done()

				denials[i] = !engine.Decide(ctx, authorization.Request{
					Subject: subject,
					Action:  "article.read",
					Scope:   authorization.ScopeFromContext(ctx),
				}).Allowed
			}()
		}
		wg.Wait()

		for _, denied := range denials {
			if denied {
				c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})

				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	// Concurrent misses may resolve the same key twice; the point is that the
	// cache stays coherent and the request does not pay one resolution per
	// goroutine.
	if got := counter.count(); got > 8 {
		t.Errorf("provider calls = %d, want a memoized handful for 33 checks", got)
	}
}
