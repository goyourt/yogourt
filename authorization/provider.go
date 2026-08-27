package authorization

import "context"

// Grants holds the roles and permissions resolved for a subject in a scope.
type Grants struct {
	Roles       []string
	Permissions []Action
}

// HasRole reports whether the grants include the given role.
func (g Grants) HasRole(role string) bool {
	for _, r := range g.Roles {
		if r == role {
			return true
		}
	}

	return false
}

// HasPermission reports whether the grants include the given permission.
func (g Grants) HasPermission(action Action) bool {
	for _, p := range g.Permissions {
		if p == action {
			return true
		}
	}

	return false
}

// clone returns a copy of the grants with independent slices, so that a
// caller receiving it — a Restriction through PolicyInput — cannot mutate the
// grants the engine holds. It matters because grant resolution is meant to be
// memoized per request: a restriction appending to or reordering the slices it
// receives would otherwise poison every later decision of the same request.
func (g Grants) clone() Grants {
	return Grants{
		Roles:       append([]string(nil), g.Roles...),
		Permissions: append([]Action(nil), g.Permissions...),
	}
}

// GrantProvider resolves the grants of a subject for a single scope. The
// engine is responsible for the union with ScopeGlobal; providers must only
// return grants bound to the exact scope they are asked about.
type GrantProvider interface {
	Resolve(ctx context.Context, subject Subject, scope Scope) (Grants, error)
}

// Binding is one role binding: a subject holding a role within a scope.
type Binding struct {
	SubjectID string
	Scope     Scope
	Role      string
}

// GrantAdmin is optionally implemented by providers whose grants can be
// administered at runtime — typically from an admin web interface. Mutations
// are expected to be idempotent, and every read exists so a UI can be built
// without reaching into the store's schema: list the roles, list the
// permissions the framework registered from the routes, show what a role
// grants, and show who holds it.
//
// Permissions themselves are never created by hand: they are registered by
// the boot synchronization (PermissionSyncer) from the permissions the routes
// declare or derive, and GrantPermissions registers an unknown one on the fly.
// An admin interface therefore offers a closed list to pick from.
type GrantAdmin interface {
	CreateRole(ctx context.Context, role string) error
	DeleteRole(ctx context.Context, role string) error
	GrantPermissions(ctx context.Context, role string, actions ...Action) error
	RevokePermissions(ctx context.Context, role string, actions ...Action) error
	BindRoles(ctx context.Context, subjectID string, scope Scope, roles ...string) error
	UnbindRoles(ctx context.Context, subjectID string, scope Scope, roles ...string) error

	// Roles lists every role, sorted by name.
	Roles(ctx context.Context) ([]string, error)
	// Permissions lists every registered permission, sorted by name.
	Permissions(ctx context.Context) ([]Action, error)
	// RolePermissions lists the permissions of one role, sorted by name.
	RolePermissions(ctx context.Context, role string) ([]Action, error)
	// Bindings lists every binding of one subject, across all scopes.
	Bindings(ctx context.Context, subjectID string) ([]Binding, error)
	// RoleBindings lists every subject holding one role, with its scope.
	RoleBindings(ctx context.Context, role string) ([]Binding, error)
}

// PermissionSyncer is optionally implemented by providers able to register
// the permissions the application declares (route Permissions maps,
// WithKnownPermissions), so that no permission row is ever inserted by hand.
// Synchronization is additive only: it never deletes anything (AUTHZ-512).
type PermissionSyncer interface {
	SyncPermissions(ctx context.Context, permissions []Action) error
}
