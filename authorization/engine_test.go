package authorization

import (
	"context"
	"errors"
	"testing"
)

type stubProvider struct {
	grants map[Scope]Grants
	err    error
	calls  []Scope
}

func (p *stubProvider) Resolve(_ context.Context, _ Subject, scope Scope) (Grants, error) {
	p.calls = append(p.calls, scope)
	if p.err != nil {
		return Grants{}, p.err
	}

	return p.grants[scope], nil
}

func subjectWithID(id string) Subject {
	return Subject{ID: id}
}

func grantingProvider(scope Scope, actions ...Action) *stubProvider {
	return &stubProvider{grants: map[Scope]Grants{scope: {Permissions: actions}}}
}

func trueRestriction(counter *int) Restriction {
	return func(context.Context, PolicyInput) (bool, error) {
		*counter++

		return true, nil
	}
}

func falseRestriction(counter *int) Restriction {
	return func(context.Context, PolicyInput) (bool, error) {
		*counter++

		return false, nil
	}
}

func errorRestriction(counter *int) Restriction {
	return func(context.Context, PolicyInput) (bool, error) {
		*counter++

		return false, errors.New("policy failure")
	}
}

// TestDecisionMatrix covers the mandatory 7-row matrix of the design
// document. Restriction call counters verify the RBAC short-circuit: a
// missing permission never evaluates restrictions.
func TestDecisionMatrix(t *testing.T) {
	const action Action = "article.update"

	tests := []struct {
		name          string
		hasPermission bool
		restrictions  []func(*int) Restriction
		wantAllowed   bool
		wantReason    Reason
		wantCalls     int
	}{
		{"permission absent, no restrictions", false, nil, false, ReasonMissingPermission, 0},
		{"permission absent, true restrictions", false, []func(*int) Restriction{trueRestriction, trueRestriction}, false, ReasonMissingPermission, 0},
		{"permission absent, erroring restriction", false, []func(*int) Restriction{errorRestriction}, false, ReasonMissingPermission, 0},
		{"permission present, no restrictions", true, nil, true, ReasonAllowed, 0},
		{"permission present, all restrictions true", true, []func(*int) Restriction{trueRestriction, trueRestriction}, true, ReasonAllowed, 2},
		{"permission present, one restriction false", true, []func(*int) Restriction{trueRestriction, falseRestriction}, false, ReasonPolicyDenied, 2},
		{"permission present, one restriction erroring", true, []func(*int) Restriction{errorRestriction}, false, ReasonPolicyError, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &stubProvider{grants: map[Scope]Grants{}}
			if tt.hasPermission {
				provider.grants[ScopeGlobal] = Grants{Permissions: []Action{action}}
			}

			calls := 0
			options := []Option{WithProvider(provider)}
			for _, restriction := range tt.restrictions {
				options = append(options, WithRestriction(action, restriction(&calls)))
			}
			engine := NewEngine(options...)

			decision := engine.Decide(context.Background(), Request{
				Subject: subjectWithID("subject-1"),
				Action:  action,
				Scope:   ScopeGlobal,
			})
			if decision.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", decision.Allowed, tt.wantAllowed)
			}
			if decision.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", decision.Reason, tt.wantReason)
			}
			if calls != tt.wantCalls {
				t.Errorf("restriction calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestDecideUnauthenticated(t *testing.T) {
	engine := NewEngine(WithProvider(grantingProvider(ScopeGlobal, "article.read")))

	decision := engine.Decide(context.Background(), Request{Action: "article.read", Scope: ScopeGlobal})
	if decision.Allowed || decision.Reason != ReasonUnauthenticated {
		t.Errorf("Decision = %+v, want denied with reason %q", decision, ReasonUnauthenticated)
	}
}

func TestDecideWithoutProvider(t *testing.T) {
	engine := NewEngine()

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  "article.read",
		Scope:   ScopeGlobal,
	})
	if decision.Allowed || decision.Reason != ReasonMisconfigured {
		t.Errorf("Decision = %+v, want denied with reason %q", decision, ReasonMisconfigured)
	}
}

func TestDecideProviderError(t *testing.T) {
	provider := &stubProvider{err: errors.New("store unavailable")}
	engine := NewEngine(WithProvider(provider))

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  "article.read",
		Scope:   ScopeGlobal,
	})
	if decision.Allowed || decision.Reason != ReasonProviderError {
		t.Errorf("Decision = %+v, want denied with reason %q", decision, ReasonProviderError)
	}
}

