package authorization

import (
	"context"
	"testing"
)

func TestSubjectContext(t *testing.T) {
	ctx := context.Background()

	if _, ok := SubjectFromContext(ctx); ok {
		t.Error("expected no subject on a bare context")
	}

	subject := Subject{ID: "uuid-1", Attributes: map[string]any{"internal_id": 42}}
	ctx = WithSubject(ctx, subject)

	got, ok := SubjectFromContext(ctx)
	if !ok {
		t.Fatal("expected a subject after WithSubject")
	}
	if got.ID != "uuid-1" || got.Attributes["internal_id"] != 42 {
		t.Errorf("SubjectFromContext = %+v", got)
	}
}

func TestScopeContext(t *testing.T) {
	ctx := context.Background()

	if scope := ScopeFromContext(ctx); scope != ScopeGlobal {
		t.Errorf("default scope = %q, want %q", scope, ScopeGlobal)
	}

	ctx = WithScope(ctx, "tenant-1")
	if scope := ScopeFromContext(ctx); scope != "tenant-1" {
		t.Errorf("scope = %q, want tenant-1", scope)
	}
}
