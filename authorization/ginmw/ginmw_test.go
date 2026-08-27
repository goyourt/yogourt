package ginmw

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type erroringProvider struct{}

func (erroringProvider) Resolve(context.Context, authorization.Subject, authorization.Scope) (authorization.Grants, error) {
	return authorization.Grants{}, errors.New("store unavailable")
}

func newTestEngine(t *testing.T, options ...authorization.Option) *authorization.Engine {
	t.Helper()

	provider := memory.NewProvider()
	if err := provider.CreateRole(context.Background(), "editor"); err != nil {
		t.Fatal(err)
	}
	if err := provider.GrantPermissions(context.Background(), "editor", "article.read"); err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRoles(context.Background(), "subject-1", authorization.ScopeGlobal, "editor"); err != nil {
		t.Fatal(err)
	}

	return authorization.NewEngine(append([]authorization.Option{authorization.WithProvider(provider)}, options...)...)
}

func serve(middleware gin.HandlerFunc, subject *authorization.Subject) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/articles", middleware, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	request := httptest.NewRequest(http.MethodGet, "/articles", nil)
	if subject != nil {
		request = request.WithContext(authorization.WithSubject(request.Context(), *subject))
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	return recorder
}

func assertGenericBody(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	// Internal reasons must never leak into response bodies (D7).
	body := recorder.Body.String()
	for _, reason := range []string{"missing_permission", "policy_denied", "policy_error", "provider_error", "misconfigured", "unauthenticated"} {
		if strings.Contains(body, reason) {
			t.Errorf("response body leaks internal reason %q: %s", reason, body)
		}
	}
}

func TestMiddlewareForAllowed(t *testing.T) {
	engine := newTestEngine(t)
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.read"), subject)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", recorder.Code)
	}
}

func TestMiddlewareForUnauthenticated(t *testing.T) {
	engine := newTestEngine(t)

	recorder := serve(MiddlewareFor(engine, "article.read"), nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestMiddlewareForForbidden(t *testing.T) {
	engine := newTestEngine(t)
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.delete"), subject)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestMiddlewareForNotFoundMasking(t *testing.T) {
	engine := newTestEngine(t, authorization.WithNotFoundOnDeny("article.delete"))
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.delete"), subject)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestMiddlewareForSkipsRestrictions(t *testing.T) {
	// The middleware enforces RBAC only: restrictions need the loaded
	// resource and run in the handler via Context.Authorize (D5). A denying
	// restriction must therefore not block the middleware.
	restrictionCalled := false
	deny := func(context.Context, authorization.PolicyInput) (bool, error) {
		restrictionCalled = true

		return false, nil
	}
	engine := newTestEngine(t, authorization.WithRestriction("article.read", deny))
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.read"), subject)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: the RBAC middleware must not evaluate restrictions", recorder.Code)
	}
	if restrictionCalled {
		t.Error("restrictions must not be evaluated at middleware time (no resource yet)")
	}
}

func TestMiddlewareForProviderError(t *testing.T) {
	engine := authorization.NewEngine(authorization.WithProvider(erroringProvider{}))
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.read"), subject)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestMiddlewareForErroringRestrictionDoesNotBlockRBAC(t *testing.T) {
	// An erroring restriction would wrongly answer 503 at middleware time if
	// restrictions were evaluated against the nil resource. RBAC-only: pass.
	failing := func(context.Context, authorization.PolicyInput) (bool, error) {
		return false, errors.New("policy failure")
	}
	engine := newTestEngine(t, authorization.WithRestriction("article.read", failing))
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.read"), subject)
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200: restrictions belong to the handler, not the middleware", recorder.Code)
	}
}

func TestMiddlewareForMisconfigured(t *testing.T) {
	// No provider: the engine denies with misconfigured, mapped to 500.
	engine := authorization.NewEngine()
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(MiddlewareFor(engine, "article.read"), subject)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
	assertGenericBody(t, recorder)

	recorder = serve(MiddlewareFor(nil, "article.read"), subject)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status with nil engine = %d, want 500", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestMiddlewareForScopeFromContext(t *testing.T) {
	provider := memory.NewProvider()
	if err := provider.CreateRole(context.Background(), "editor"); err != nil {
		t.Fatal(err)
	}
	if err := provider.GrantPermissions(context.Background(), "editor", "article.read"); err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRoles(context.Background(), "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatal(err)
	}
	engine := authorization.NewEngine(authorization.WithProvider(provider))

	router := gin.New()
	router.GET("/articles", MiddlewareFor(engine, "article.read"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	request := httptest.NewRequest(http.MethodGet, "/articles", nil)
	ctx := authorization.WithSubject(request.Context(), authorization.Subject{ID: "subject-1"})
	ctx = authorization.WithScope(ctx, "tenant-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request.WithContext(ctx))
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for the tenant-scoped binding", recorder.Code)
	}

	// Without the scope in the context, resolution falls back to ScopeGlobal
	// where the subject has no binding.
	recorder = serve(MiddlewareFor(engine, "article.read"), &authorization.Subject{ID: "subject-1"})
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 outside the bound scope", recorder.Code)
	}
}

// TestLazyMiddleware exercises ginmw.Middleware around the process-wide
// published engine. The storage is write-once, so a single test drives both
// the unpublished (500) and the published paths, in that order.
func TestLazyMiddleware(t *testing.T) {
	subject := &authorization.Subject{ID: "subject-1"}

	recorder := serve(Middleware("article.read"), subject)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status before Publish = %d, want 500", recorder.Code)
	}
	assertGenericBody(t, recorder)

	if err := authorization.Publish(newTestEngine(t)); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	recorder = serve(Middleware("article.read"), subject)
	if recorder.Code != http.StatusOK {
		t.Errorf("status after Publish = %d, want 200", recorder.Code)
	}

	recorder = serve(Middleware("article.read"), nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status for anonymous request = %d, want 401", recorder.Code)
	}
	assertGenericBody(t, recorder)

	recorder = serve(Middleware("article.delete"), subject)
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status for missing permission = %d, want 403", recorder.Code)
	}
	assertGenericBody(t, recorder)
}

func TestDeniedHandlerNeverExecuted(t *testing.T) {
	engine := newTestEngine(t)
	handlerCalled := false

	router := gin.New()
	router.GET("/articles", MiddlewareFor(engine, "article.delete"), func(c *gin.Context) {
		handlerCalled = true
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	request := httptest.NewRequest(http.MethodGet, "/articles", nil)
	request = request.WithContext(authorization.WithSubject(request.Context(), authorization.Subject{ID: "subject-1"}))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if handlerCalled {
		t.Error("a denied request must never reach the handler")
	}
	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", recorder.Code)
	}
}
