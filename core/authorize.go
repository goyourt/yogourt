package core

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/ginmw"
)

// routePermissionKey is the Gin context key carrying the permission declared
// for the matched route method. It is written by the routing layer and
// consumed by Context.Authorize.
const routePermissionKey = "yogourt.route_permission"

// SetRoutePermission records the permission declared for the matched route
// method. It is called by the framework when a route is registered with an
// authorizer configured; applications should not need it.
func SetRoutePermission(c *gin.Context, permission string) {
	c.Set(routePermissionKey, permission)
}

// HasPermission answers the RBAC question alone for the current subject,
// without evaluating restrictions and without any HTTP effect. It reports
// false when no engine is configured or on any technical failure.
func (c *Context) HasPermission(action string) bool {
	engine, ok := authorization.Published()
	if !ok {
		return false
	}

	ctx := c.authorizationContext()
	subject, _ := authorization.SubjectFromContext(ctx)
	allowed, err := engine.HasPermission(ctx, subject, authorization.ScopeFromContext(ctx), authorization.Action(action))

	return err == nil && allowed
}

// Can evaluates the full decision (RBAC then ABAC restrictions) for an
// explicit action and a loaded resource, without any HTTP effect. It reports
// false when no engine is configured.
func (c *Context) Can(action string, resource any) bool {
	engine, ok := authorization.Published()
	if !ok {
		return false
	}

	return c.decide(engine, authorization.Action(action), resource).Allowed
}

// Authorize evaluates the full decision for the permission declared by the
// route for the current method, writing the denial status and aborting the
// request when access is refused. It must be called after loading the
// resource and before any mutation or response write (D5). On a route
// declared authorization.Public no permission is defined: the call is
// misconfigured and responds 500 — use Can or AuthorizeAction there.
func (c *Context) Authorize(resource any) bool {
	permission, ok := c.routePermission()
	if !ok || permission == authorization.Public {
		ginmw.AbortDenied(c.Context, nil, "", authorization.ReasonMisconfigured)

		return false
	}

	return c.AuthorizeAction(permission, resource)
}

// AuthorizeAction behaves like Authorize for an explicit action instead of
// the one declared by the route.
func (c *Context) AuthorizeAction(action string, resource any) bool {
	engine, ok := authorization.Published()
	if !ok {
		ginmw.AbortDenied(c.Context, nil, authorization.Action(action), authorization.ReasonMisconfigured)

		return false
	}

	decision := c.decide(engine, authorization.Action(action), resource)
	if decision.Allowed {
		return true
	}
	ginmw.AbortDenied(c.Context, engine, authorization.Action(action), decision.Reason)

	return false
}

func (c *Context) decide(engine *authorization.Engine, action authorization.Action, resource any) authorization.Decision {
	ctx := c.authorizationContext()
	subject, _ := authorization.SubjectFromContext(ctx)

	return engine.Decide(ctx, authorization.Request{
		Subject:  subject,
		Action:   action,
		Scope:    authorization.ScopeFromContext(ctx),
		Resource: resource,
	})
}

// authorizationContext returns the context every authorization helper of the
// request must use: the request context, made sure to carry the per-request
// grant cache (AUTHZ-601). A handler calling Authorize then Can then
// HasPermission therefore resolves the grants of the subject once, and shares
// that resolution with the RBAC middleware that already ran — which is where
// the cache usually comes from.
//
// The request is rewritten when a cache had to be created, so that the
// helpers called later in the same handler see it: the context of a request
// only lives in c.Request, exactly as for the subject attached by
// services.AttachSubject.
//
// A Context without a request — a unit test building one by hand — keeps
// working with no memoization at all: the background context carries no
// cache, so every check queries the provider as before.
//
// The rewrite happens in the request's goroutine, at most once per request
// since the RBAC middleware usually installed the cache already. A handler
// fanning out into goroutines must hand them the context (or a c.Copy()), as
// Gin already requires — the cache itself is safe for concurrent use, a
// *gin.Context never was.
func (c *Context) authorizationContext() context.Context {
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

func (c *Context) routePermission() (string, bool) {
	value, exists := c.Context.Get(routePermissionKey)
	if !exists {
		return "", false
	}
	permission, ok := value.(string)
	if !ok || permission == "" {
		return "", false
	}

	return permission, true
}
