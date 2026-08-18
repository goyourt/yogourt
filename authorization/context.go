package authorization

import "context"

type contextKey int

const (
	subjectContextKey contextKey = iota
	scopeContextKey
)

// WithSubject attaches the authenticated subject to the context. This is the
// identity contract used by the HTTP middlewares.
func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectContextKey, subject)
}

// SubjectFromContext returns the subject attached to the context, if any.
func SubjectFromContext(ctx context.Context) (Subject, bool) {
	subject, ok := ctx.Value(subjectContextKey).(Subject)

	return subject, ok
}

// WithScope attaches the authorization scope of the current request to the
// context.
func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeContextKey, scope)
}

// ScopeFromContext returns the scope attached to the context, defaulting to
// ScopeGlobal when none was set.
func ScopeFromContext(ctx context.Context) Scope {
	if scope, ok := ctx.Value(scopeContextKey).(Scope); ok {
		return scope
	}

	return ScopeGlobal
}
