package core_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
	"github.com/goyourt/yogourt/core"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestContext builds a Yogourt context around a recorded request. subject
// is attached to the request context when non-nil.
func newTestContext(t *testing.T, subject *authorization.Subject) (*core.Context, *httptest.ResponseRecorder) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/articles", nil)
	if subject != nil {
		request = request.WithContext(authorization.WithSubject(request.Context(), *subject))
	}
	ginCtx.Request = request

	return core.NewContext(ginCtx), recorder
}

// lockableArticle is a resource a request can lock between two checks.
type lockableArticle struct {
	locked bool
}

// countingProvider counts the resolutions the store is asked for, so a test
// can prove the per-request memoization actually spares provider calls
// (AUTHZ-601). It is concurrency-safe: a handler may authorize from several
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

func (p *countingProvider) countAndReset() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	calls := p.calls
	p.calls = 0

	return calls
}

// newScopedContext builds a context for a request made within a scope, so
// that a resolution costs the two provider calls of the union {scope,
// ScopeGlobal} and the memoization is measurable.
func newScopedContext(t *testing.T, subject *authorization.Subject, scope authorization.Scope) *core.Context {
	t.Helper()

	c, _ := newTestContext(t, subject)
	c.Request = c.Request.WithContext(authorization.WithScope(c.Request.Context(), scope))

	return c
}

func assertAborted(t *testing.T, c *core.Context, recorder *httptest.ResponseRecorder, status int) {
	t.Helper()

	if !c.IsAborted() {
		t.Error("expected the request to be aborted")
	}
	if recorder.Code != status {
		t.Errorf("status = %d, want %d", recorder.Code, status)
	}
	// Internal reasons must never leak into response bodies (D7).
	body := recorder.Body.String()
	for _, reason := range []string{"missing_permission", "policy_denied", "policy_error", "provider_error", "misconfigured", "unauthenticated"} {
		if strings.Contains(body, reason) {
			t.Errorf("response body leaks internal reason %q: %s", reason, body)
		}
	}
}

