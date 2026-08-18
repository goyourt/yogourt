package core_test

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
	if err := provider.CreateRole("editor"); err != nil {
		t.Fatal(err)
	}
	if err := provider.GrantPermissions("editor", "article.read", "article.denied", "article.broken"); err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRoles("subject-1", authorization.ScopeGlobal, "editor"); err != nil {
		t.Fatal(err)
	}
	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithNotFoundOnDeny("article.hidden"),
		authorization.WithRestriction("article.denied", func(context.Context, authorization.PolicyInput) (bool, error) {
			return false, nil
		}),
		authorization.WithRestriction("article.broken", func(context.Context, authorization.PolicyInput) (bool, error) {
			return false, errors.New("policy failure")
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
}
