package authorization

import (
	"context"
	"errors"
	"log"
	"time"
)

// DecisionKind tells which question a DecisionEvent answers, so that metrics
// stay honest: a request crossing the RBAC middleware and then calling
// Context.Authorize in its handler produces two events, one of each kind, and
// counting them as one population would inflate both the totals and the
// refusal rate.
type DecisionKind string

const (
	// KindFull is a complete decision: the RBAC permission check followed by
	// the ABAC restrictions of the action (Engine.Decide).
	KindFull DecisionKind = "full"
	// KindPermission is the RBAC-only question, without any restriction
	// (Engine.HasPermission) — what the RBAC middleware asks.
	KindPermission DecisionKind = "permission"
)

// DecisionEvent describes one decision, for logs and metrics. Every field is
// a plain scalar: an event can be logged or turned into metric labels as is.
//
// It deliberately does NOT carry the business resource (AUTHZ-605), and does
// not carry the Subject either — only its stable identity. Structuring the
// event outside the hook is the only real guarantee that a whole object never
// reaches a log: a hook receiving the resource would be one fmt.Sprintf("%+v")
// away from writing a user row, an order or a medical record into a log file
// or a metric label, and the subject Attributes map is an open door of the
// same kind. A hook that genuinely needs to name the resource logs the
// identifier it already has at its call site, where the sensitivity of each
// field is known.
//
// Duration measures the decision itself — the provider resolution and the
// restrictions — and is only measured when at least one hook is registered,
// so an engine without hooks keeps reading no clock at all.
type DecisionEvent struct {
	// Kind is the question that was answered.
	Kind DecisionKind
	// SubjectID is the stable identity of the caller, empty for an anonymous
	// request.
	SubjectID string
	// Action is the permission that was checked.
	Action Action
	// Scope is the scope the check was made in, before the ScopeGlobal union.
	Scope Scope
	// Allowed is the outcome.
	Allowed bool
	// Reason is the typed reason of the outcome. It belongs to logs and
	// metrics only and must never be written into an HTTP response body (D7).
	Reason Reason
	// Duration is how long the decision took.
	Duration time.Duration
}

// DecisionHook observes decisions. It returns nothing: a hook is an observer,
// never a participant, so no hook can turn a refusal into an authorization —
// which also means a buggy or hostile hook cannot open a door.
//
// The context of the decision is passed so a hook can reach request-scoped
// values it needs (a trace span, a request id, the actor). It is called
// synchronously, in the request's goroutine: a hook doing anything slow must
// hand its work to its own queue rather than block the decision.
type DecisionHook func(ctx context.Context, event DecisionEvent)

// notify reports one event to every registered hook.
func (e *Engine) notify(ctx context.Context, event DecisionEvent) {
	for _, hook := range e.decisionHooks {
		callDecisionHook(ctx, hook, event)
	}
}

// callDecisionHook calls one hook, containing its panics. Observability must
// never be able to break a request: the decision was already taken when the
// hook runs, so a panicking hook is logged and skipped, and the engine's
// answer stands. Each hook is isolated, so one panicking hook does not
// silence the others.
func callDecisionHook(ctx context.Context, hook DecisionHook, event DecisionEvent) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("authorization: decision hook panicked: %v", recovered)
		}
	}()

	hook(ctx, event)
}

// permissionReason maps the (bool, error) answer of an RBAC-only check to the
// typed reason a hook expects.
func permissionReason(subject Subject, allowed bool, err error) Reason {
	switch {
	case errors.Is(err, ErrNoProvider):
		return ReasonMisconfigured
	case err != nil:
		return ReasonProviderError
	case subject.ID == "":
		return ReasonUnauthenticated
	case allowed:
		return ReasonAllowed
	default:
		return ReasonMissingPermission
	}
}
