package authorization

import (
	"context"
	"testing"
)

// TestPolicyInputGrants verifies that a restriction receives the grants the
// engine resolved, including those coming from the ScopeGlobal union.
func TestPolicyInputGrants(t *testing.T) {
	const action Action = "profiles.read"

	provider := &stubProvider{grants: map[Scope]Grants{
		"tenant-1": {Roles: []string{"member"}, Permissions: []Action{action}},
		ScopeGlobal: {
			Roles:       []string{"moderator"},
			Permissions: []Action{"profiles.read_private"},
		},
	}}

	var seen Grants
	engine := NewEngine(
		WithProvider(provider),
		WithRestriction(action, func(_ context.Context, input PolicyInput) (bool, error) {
			seen = input.Grants

			return true, nil
		}),
	)

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  action,
		Scope:   "tenant-1",
	})
	if !decision.Allowed {
		t.Fatalf("Decision = %+v, want allowed", decision)
	}

	for _, role := range []string{"member", "moderator"} {
		if !seen.HasRole(role) {
			t.Errorf("PolicyInput.Grants missing role %q, got %v", role, seen.Roles)
		}
	}
	for _, perm := range []Action{action, "profiles.read_private"} {
		if !seen.HasPermission(perm) {
			t.Errorf("PolicyInput.Grants missing permission %q, got %v", perm, seen.Permissions)
		}
	}
}

// TestPolicyInputGrantsAreACopy verifies the memoization safety net: a
// restriction mutating the slices it receives alters neither the engine state
// nor the following decisions.
func TestPolicyInputGrantsAreACopy(t *testing.T) {
	const action Action = "profiles.read"

	resolved := Grants{
		Roles:       []string{"member"},
		Permissions: []Action{action, "profiles.read_private"},
	}
	provider := &stubProvider{grants: map[Scope]Grants{ScopeGlobal: resolved}}

	var seen []Grants
	engine := NewEngine(
		WithProvider(provider),
		WithRestriction(action, func(_ context.Context, input PolicyInput) (bool, error) {
			// Snapshot what was received before tampering with it, so the
			// assertions below cannot observe this call's own mutation.
			seen = append(seen, input.Grants.clone())
			// A misbehaving restriction: it rewrites its input in place.
			for i := range input.Grants.Roles {
				input.Grants.Roles[i] = "tampered"
			}
			for i := range input.Grants.Permissions {
				input.Grants.Permissions[i] = "tampered"
			}

			return input.Grants.HasPermission("profiles.read_private"), nil
		}),
	)

	request := Request{Subject: subjectWithID("subject-1"), Action: action, Scope: ScopeGlobal}
	if decision := engine.Decide(context.Background(), request); decision.Allowed {
		t.Fatalf("Decision = %+v, want denied: the restriction erased its own criterion", decision)
	}

	// Second decision: unaffected by the previous mutation.
	if decision := engine.Decide(context.Background(), request); decision.Allowed {
		t.Fatalf("Decision = %+v, want denied for the same reason", decision)
	}
	if len(seen) != 2 {
		t.Fatalf("restriction calls = %d, want 2", len(seen))
	}
	if got := seen[1]; got.Roles[0] != "member" || got.Permissions[0] != action {
		t.Errorf("second call received tampered grants: %+v", got)
	}

	// The grants held by the provider — hence the engine's view of the store —
	// are intact.
	if resolved.Roles[0] != "member" || resolved.Permissions[0] != action {
		t.Errorf("provider grants were mutated: %+v", resolved)
	}
}

// TestGrantsNotClonedWithoutRestriction verifies that the pure RBAC path does
// not clone anything: with no restriction registered for the action, no
// PolicyInput is built at all.
func TestGrantsNotClonedWithoutRestriction(t *testing.T) {
	const action Action = "profiles.read"

	calls := 0
	engine := NewEngine(
		WithProvider(grantingProvider(ScopeGlobal, action)),
		// Registered on another action: it must never run, and the grants of
		// "profiles.read" must never be cloned for it.
		WithRestriction("profiles.update", falseRestriction(&calls)),
	)

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  action,
		Scope:   ScopeGlobal,
	})
	if !decision.Allowed || decision.Reason != ReasonAllowed {
		t.Errorf("Decision = %+v, want allowed", decision)
	}
	if calls != 0 {
		t.Errorf("restriction calls = %d, want 0", calls)
	}
}

