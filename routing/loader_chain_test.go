package routing

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
)

// serveChain runs one request through the exact handler chain the loader
// would register, with an optional authenticated subject.
func serveChain(t *testing.T, chain []gin.HandlerFunc, subject *authorization.Subject) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.GET("/resource", chain...)

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	if subject != nil {
		req = req.WithContext(authorization.WithSubject(req.Context(), *subject))
	}
	r.ServeHTTP(w, req)

	return w
}

func grantedEngine(t *testing.T, subjectID string, action authorization.Action) *authorization.Engine {
	t.Helper()

	provider := memory.NewProvider()
	if err := provider.CreateRole("reader"); err != nil {
		t.Fatal(err)
	}
	if err := provider.GrantPermissions("reader", action); err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRoles(subjectID, authorization.ScopeGlobal, "reader"); err != nil {
		t.Fatal(err)
	}

	return authorization.NewEngine(authorization.WithProvider(provider))
}

func TestRouteHandlerChainWithoutAuthorizerIsUnchanged(t *testing.T) {
	baseRan := false
	base := []gin.HandlerFunc{func(c *gin.Context) { baseRan = true }}
	handlerRan := false
	handler := func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	}

	chain := routeHandlerChain(nil, "", base, handler)

	// D1: without an authorizer the chain is strictly base middlewares + handler.
	if len(chain) != len(base)+1 {
		t.Fatalf("expected %d handlers without authorizer, got %d", len(base)+1, len(chain))
	}
	w := serveChain(t, chain, nil)
	if !baseRan || !handlerRan || w.Code != http.StatusOK {
		t.Errorf("anonymous request should reach the handler: base=%v handler=%v code=%d", baseRan, handlerRan, w.Code)
	}
}

func TestRouteHandlerChainPublicSkipsRBAC(t *testing.T) {
	engine := grantedEngine(t, "11111111-1111-1111-1111-111111111111", "article.read")
	handlerRan := false
	handler := func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	}

	chain := routeHandlerChain(engine, authorization.Public, nil, handler)

	// Permission-recording middleware + handler, but no RBAC middleware.
	if len(chain) != 2 {
		t.Fatalf("expected 2 handlers for a public method, got %d", len(chain))
	}
	w := serveChain(t, chain, nil)
	if !handlerRan || w.Code != http.StatusOK {
		t.Errorf("anonymous request on a public route must reach the handler, got code=%d handler=%v", w.Code, handlerRan)
	}
}

func TestRouteHandlerChainProtectedDeniesAnonymous(t *testing.T) {
	engine := grantedEngine(t, "11111111-1111-1111-1111-111111111111", "article.read")
	baseRan := false
	base := []gin.HandlerFunc{func(c *gin.Context) { baseRan = true }}
	handlerRan := false
	handler := func(c *gin.Context) { handlerRan = true }

	chain := routeHandlerChain(engine, "article.read", base, handler)

	// Inherited callbacks, permission recording, RBAC last, then the handler.
	if len(chain) != 4 {
		t.Fatalf("expected 4 handlers for a protected method, got %d", len(chain))
	}
	w := serveChain(t, chain, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anonymous request on a protected route must get 401, got %d", w.Code)
	}
	if !baseRan {
		t.Error("inherited callbacks must run before the RBAC middleware")
	}
	if handlerRan {
		t.Error("a denied handler must never be executed")
	}
}

func TestRouteHandlerChainProtectedForbidsUngrantedSubject(t *testing.T) {
	engine := grantedEngine(t, "11111111-1111-1111-1111-111111111111", "article.read")
	handlerRan := false
	handler := func(c *gin.Context) { handlerRan = true }

	chain := routeHandlerChain(engine, "article.read", nil, handler)

	stranger := authorization.Subject{ID: "22222222-2222-2222-2222-222222222222"}
	w := serveChain(t, chain, &stranger)
	if w.Code != http.StatusForbidden {
		t.Errorf("subject without the permission must get 403, got %d", w.Code)
	}
	if handlerRan {
		t.Error("a denied handler must never be executed")
	}
}

func TestRouteHandlerChainProtectedAllowsGrantedSubject(t *testing.T) {
	subjectID := "11111111-1111-1111-1111-111111111111"
	engine := grantedEngine(t, subjectID, "article.read")
	handlerRan := false
	handler := func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	}

	chain := routeHandlerChain(engine, "article.read", nil, handler)

	granted := authorization.Subject{ID: subjectID}
	w := serveChain(t, chain, &granted)
	if !handlerRan || w.Code != http.StatusOK {
		t.Errorf("granted subject must reach the handler, got code=%d handler=%v", w.Code, handlerRan)
	}
}
