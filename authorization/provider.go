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

// GrantProvider resolves the grants of a subject for a single scope. The
// engine is responsible for the union with ScopeGlobal; providers must only
// return grants bound to the exact scope they are asked about.
type GrantProvider interface {
	Resolve(ctx context.Context, subject Subject, scope Scope) (Grants, error)
}
