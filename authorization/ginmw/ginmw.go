// Package ginmw provides the Gin middlewares of the authorization engine. It
// is the only authorization package allowed to import Gin.
package ginmw

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
)

// MiddlewareFor returns a Gin middleware enforcing the RBAC permission for
// the given action against an explicit engine. ABAC restrictions are NOT
// evaluated here — the resource is not loaded yet at middleware time; they
// run in the handler through Context.Authorize (D5). It is usable in any Gin
// application, with or without Yogourt.
func MiddlewareFor(engine *authorization.Engine, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		enforce(c, engine, authorization.Action(action))
	}
}

// Middleware returns a Gin middleware enforcing the given action against the
// engine published via authorization.Publish. The engine is resolved lazily
// on each request; when none is published the request is aborted with 500.
func Middleware(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		engine, ok := authorization.Published()
		if !ok {
			abort(c, http.StatusInternalServerError, "Internal server error")

			return
		}
		enforce(c, engine, authorization.Action(action))
	}
}

func enforce(c *gin.Context, engine *authorization.Engine, action authorization.Action) {
	if engine == nil {
		abort(c, http.StatusInternalServerError, "Internal server error")

		return
	}

	ctx := c.Request.Context()
	subject, _ := authorization.SubjectFromContext(ctx)
	if subject.ID == "" {
		AbortDenied(c, engine, action, authorization.ReasonUnauthenticated)

		return
	}

	allowed, err := engine.HasPermission(ctx, subject, authorization.ScopeFromContext(ctx), action)
	if err != nil {
		reason := authorization.ReasonProviderError
		if errors.Is(err, authorization.ErrNoProvider) {
			reason = authorization.ReasonMisconfigured
		}
		AbortDenied(c, engine, action, reason)

		return
	}
	if !allowed {
		AbortDenied(c, engine, action, authorization.ReasonMissingPermission)

		return
	}

	c.Next()
}

// AbortDenied maps a denied decision to its HTTP status and aborts the
// request. Internal reasons never reach the response body: generic messages
// only (D7). The 404 masking of denials follows the engine's NotFoundOnDeny
// configuration for the action; a nil engine falls back to 403. It is shared
// by the RBAC middleware and by the Context authorization helpers so both
// layers answer identically (D8).
func AbortDenied(c *gin.Context, engine *authorization.Engine, action authorization.Action, reason authorization.Reason) {
	switch reason {
	case authorization.ReasonUnauthenticated:
		abort(c, http.StatusUnauthorized, "Unauthorized")
	case authorization.ReasonMissingPermission, authorization.ReasonPolicyDenied:
		if engine != nil && engine.NotFoundOnDeny(action) {
			abort(c, http.StatusNotFound, "Resource not found")
		} else {
			abort(c, http.StatusForbidden, "Forbidden")
		}
	case authorization.ReasonProviderError, authorization.ReasonPolicyError:
		abort(c, http.StatusServiceUnavailable, "Service unavailable")
	default:
		abort(c, http.StatusInternalServerError, "Internal server error")
	}
}

func abort(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}
