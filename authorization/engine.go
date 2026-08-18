package authorization

import "context"

// Engine evaluates authorization requests: RBAC permission check first, then
// the ABAC restrictions registered for the action. It is immutable once built
// by NewEngine and safe for concurrent use.
type Engine struct {
	provider         GrantProvider
	restrictions     map[Action][]Restriction
	notFoundOnDeny   map[Action]bool
	knownPermissions []Action
}

// Option configures an Engine at construction time.
type Option func(*Engine)

// NewEngine builds an immutable Engine from the given options.
func NewEngine(options ...Option) *Engine {
	engine := &Engine{
		restrictions:   make(map[Action][]Restriction),
		notFoundOnDeny: make(map[Action]bool),
	}
	for _, option := range options {
		option(engine)
	}

	return engine
}

// WithProvider sets the grant provider used to resolve roles and permissions.
func WithProvider(provider GrantProvider) Option {
	return func(e *Engine) {
		e.provider = provider
	}
}

// WithRestriction registers a restriction for an action. Multiple
// restrictions on the same action are combined with a logical AND.
func WithRestriction(action Action, restriction Restriction) Option {
	return func(e *Engine) {
		e.restrictions[action] = append(e.restrictions[action], restriction)
	}
}

// WithNotFoundOnDeny marks actions whose denials must be masked as 404
// instead of 403 by the HTTP layer.
func WithNotFoundOnDeny(actions ...Action) Option {
	return func(e *Engine) {
		for _, action := range actions {
			e.notFoundOnDeny[action] = true
		}
	}
}

// WithKnownPermissions declares the full list of permissions the application
// knows about, enabling strict validation of route declarations at boot.
func WithKnownPermissions(perms ...Action) Option {
	return func(e *Engine) {
		e.knownPermissions = append(e.knownPermissions, perms...)
	}
}

// KnownPermissions returns the declared permission list, if any.
func (e *Engine) KnownPermissions() []Action {
	perms := make([]Action, len(e.knownPermissions))
	copy(perms, e.knownPermissions)

	return perms
}

// NotFoundOnDeny reports whether denials of the given action must be masked
// as 404.
func (e *Engine) NotFoundOnDeny(action Action) bool {
	return e.notFoundOnDeny[action]
}

// Decide evaluates a request. Deny by default: any technical failure results
// in a denied decision with the matching reason, never in an authorization.
func (e *Engine) Decide(ctx context.Context, request Request) Decision {
	if request.Subject.ID == "" {
		return Decision{Reason: ReasonUnauthenticated}
	}
	if e.provider == nil {
		return Decision{Reason: ReasonMisconfigured}
	}

	grants, err := e.resolveGrants(ctx, request.Subject, request.Scope)
	if err != nil {
		return Decision{Reason: ReasonProviderError}
	}

	// Missing RBAC permission short-circuits: restrictions are never evaluated.
	if !grants.HasPermission(request.Action) {
		return Decision{Reason: ReasonMissingPermission}
	}

	input := PolicyInput{
		Subject:  request.Subject,
		Action:   request.Action,
		Scope:    request.Scope,
		Resource: request.Resource,
	}
	for _, restriction := range e.restrictions[request.Action] {
		allowed, err := restriction(ctx, input)
		if err != nil {
			return Decision{Reason: ReasonPolicyError}
		}
		if !allowed {
			return Decision{Reason: ReasonPolicyDenied}
		}
	}

	return Decision{Allowed: true, Reason: ReasonAllowed}
}

// HasPermission answers the RBAC question alone, without evaluating
// restrictions. It reports false without error for an anonymous subject.
func (e *Engine) HasPermission(ctx context.Context, subject Subject, scope Scope, action Action) (bool, error) {
	if subject.ID == "" {
		return false, nil
	}
	if e.provider == nil {
		return false, ErrNoProvider
	}

	grants, err := e.resolveGrants(ctx, subject, scope)
	if err != nil {
		return false, err
	}

	return grants.HasPermission(action), nil
}

// resolveGrants resolves the union of the grants bound to the requested scope
// and to ScopeGlobal. When the requested scope is ScopeGlobal, the provider
// is called only once.
func (e *Engine) resolveGrants(ctx context.Context, subject Subject, scope Scope) (Grants, error) {
	grants, err := e.provider.Resolve(ctx, subject, scope)
	if err != nil {
		return Grants{}, err
	}
	if scope == ScopeGlobal {
		return grants, nil
	}

	global, err := e.provider.Resolve(ctx, subject, ScopeGlobal)
	if err != nil {
		return Grants{}, err
	}

	return mergeGrants(grants, global), nil
}

func mergeGrants(a, b Grants) Grants {
	merged := Grants{
		Roles:       append([]string(nil), a.Roles...),
		Permissions: append([]Action(nil), a.Permissions...),
	}
	for _, role := range b.Roles {
		if !merged.HasRole(role) {
			merged.Roles = append(merged.Roles, role)
		}
	}
	for _, perm := range b.Permissions {
		if !merged.HasPermission(perm) {
			merged.Permissions = append(merged.Permissions, perm)
		}
	}

	return merged
}