// TestRestrictionNotCalledWithoutPermission re-asserts the RBAC short-circuit
// from the grants angle: when the permission is missing the restriction is
// never invoked, so no clone happens either.
func TestRestrictionNotCalledWithoutPermission(t *testing.T) {
	const action Action = "profiles.read"

	calls := 0
	engine := NewEngine(
		WithProvider(grantingProvider(ScopeGlobal, "profiles.read_private")),
		WithRestriction(action, trueRestriction(&calls)),
	)

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  action,
		Scope:   ScopeGlobal,
	})
	if decision.Allowed || decision.Reason != ReasonMissingPermission {
		t.Errorf("Decision = %+v, want denied with reason %q", decision, ReasonMissingPermission)
	}
	if calls != 0 {
		t.Errorf("restriction calls = %d, want 0", calls)
	}
}

func TestGrantsClone(t *testing.T) {
	original := Grants{Roles: []string{"member"}, Permissions: []Action{"profiles.read"}}

	copied := original.clone()
	copied.Roles[0] = "tampered"
	copied.Permissions[0] = "tampered"

	if original.Roles[0] != "member" || original.Permissions[0] != "profiles.read" {
		t.Errorf("clone shares its backing arrays: %+v", original)
	}

	// A zero value clones without panicking and stays empty.
	empty := Grants{}.clone()
	if len(empty.Roles) != 0 || len(empty.Permissions) != 0 {
		t.Errorf("clone of a zero Grants = %+v, want empty", empty)
	}
}

// profile is the resource of the reference "public OR owner OR escalation
// permission" pattern.
type profile struct {
	OwnerID int
	Public  bool
}

// readProfile is the complete rule expressed in a single restriction: a public
// profile is readable by anyone holding "profiles.read", a private one only by
// its owner or by a subject holding the explicit escalation permission. The
// escalation is tested with Grants.HasPermission, never with
// Grants.HasRole("admin").
func readProfile(_ context.Context, input PolicyInput) (bool, error) {
	resource, ok := input.Resource.(profile)
	if !ok {
		return false, nil
	}
	if resource.Public {
		return true, nil
	}
	if internalID, ok := input.Subject.Attributes["internal_id"].(int); ok && internalID == resource.OwnerID {
		return true, nil
	}

	return input.Grants.HasPermission("profiles.read_private"), nil
}

// TestPublicOrOwnerOrEscalation covers the four cases of the reference pattern.
func TestPublicOrOwnerOrEscalation(t *testing.T) {
	const action Action = "profiles.read"

	provider := &stubProvider{grants: map[Scope]Grants{}}
	engine := NewEngine(
		WithProvider(provider),
		WithRestriction(action, readProfile),
	)

	owner := Subject{ID: "owner", Attributes: map[string]any{"internal_id": 42}}
	moderator := Subject{ID: "moderator", Attributes: map[string]any{"internal_id": 7}}
	stranger := Subject{ID: "stranger", Attributes: map[string]any{"internal_id": 99}}

	// Everyone holds the base permission; only the moderator can escalate.
	base := Grants{Roles: []string{"member"}, Permissions: []Action{action}}
	provider.grants[ScopeGlobal] = base

	privateProfile := profile{OwnerID: 42}
	publicProfile := profile{OwnerID: 42, Public: true}

	tests := []struct {
		name        string
		subject     Subject
		grants      Grants
		resource    profile
		wantAllowed bool
		wantReason  Reason
	}{
		{"public profile, third party", stranger, base, publicProfile, true, ReasonAllowed},
		{"private profile, owner", owner, base, privateProfile, true, ReasonAllowed},
		{
			"private profile, moderator",
			moderator,
			Grants{Roles: []string{"moderator"}, Permissions: []Action{action, "profiles.read_private"}},
			privateProfile,
			true,
			ReasonAllowed,
		},
		{"private profile, third party", stranger, base, privateProfile, false, ReasonPolicyDenied},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider.grants[ScopeGlobal] = tt.grants

			decision := engine.Decide(context.Background(), Request{
				Subject:  tt.subject,
				Action:   action,
				Scope:    ScopeGlobal,
				Resource: tt.resource,
			})
			if decision.Allowed != tt.wantAllowed || decision.Reason != tt.wantReason {
				t.Errorf("Decision = %+v, want allowed=%v reason=%q", decision, tt.wantAllowed, tt.wantReason)
			}
		})
	}
}