// TestContextAuthorization drives every Context helper around the process-wide
// published engine. The storage is write-once, so a single ordered test
// exercises the engine-less paths first, then publishes an engine for the
// remaining cases.
func TestContextAuthorization(t *testing.T) {
	subject := &authorization.Subject{ID: "subject-1"}

	t.Run("without engine", func(t *testing.T) {
		c, _ := newTestContext(t, subject)
		if c.HasPermission("article.read") {
			t.Error("HasPermission must be false without a published engine")
		}
		if c.Can("article.read", nil) {
			t.Error("Can must be false without a published engine")
		}

		c, recorder := newTestContext(t, subject)
		if c.AuthorizeAction("article.read", nil) {
			t.Error("AuthorizeAction must deny without a published engine")
		}
		assertAborted(t, c, recorder, http.StatusInternalServerError)

		c, recorder = newTestContext(t, subject)
		core.SetRoutePermission(c.Gin(), "article.read")
		if c.Authorize(nil) {
			t.Error("Authorize must deny without a published engine")
		}
		assertAborted(t, c, recorder, http.StatusInternalServerError)
	})

	provider := memory.NewProvider()
	if err := provider.CreateRole(context.Background(), "editor"); err != nil {
		t.Fatal(err)
	}
	if err := provider.GrantPermissions(context.Background(), "editor", "article.read", "article.denied", "article.broken"); err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRoles(context.Background(), "subject-1", authorization.ScopeGlobal, "editor"); err != nil {
		t.Fatal(err)
	}
	counter := &countingProvider{inner: provider}
	engine := authorization.NewEngine(
		authorization.WithProvider(counter),
		authorization.WithNotFoundOnDeny("article.hidden"),
		authorization.WithRestriction("article.denied", func(context.Context, authorization.PolicyInput) (bool, error) {
			return false, nil
		}),
		authorization.WithRestriction("article.broken", func(context.Context, authorization.PolicyInput) (bool, error) {
			return false, errors.New("policy failure")
		}),
		// A restriction whose answer depends on the state of the resource:
		// the state can change during the request, which is why a final
		// decision may never be cached (AUTHZ-606).
		authorization.WithRestriction("article.stateful", func(_ context.Context, input authorization.PolicyInput) (bool, error) {
			article, ok := input.Resource.(*lockableArticle)

			return ok && !article.locked, nil
		}),
	)
	if err := authorization.Publish(engine); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	t.Run("HasPermission", func(t *testing.T) {
		c, _ := newTestContext(t, subject)
		if !c.HasPermission("article.read") {
			t.Error("expected the granted permission to be reported")
		}
		if c.HasPermission("article.delete") {
			t.Error("expected the missing permission to be reported false")
		}
		if c.IsAborted() {
			t.Error("HasPermission must have no HTTP effect")
		}

		c, _ = newTestContext(t, nil)
		if c.HasPermission("article.read") {
			t.Error("expected false for an anonymous request")
		}
	})

	t.Run("Can", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if !c.Can("article.read", nil) {
			t.Error("expected Can to allow the granted permission")
		}
		if c.Can("article.denied", nil) {
			t.Error("expected Can to report the restriction denial")
		}
		if c.IsAborted() || recorder.Body.Len() != 0 {
			t.Error("Can must have no HTTP effect")
		}
	})

	t.Run("AuthorizeAction allowed", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if !c.AuthorizeAction("article.read", nil) {
			t.Error("expected the action to be authorized")
		}
		if c.IsAborted() || recorder.Body.Len() != 0 {
			t.Error("an allowed decision must not write a response")
		}
	})

	t.Run("AuthorizeAction forbidden", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if c.AuthorizeAction("article.delete", nil) {
			t.Error("expected the missing permission to be denied")
		}
		assertAborted(t, c, recorder, http.StatusForbidden)
	})

	t.Run("AuthorizeAction masked as not found", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if c.AuthorizeAction("article.hidden", nil) {
			t.Error("expected the masked action to be denied")
		}
		assertAborted(t, c, recorder, http.StatusNotFound)
	})

	t.Run("AuthorizeAction unauthenticated", func(t *testing.T) {
		c, recorder := newTestContext(t, nil)
		if c.AuthorizeAction("article.read", nil) {
			t.Error("expected the anonymous request to be denied")
		}
		assertAborted(t, c, recorder, http.StatusUnauthorized)
	})

	t.Run("AuthorizeAction policy error", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if c.AuthorizeAction("article.broken", nil) {
			t.Error("a technical failure must never authorize")
		}
		assertAborted(t, c, recorder, http.StatusServiceUnavailable)
	})

	t.Run("Authorize uses the declared route permission", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		core.SetRoutePermission(c.Gin(), "article.read")
		if !c.Authorize(nil) {
			t.Error("expected the declared permission to authorize")
		}
		if c.IsAborted() || recorder.Body.Len() != 0 {
			t.Error("an allowed decision must not write a response")
		}

		c, recorder = newTestContext(t, subject)
		core.SetRoutePermission(c.Gin(), "article.denied")
		if c.Authorize(nil) {
			t.Error("expected the restriction to deny")
		}
		assertAborted(t, c, recorder, http.StatusForbidden)
	})

	t.Run("Authorize on a public route is misconfigured", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		core.SetRoutePermission(c.Gin(), authorization.Public)
		if c.Authorize(nil) {
			t.Error("Authorize is undefined on a Public route")
		}
		assertAborted(t, c, recorder, http.StatusInternalServerError)
	})

	t.Run("Authorize without a declared permission is misconfigured", func(t *testing.T) {
		c, recorder := newTestContext(t, subject)
		if c.Authorize(nil) {
			t.Error("Authorize is undefined without a declared permission")
		}
		assertAborted(t, c, recorder, http.StatusInternalServerError)
	})

	// AUTHZ-601: several helpers called on the same request share one grant
	// resolution, because the Context helpers put the per-request cache on
	// c.Request the same way the RBAC middleware does.
	t.Run("the helpers of one request share the grant cache", func(t *testing.T) {
		c := newScopedContext(t, subject, "tenant-1")
		counter.countAndReset()

		if !c.HasPermission("article.read") {
			t.Error("expected the granted permission to be reported")
		}
		if !c.Can("article.read", nil) {
			t.Error("expected Can to allow the granted permission")
		}
		if !c.AuthorizeAction("article.read", nil) {
			t.Error("expected AuthorizeAction to allow the granted permission")
		}

		// The union {tenant-1, ScopeGlobal} resolved once for three checks.
		if got := counter.countAndReset(); got != 2 {
			t.Errorf("provider calls = %d, want 2 for the whole request", got)
		}
	})

	// AUTHZ-606: only the RBAC grants are memoized. The restriction is
	// re-evaluated on every call and sees the resource as it is then.
	t.Run("restrictions are re-evaluated despite the cache", func(t *testing.T) {
		if err := provider.GrantPermissions(context.Background(), "editor", "article.stateful"); err != nil {
			t.Fatal(err)
		}

		c := newScopedContext(t, subject, "tenant-1")
		counter.countAndReset()

		article := &lockableArticle{}
		if !c.Can("article.stateful", article) {
			t.Fatal("expected the unlocked article to be authorized")
		}

		// The request itself changes the state of the resource.
		article.locked = true
		if c.Can("article.stateful", article) {
			t.Error("a final decision must never be cached: the restriction has to run again")
		}

		if got := counter.countAndReset(); got != 2 {
			t.Errorf("provider calls = %d, want 2: the grants alone are memoized", got)
		}
	})

	// AUTHZ-602: the cache belongs to the request, so a revocation is visible
	// from the next request on, with nothing to invalidate.
	t.Run("a revocation is visible on the next request", func(t *testing.T) {
		ctx := context.Background()

		first := newScopedContext(t, subject, "tenant-1")
		if !first.HasPermission("article.read") {
			t.Fatal("expected the permission before the revocation")
		}

		if err := provider.UnbindRoles(ctx, "subject-1", authorization.ScopeGlobal, "editor"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := provider.BindRoles(ctx, "subject-1", authorization.ScopeGlobal, "editor"); err != nil {
				t.Fatal(err)
			}
		})

		second := newScopedContext(t, subject, "tenant-1")
		if second.HasPermission("article.read") {
			t.Error("the revocation must be visible on the next request")
		}

		// Within the request that already resolved them, the grants stay
		// stable: that coherence is the point of a per-request cache.
		if !first.HasPermission("article.read") {
			t.Error("the grants of a request in flight must stay coherent")
		}
	})
}
