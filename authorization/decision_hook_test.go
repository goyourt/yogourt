package authorization

import (
	"context"
	"reflect"
	"testing"
)

// recordingHook collects the events it receives.
type recordingHook struct {
	events []DecisionEvent
}

func (r *recordingHook) hook(_ context.Context, event DecisionEvent) {
	r.events = append(r.events, event)
}

func (r *recordingHook) last(t *testing.T) DecisionEvent {
	t.Helper()

	if len(r.events) == 0 {
		t.Fatal("no decision event was recorded")
	}

	return r.events[len(r.events)-1]
}

// TestDecisionHookReceivesTheDecision covers AUTHZ-604: a hook sees the
// subject identity, the action, the scope, the outcome, the typed reason and
// the duration of each decision.
func TestDecisionHookReceivesTheDecision(t *testing.T) {
	const action Action = "article.update"

	recorder := &recordingHook{}
	provider := newCountingProvider().grant("subject-1", "tenant-1", action)
	engine := NewEngine(
		WithProvider(provider),
		WithDecisionHook(recorder.hook),
		WithRestriction(action, func(_ context.Context, input PolicyInput) (bool, error) {
			article, _ := input.Resource.(*mutableArticle)

			return !article.locked, nil
		}),
	)

	ctx := context.Background()

	engine.Decide(ctx, Request{
		Subject:  Subject{ID: "subject-1", Attributes: map[string]any{"email": "user@example.test"}},
		Action:   action,
		Scope:    "tenant-1",
		Resource: &mutableArticle{},
	})

	event := recorder.last(t)
	if event.Kind != KindFull {
		t.Errorf("kind = %q, want %q", event.Kind, KindFull)
	}
	if event.SubjectID != "subject-1" {
		t.Errorf("subject = %q, want subject-1", event.SubjectID)
	}
	if event.Action != action {
		t.Errorf("action = %q, want %q", event.Action, action)
	}
	if event.Scope != "tenant-1" {
		t.Errorf("scope = %q, want tenant-1", event.Scope)
	}
	if !event.Allowed || event.Reason != ReasonAllowed {
		t.Errorf("outcome = (%v, %q), want (true, %q)", event.Allowed, event.Reason, ReasonAllowed)
	}
	if event.Duration < 0 {
		t.Errorf("duration = %v, want a non-negative measure", event.Duration)
	}

	// A denial by a restriction reports the ABAC reason, not the RBAC one.
	engine.Decide(ctx, Request{
		Subject:  Subject{ID: "subject-1"},
		Action:   action,
		Scope:    "tenant-1",
		Resource: &mutableArticle{locked: true},
	})
	if event := recorder.last(t); event.Allowed || event.Reason != ReasonPolicyDenied {
		t.Errorf("denied event = %+v, want denied with %q", event, ReasonPolicyDenied)
	}

	// An anonymous request never reaches the provider but is still reported.
	engine.Decide(ctx, Request{Action: action, Scope: "tenant-1"})
	if event := recorder.last(t); event.Allowed || event.Reason != ReasonUnauthenticated {
		t.Errorf("anonymous event = %+v, want denied with %q", event, ReasonUnauthenticated)
	}
}

// TestDecisionHookOnPermissionCheck proves the RBAC-only question — the one
// the middleware asks, where most refusals happen — is reported too, and
// distinguishable from a full decision.
func TestDecisionHookOnPermissionCheck(t *testing.T) {
	const action Action = "article.read"

	recorder := &recordingHook{}
	provider := newCountingProvider().grant("subject-1", ScopeGlobal, action)
	engine := NewEngine(WithProvider(provider), WithDecisionHook(recorder.hook))

	if _, err := engine.HasPermission(context.Background(), Subject{ID: "subject-1"}, ScopeGlobal, action); err != nil {
		t.Fatal(err)
	}
	event := recorder.last(t)
	if event.Kind != KindPermission {
		t.Errorf("kind = %q, want %q", event.Kind, KindPermission)
	}
	if !event.Allowed || event.Reason != ReasonAllowed {
		t.Errorf("event = %+v, want allowed", event)
	}

	if _, err := engine.HasPermission(context.Background(), Subject{ID: "subject-1"}, ScopeGlobal, "article.delete"); err != nil {
		t.Fatal(err)
	}
	if event := recorder.last(t); event.Allowed || event.Reason != ReasonMissingPermission {
		t.Errorf("event = %+v, want denied with %q", event, ReasonMissingPermission)
	}

	// An anonymous subject is refused without error, and reported as such.
	if _, err := engine.HasPermission(context.Background(), Subject{}, ScopeGlobal, action); err != nil {
		t.Fatal(err)
	}
	if event := recorder.last(t); event.Reason != ReasonUnauthenticated {
		t.Errorf("reason = %q, want %q", event.Reason, ReasonUnauthenticated)
	}

	// A misconfigured engine is reported as such, not as a store failure.
	engineWithoutProvider := NewEngine(WithDecisionHook(recorder.hook))
	if _, err := engineWithoutProvider.HasPermission(context.Background(), Subject{ID: "subject-1"}, ScopeGlobal, action); err == nil {
		t.Fatal("expected ErrNoProvider")
	}
	if event := recorder.last(t); event.Reason != ReasonMisconfigured {
		t.Errorf("reason = %q, want %q", event.Reason, ReasonMisconfigured)
	}

	// A store failure is reported with the provider reason.
	failing := newCountingProvider()
	failing.failures = 1
	engineWithFailure := NewEngine(WithProvider(failing), WithDecisionHook(recorder.hook))
	if _, err := engineWithFailure.HasPermission(context.Background(), Subject{ID: "subject-1"}, ScopeGlobal, action); err == nil {
		t.Fatal("expected the provider error to be returned")
	}
	if event := recorder.last(t); event.Reason != ReasonProviderError {
		t.Errorf("reason = %q, want %q", event.Reason, ReasonProviderError)
	}
}

