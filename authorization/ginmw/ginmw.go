// Package ginmw provides the Gin middlewares of the authorization engine. It
// is the only authorization package allowed to import Gin.
package ginmw

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/core"
)

// MiddlewareFor returns a Gin middleware enforcing the RBAC permission for
// the given action against an explicit engine. The middleware records the
// action as the route permission, so that the handler can evaluate the ABAC
// restrictions through Context.Authorize once the resource is loaded (D5) —
// they are NOT evaluated here, the resource is not loaded yet at middleware
// time. On an authorization.Public action the permission is recorded but no
// RBAC check is made.
func MiddlewareFor(engine *authorization.Engine, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		core.SetPermission(c, action)
		if action != authorization.Public {
			enforce(c, engine, authorization.Action(action))
		}
	}
}

func enforce(c *gin.Context, engine *authorization.Engine, action authorization.Action) {
	if engine == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})

		return
	}

	ctx := attachGrantCache(c)
	subject, _ := authorization.SubjectFromContext(ctx)
	allowed, err := engine.HasPermission(ctx, subject, authorization.ScopeFromContext(ctx), action)
	if err != nil {
		reason := authorization.ReasonProviderError
		if errors.Is(err, authorization.ErrNoProvider) {
			reason = authorization.ReasonMisconfigured
		}
		core.AbortAfterReason(c, engine, action, reason)

		return
	}
	if subject.ID == "" {
		core.AbortAfterReason(c, engine, action, authorization.ReasonUnauthenticated)

		return
	}
	if !allowed {
		core.AbortAfterReason(c, engine, action, authorization.ReasonMissingPermission)

		return
	}

	c.Next()
}

// attachGrantCache makes sure the request carries the per-request grant cache
// and returns the request context to use for the check. The cache is created
// here because the middleware is where a request enters the authorization
// layer: the grants it resolves are then reused by the handler's
// Context.Authorize instead of being resolved a second time (AUTHZ-601).
//
// The request itself must be rewritten, not only the local context: Gin
// exposes the request context through c.Request, so a context kept in a local
// variable would stay invisible to the handler and to every later middleware
// — the same reason services.AttachSubject rewrites c.Request. The cache is
// created once per request, which is exactly what keeps a revocation visible
// from the next request on (AUTHZ-602).
func attachGrantCache(c *gin.Context) context.Context {
	if c.Request == nil {
		return context.Background()
	}

	ctx := c.Request.Context()
	cached := authorization.EnsureGrantCache(ctx)
	if cached != ctx {
		c.Request = c.Request.WithContext(cached)
	}

	return cached
}
