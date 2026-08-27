package authorization

import "context"

// PolicyInput is what a Restriction receives: the subject, the action, the
// scope, the loaded resource under evaluation and the RBAC grants already
// resolved for that subject and scope.
type PolicyInput struct {
	Subject  Subject
	Action   Action
	Scope    Scope
	Resource any

	// Grants holds the RBAC grants the engine already resolved for Subject on
	// the union {Scope, ScopeGlobal} — the very grants whose permission check
	// let the request reach this restriction. They are exposed so that a
	// complete rule can be expressed in a single place, typically "the
	// resource is public, OR the subject owns it, OR the subject holds an
	// escalation permission":
	//
	//	if profile.Public || owns(input) {
	//	    return true, nil
	//	}
	//
	//	return input.Grants.HasPermission("profiles.read_private"), nil
	//
	// No extra provider call is made to populate this field.
	//
	// Guardrail: express escalation with Grants.HasPermission — an explicit
	// permission, granted by the store and therefore auditable — and never
	// with Grants.HasRole("admin"). A role-name test would rebuild the
	// implicit admin bypass that deny by default forbids: it grants powers no
	// permission row records, it cannot be revoked without unbinding the whole
	// role, and it silently drifts as soon as roles are renamed or a second
	// privileged role appears. Grants.HasRole stays available for genuinely
	// role-shaped business rules (a workflow step reserved to "reviewer", for
	// instance), not for bypassing the rule being evaluated.
	//
	// The slices are a defensive copy, so nothing a restriction does can reach
	// the grants the engine holds — which matters because grant resolution is
	// meant to be memoized per request. That copy is shared by the
	// restrictions of a single action though: treat this field as read-only,
	// an in-place edit would still be seen by the next restriction of the same
	// action.
	Grants Grants
}

// Restriction is an ABAC check evaluated after the RBAC permission check.
// It returns whether the request is allowed, or an error when the policy
// itself fails; a policy error never grants access.
type Restriction func(ctx context.Context, input PolicyInput) (bool, error)
