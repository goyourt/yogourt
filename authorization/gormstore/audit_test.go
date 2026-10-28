package gormstore_test

import (
	"context"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

// TestAuditedStoreEmitsForEveryMutation checks the audit decorator
// (AUTHZ-603) against the real SQL store: the trail records every mutation of
// the GrantAdmin contract, the store keeps behaving exactly as undecorated,
// and a mutation refused by the database is audited with its error.
func TestAuditedStoreEmitsForEveryMutation(t *testing.T) {
	db, store := setup(t)
	ctx := authorization.WithSubject(context.Background(), authorization.Subject{ID: "admin-7"})

	var events []authorization.AuditEvent
	admin := authorization.AuditGrantAdmin(store, func(hookCtx context.Context, event authorization.AuditEvent) {
		// The store does not know the actor: it comes from the context.
		actor, _ := authorization.SubjectFromContext(hookCtx)
		if actor.ID != "admin-7" {
			t.Errorf("actor = %q, want admin-7", actor.ID)
		}
		events = append(events, event)
	})

	if err := admin.CreateRole(ctx, "editor"); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := admin.GrantPermissions(ctx, "editor", "article.read", "article.update"); err != nil {
		t.Fatalf("grant permissions: %v", err)
	}
	if err := admin.RevokePermissions(ctx, "editor", "article.update"); err != nil {
		t.Fatalf("revoke permissions: %v", err)
	}
	if err := admin.BindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatalf("bind roles: %v", err)
	}

	// The decoration changes nothing of what the store did.
	grants := resolve(t, store, "subject-1", "tenant-1")
	if !grants.HasPermission("article.read") || grants.HasPermission("article.update") {
		t.Errorf("grants = %+v, want article.read only", grants)
	}

	// A mutation the database refuses: binding a role that does not exist.
	if err := admin.BindRoles(ctx, "subject-1", "tenant-1", "ghost"); err == nil {
		t.Fatal("expected binding an unknown role to fail")
	}

	if err := admin.UnbindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatalf("unbind roles: %v", err)
	}
	if err := admin.DeleteRole(ctx, "editor"); err != nil {
		t.Fatalf("delete role: %v", err)
	}

	// Reads pass through and emit nothing.
	if _, err := admin.Roles(ctx); err != nil {
		t.Fatalf("roles: %v", err)
	}
	if _, err := admin.RoleBindings(ctx, "editor"); err != nil {
		t.Fatalf("role bindings: %v", err)
	}

	wanted := []struct {
		mutation authorization.GrantMutation
		failed   bool
	}{
		{authorization.MutationCreateRole, false},
		{authorization.MutationGrantPermissions, false},
		{authorization.MutationRevokePermissions, false},
		{authorization.MutationBindRoles, false},
		{authorization.MutationBindRoles, true},
		{authorization.MutationUnbindRoles, false},
		{authorization.MutationDeleteRole, false},
	}
	if len(events) != len(wanted) {
		t.Fatalf("audit trail has %d events, want %d: %+v", len(events), len(wanted), events)
	}
	for i, want := range wanted {
		if events[i].Mutation != want.mutation {
			t.Errorf("event %d = %q, want %q", i, events[i].Mutation, want.mutation)
		}
		if failed := events[i].Err != nil; failed != want.failed {
			t.Errorf("event %d (%s) err = %v, want failed = %t", i, events[i].Mutation, events[i].Err, want.failed)
		}
	}

	// The refused binding left no row behind, and the audit trail is the only
	// trace of the attempt.
	if got := countRows(t, db, "authz_role_bindings"); got != 0 {
		t.Errorf("authz_role_bindings has %d rows, want 0", got)
	}
}
