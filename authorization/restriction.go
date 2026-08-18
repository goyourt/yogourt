package authorization

import "context"

// PolicyInput is what a Restriction receives: the subject, the action, the
// scope and the loaded resource under evaluation.
type PolicyInput struct {
	Subject  Subject
	Action   Action
	Scope    Scope
	Resource any
}

// Restriction is an ABAC check evaluated after the RBAC permission check.
// It returns whether the request is allowed, or an error when the policy
// itself fails; a policy error never grants access.
type Restriction func(ctx context.Context, input PolicyInput) (bool, error)
