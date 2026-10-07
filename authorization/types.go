// Package authorization provides the pure authorization engine of Yogourt:
// RBAC permission checks followed by optional ABAC restrictions, with deny by
// default. It depends only on the standard library (no Gin, no GORM).
package authorization

// Action identifies an operation to authorize, e.g. "article.update".
type Action string

// Scope is an opaque identifier, typically a tenant or an organization.
type Scope string

// ScopeGlobal is the global scope. Grant resolution always unions the
// requested scope with ScopeGlobal; it is a dedicated constant, not a pattern.
// Its value starts with '@' so that application-derived scopes (tenant slugs,
// organization names...) cannot collide with it; never build a Scope from raw
// user input without validating it, or a user could name a tenant after this
// sentinel and receive global grants.
const ScopeGlobal Scope = "@global"

// Public is the explicit exemption value used in route Permissions maps to
// mark a method as publicly accessible. Its value starts with '@' so that a
// business permission named "public" stays a regular permission and cannot
// silently disable the RBAC middleware.
const Public = "@public"

// Subject is the authenticated caller. ID is the stable identity (UUID).
// Attributes carries optional data needed by policies, such as the internal
// SQL id under the "internal_id" key.
type Subject struct {
	ID         string
	Attributes map[string]any
}

// SubjectResolver is implemented by application values — typically the user
// model — able to describe themselves as an authorization Subject. The
// framework uses it after a successful authentication to attach the subject
// to the request context via WithSubject.
type SubjectResolver interface {
	AuthorizationSubject() Subject
}

// Request is a full authorization question submitted to the engine.
type Request struct {
	Subject  Subject
	Action   Action
	Scope    Scope
	Resource any
}

// Reason explains a Decision. Reasons are meant for logs and metrics and must
// never be written into HTTP response bodies.
type Reason string

const (
	ReasonAllowed           Reason = "allowed"
	ReasonUnauthenticated   Reason = "unauthenticated"
	ReasonMissingPermission Reason = "missing_permission"
	ReasonPolicyDenied      Reason = "policy_denied"
	ReasonPolicyError       Reason = "policy_error"
	ReasonProviderError     Reason = "provider_error"
	ReasonMisconfigured     Reason = "misconfigured"
)

// Decision is the outcome of an authorization request.
type Decision struct {
	Allowed bool
	Reason  Reason
}
