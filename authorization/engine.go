package authorization

import (
	"context"
	"time"
)

// Engine evaluates authorization requests: RBAC permission check first, then
// the ABAC restrictions registered for the action. It is immutable once built
// by NewEngine and safe for concurrent use.
type Engine struct {
	provider         GrantProvider
	restrictions     map[Action][]Restriction
	notFoundOnDeny   map[Action]bool
	knownPermissions []Action
	decisionHooks    []DecisionHook
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

// WithDecisionHook registers a hook notified of every decision the engine
// takes, for logs and metrics. Several hooks can be registered; they are
// called in registration order. A nil hook is ignored.
//
// The hook cannot influence the decision: it receives values and returns
// nothing, and it is called once the decision is final. A panic inside a hook
// is recovered and does not affect the answer given to the caller.
func WithDecisionHook(hook DecisionHook) Option {
	return func(e *Engine) {
		if hook == nil {
			return
		}
		e.decisionHooks = append(e.decisionHooks, hook)
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
//
// Every decision is reported to the hooks registered with WithDecisionHook,
// after it was taken and without any way to change it.
func (e *Engine) Decide(ctx context.Context, request Request) Decision {
	if len(e.decisionHooks) == 0 {
		return e.decide(ctx, request)
	}

	started := time.Now()
	decision := e.decide(ctx, request)
	e.notify(ctx, DecisionEvent{
		Kind:      KindFull,
		SubjectID: request.Subject.ID,
		Action:    request.Action,
		Scope:     request.Scope,
		Allowed:   decision.Allowed,
		Reason:    decision.Reason,
		Duration:  time.Since(started),
	})

	return decision
}

func (e *Engine) decide(ctx context.Context, request Request) Decision {
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

	restrictions := e.restrictions[request.Action]
	if len(restrictions) == 0 {
		return Decision{Allowed: true, Reason: ReasonAllowed}
	}

	// The grants are cloned once, and only when the action actually has
	// restrictions: the pure RBAC path — by far the common one — keeps
	// allocating nothing, while no restriction can reach the grants the engine
	// resolved and will memoize per request (see Grants.clone). The single copy
	// is shared by the restrictions of one action, which is enough for that
	// guarantee; restrictions are checks, not transformations, so a restriction
	// mutating its input remains a bug on its side.
	input := PolicyInput{
		Subject:  request.Subject,
		Action:   request.Action,
		Scope:    request.Scope,
		Resource: request.Resource,
		Grants:   grants.clone(),
	}
	for _, restriction := range restrictions {
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

// SyncPermissions registers the given permissions with the provider when it
// supports synchronization (PermissionSyncer); otherwise it is a no-op. The
// framework calls it at boot with every permission declared by the routes,
// so applications never insert permission rows by hand.
func (e *Engine) SyncPermissions(ctx context.Context, permissions []Action) error {
	if e.provider == nil || len(permissions) == 0 {
		return nil
	}
	syncer, ok := e.provider.(PermissionSyncer)
	if !ok {
		return nil
	}

	return syncer.SyncPermissions(ctx, permissions)
}

// HasPermission answers the RBAC question alone, without evaluating
// restrictions. It reports false without error for an anonymous subject.
//
// It is the question the RBAC middleware asks, so it is reported to the
// decision hooks as well — with Kind set to KindPermission, since most
// refusals of an application happen here and a hook that only saw Decide
// would miss them.
func (e *Engine) HasPermission(ctx context.Context, subject Subject, scope Scope, action Action) (bool, error) {
	if len(e.decisionHooks) == 0 {
		return e.hasPermission(ctx, subject, scope, action)
	}

	started := time.Now()
	allowed, err := e.hasPermission(ctx, subject, scope, action)
	e.notify(ctx, DecisionEvent{
		Kind:      KindPermission,
		SubjectID: subject.ID,
		Action:    action,
		Scope:     scope,
		Allowed:   allowed,
		Reason:    permissionReason(subject, allowed, err),
		Duration:  time.Since(started),
	})

	return allowed, err
}

func (e *Engine) hasPermission(ctx context.Context, subject Subject, scope Scope, action Action) (bool, error) {
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
//
// Each of the two resolutions goes through the per-request memoization when
// the context carries a grant cache (WithGrantCache): the union is rebuilt on
// every call — it costs no provider round-trip — while each (subject, scope)
// pair is asked of the provider at most once per request. Memoizing the two
// scopes separately also lets the ScopeGlobal answer be reused by checks made
// on different scopes within the same request.
func (e *Engine) resolveGrants(ctx context.Context, subject Subject, scope Scope) (Grants, error) {
	grants, err := resolveScopeGrants(ctx, e.provider, subject, scope)
	if err != nil {
		return Grants{}, err
	}
	if scope == ScopeGlobal {
		return grants, nil
	}

	global, err := resolveScopeGrants(ctx, e.provider, subject, ScopeGlobal)
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