// TestDecisionEventCarriesNoResource covers AUTHZ-605 structurally: the event
// type has no field able to hold a business object, so no hook can log one by
// accident. Adding such a field must break this test.
func TestDecisionEventCarriesNoResource(t *testing.T) {
	eventType := reflect.TypeOf(DecisionEvent{})
	for i := range eventType.NumField() {
		field := eventType.Field(i)
		switch field.Type.Kind() {
		case reflect.String, reflect.Bool, reflect.Int64:
		default:
			t.Errorf("DecisionEvent.%s is a %s: the event must only carry scalars, never a resource, a subject or a map of attributes",
				field.Name, field.Type.Kind())
		}
	}

	// Belt and braces: a hook must not be able to reach the resource through
	// the event, whatever the shape of the type.
	resource := &mutableArticle{}
	var seen []DecisionEvent
	engine := NewEngine(
		WithProvider(newCountingProvider().grant("subject-1", ScopeGlobal, "article.read")),
		WithDecisionHook(func(_ context.Context, event DecisionEvent) {
			seen = append(seen, event)
		}),
	)
	engine.Decide(context.Background(), Request{
		Subject:  Subject{ID: "subject-1", Attributes: map[string]any{"secret": "shh"}},
		Action:   "article.read",
		Scope:    ScopeGlobal,
		Resource: resource,
	})
	if len(seen) != 1 {
		t.Fatalf("recorded %d events, want 1", len(seen))
	}
	if reflect.ValueOf(seen[0]).NumField() != eventType.NumField() {
		t.Fatal("unexpected event shape")
	}
}

// TestDecisionHookCannotChangeTheDecision proves a hook is an observer: it
// runs after the fact, and neither its mutations of the event nor a panic
// change what the caller is told.
func TestDecisionHookCannotChangeTheDecision(t *testing.T) {
	const action Action = "article.read"

	provider := newCountingProvider().grant("subject-1", ScopeGlobal, action)
	tampering := func(_ context.Context, event DecisionEvent) {
		event.Allowed = !event.Allowed
		event.Reason = "tampered"
	}
	engine := NewEngine(WithProvider(provider), WithDecisionHook(tampering))

	// Allowed stays allowed.
	if decision := engine.Decide(context.Background(), Request{
		Subject: Subject{ID: "subject-1"},
		Action:  action,
		Scope:   ScopeGlobal,
	}); !decision.Allowed || decision.Reason != ReasonAllowed {
		t.Errorf("decision = %+v, want allowed", decision)
	}

	// Denied stays denied.
	if decision := engine.Decide(context.Background(), Request{
		Subject: Subject{ID: "subject-1"},
		Action:  "article.delete",
		Scope:   ScopeGlobal,
	}); decision.Allowed {
		t.Error("a hook must never be able to authorize a denied request")
	}
}

// TestDecisionHookPanicIsContained proves a panicking hook breaks neither the
// decision nor the hooks registered after it.
func TestDecisionHookPanicIsContained(t *testing.T) {
	const action Action = "article.read"

	provider := newCountingProvider().grant("subject-1", ScopeGlobal, action)
	recorder := &recordingHook{}
	engine := NewEngine(
		WithProvider(provider),
		WithDecisionHook(func(context.Context, DecisionEvent) {
			panic("metrics backend exploded")
		}),
		WithDecisionHook(recorder.hook),
		WithDecisionHook(nil),
	)

	decision := engine.Decide(context.Background(), Request{
		Subject: Subject{ID: "subject-1"},
		Action:  action,
		Scope:   ScopeGlobal,
	})
	if !decision.Allowed || decision.Reason != ReasonAllowed {
		t.Errorf("decision = %+v, want allowed despite the panicking hook", decision)
	}

	allowed, err := engine.HasPermission(context.Background(), Subject{ID: "subject-1"}, ScopeGlobal, action)
	if err != nil || !allowed {
		t.Errorf("HasPermission = (%v, %v), want (true, nil) despite the panicking hook", allowed, err)
	}

	if len(recorder.events) != 2 {
		t.Errorf("the hook after the panicking one received %d events, want 2", len(recorder.events))
	}
}

// TestDecisionHooksCalledInOrder pins that several hooks are supported and
// called in registration order.
func TestDecisionHooksCalledInOrder(t *testing.T) {
	var order []string
	engine := NewEngine(
		WithProvider(newCountingProvider()),
		WithDecisionHook(func(context.Context, DecisionEvent) { order = append(order, "first") }),
		WithDecisionHook(func(context.Context, DecisionEvent) { order = append(order, "second") }),
	)

	engine.Decide(context.Background(), Request{
		Subject: Subject{ID: "subject-1"},
		Action:  "article.read",
		Scope:   ScopeGlobal,
	})

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("hook order = %v, want [first second]", order)
	}
}