// TestScopeUnion verifies D4: a binding on ScopeGlobal is visible from a
// tenant scope, without any wildcard.
func TestScopeUnion(t *testing.T) {
	provider := grantingProvider(ScopeGlobal, "article.read")
	engine := NewEngine(WithProvider(provider))

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  "article.read",
		Scope:   "tenant-1",
	})
	if !decision.Allowed {
		t.Errorf("Decision = %+v, want allowed via ScopeGlobal union", decision)
	}
	if len(provider.calls) != 2 || provider.calls[0] != "tenant-1" || provider.calls[1] != ScopeGlobal {
		t.Errorf("provider calls = %v, want [tenant-1 global]", provider.calls)
	}
}

func TestScopeGlobalResolvedOnce(t *testing.T) {
	provider := grantingProvider(ScopeGlobal, "article.read")
	engine := NewEngine(WithProvider(provider))

	engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  "article.read",
		Scope:   ScopeGlobal,
	})
	if len(provider.calls) != 1 {
		t.Errorf("provider calls = %v, want a single resolution for ScopeGlobal", provider.calls)
	}
}

// TestScopeExactMatch verifies that a grant on one tenant scope gives nothing
// on another tenant scope: comparison is exact, no pattern matching.
func TestScopeExactMatch(t *testing.T) {
	provider := grantingProvider("tenant-1", "article.read")
	engine := NewEngine(WithProvider(provider))

	decision := engine.Decide(context.Background(), Request{
		Subject: subjectWithID("subject-1"),
		Action:  "article.read",
		Scope:   "tenant-2",
	})
	if decision.Allowed || decision.Reason != ReasonMissingPermission {
		t.Errorf("Decision = %+v, want denied with reason %q", decision, ReasonMissingPermission)
	}
}

func TestHasPermission(t *testing.T) {
	engine := NewEngine(WithProvider(grantingProvider(ScopeGlobal, "article.read")))

	ok, err := engine.HasPermission(context.Background(), subjectWithID("subject-1"), "tenant-1", "article.read")
	if err != nil || !ok {
		t.Errorf("HasPermission = (%v, %v), want (true, nil)", ok, err)
	}

	ok, err = engine.HasPermission(context.Background(), subjectWithID("subject-1"), "tenant-1", "article.delete")
	if err != nil || ok {
		t.Errorf("HasPermission = (%v, %v), want (false, nil)", ok, err)
	}

	ok, err = engine.HasPermission(context.Background(), Subject{}, ScopeGlobal, "article.read")
	if err != nil || ok {
		t.Errorf("HasPermission for anonymous subject = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestHasPermissionErrors(t *testing.T) {
	engine := NewEngine()
	if _, err := engine.HasPermission(context.Background(), subjectWithID("subject-1"), ScopeGlobal, "article.read"); !errors.Is(err, ErrNoProvider) {
		t.Errorf("err = %v, want ErrNoProvider", err)
	}

	providerErr := errors.New("store unavailable")
	engine = NewEngine(WithProvider(&stubProvider{err: providerErr}))
	ok, err := engine.HasPermission(context.Background(), subjectWithID("subject-1"), ScopeGlobal, "article.read")
	if ok || !errors.Is(err, providerErr) {
		t.Errorf("HasPermission = (%v, %v), want (false, provider error)", ok, err)
	}
}

func TestNotFoundOnDeny(t *testing.T) {
	engine := NewEngine(WithNotFoundOnDeny("article.read", "article.update"))

	if !engine.NotFoundOnDeny("article.read") || !engine.NotFoundOnDeny("article.update") {
		t.Error("expected declared actions to be masked")
	}
	if engine.NotFoundOnDeny("article.delete") {
		t.Error("expected undeclared action not to be masked")
	}
}

func TestKnownPermissions(t *testing.T) {
	engine := NewEngine(WithKnownPermissions("article.read", "article.update"))

	perms := engine.KnownPermissions()
	if len(perms) != 2 || perms[0] != "article.read" || perms[1] != "article.update" {
		t.Errorf("KnownPermissions = %v", perms)
	}

	// The returned slice is a copy: mutating it must not affect the engine.
	perms[0] = "tampered"
	if engine.KnownPermissions()[0] != "article.read" {
		t.Error("KnownPermissions must return a copy")
	}
}

func TestGrants(t *testing.T) {
	grants := Grants{Roles: []string{"editor"}, Permissions: []Action{"article.read"}}

	if !grants.HasRole("editor") || grants.HasRole("admin") {
		t.Error("HasRole mismatch")
	}
	if !grants.HasPermission("article.read") || grants.HasPermission("article.delete") {
		t.Error("HasPermission mismatch")
	}
}
