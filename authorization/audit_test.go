package authorization

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// stubAdmin is a GrantAdmin recording the calls it receives and returning a
// configurable error, so the decorator can be tested on both outcomes without
// a store.
type stubAdmin struct {
	calls []string
	err   error
}

func (s *stubAdmin) record(call string) error {
	s.calls = append(s.calls, call)

	return s.err
}

func (s *stubAdmin) CreateRole(_ context.Context, role string) error {
	return s.record("CreateRole:" + role)
}

func (s *stubAdmin) DeleteRole(_ context.Context, role string) error {
	return s.record("DeleteRole:" + role)
}

func (s *stubAdmin) GrantPermissions(_ context.Context, role string, _ ...Action) error {
	return s.record("GrantPermissions:" + role)
}

func (s *stubAdmin) RevokePermissions(_ context.Context, role string, _ ...Action) error {
	return s.record("RevokePermissions:" + role)
}

func (s *stubAdmin) BindRoles(_ context.Context, subjectID string, _ Scope, _ ...string) error {
	return s.record("BindRoles:" + subjectID)
}

func (s *stubAdmin) UnbindRoles(_ context.Context, subjectID string, _ Scope, _ ...string) error {
	return s.record("UnbindRoles:" + subjectID)
}

func (s *stubAdmin) Roles(context.Context) ([]string, error) {
	return []string{"editor"}, s.record("Roles")
}

func (s *stubAdmin) Permissions(context.Context) ([]Action, error) {
	return []Action{"article.read"}, s.record("Permissions")
}

func (s *stubAdmin) RolePermissions(_ context.Context, role string) ([]Action, error) {
	return []Action{"article.read"}, s.record("RolePermissions:" + role)
}

func (s *stubAdmin) Bindings(_ context.Context, subjectID string) ([]Binding, error) {
	return []Binding{{SubjectID: subjectID, Scope: ScopeGlobal, Role: "editor"}}, s.record("Bindings:" + subjectID)
}

func (s *stubAdmin) RoleBindings(_ context.Context, role string) ([]Binding, error) {
	return []Binding{{SubjectID: "subject-1", Scope: ScopeGlobal, Role: role}}, s.record("RoleBindings:" + role)
}

// runMutations drives every mutation of the contract once, in a fixed order.
func runMutations(t *testing.T, admin GrantAdmin) {
	t.Helper()

	ctx := context.Background()
	mutations := []func() error{
		func() error { return admin.CreateRole(ctx, "editor") },
		func() error { return admin.GrantPermissions(ctx, "editor", "article.read", "article.update") },
		func() error { return admin.RevokePermissions(ctx, "editor", "article.update") },
		func() error { return admin.BindRoles(ctx, "subject-1", "tenant-1", "editor", "reviewer") },
		func() error { return admin.UnbindRoles(ctx, "subject-1", "tenant-1", "reviewer") },
		func() error { return admin.DeleteRole(ctx, "editor") },
	}
	for _, mutate := range mutations {
		_ = mutate()
	}
}

// wantedEvents is the audit trail every mutation must produce, with err set
// to the error the store returned.
func wantedEvents(err error) []AuditEvent {
	return []AuditEvent{
		{Mutation: MutationCreateRole, Roles: []string{"editor"}, Err: err},
		{Mutation: MutationGrantPermissions, Roles: []string{"editor"}, Permissions: []Action{"article.read", "article.update"}, Err: err},
		{Mutation: MutationRevokePermissions, Roles: []string{"editor"}, Permissions: []Action{"article.update"}, Err: err},
		{Mutation: MutationBindRoles, Roles: []string{"editor", "reviewer"}, SubjectID: "subject-1", Scope: "tenant-1", Err: err},
		{Mutation: MutationUnbindRoles, Roles: []string{"reviewer"}, SubjectID: "subject-1", Scope: "tenant-1", Err: err},
		{Mutation: MutationDeleteRole, Roles: []string{"editor"}, Err: err},
	}
}

// TestAuditGrantAdminEmitsOnSuccess covers AUTHZ-603 for successful
// mutations: every mutation of the contract produces one event carrying the
// operation, the roles, the subject, the scope and the permissions.
func TestAuditGrantAdminEmitsOnSuccess(t *testing.T) {
	var events []AuditEvent
	store := &stubAdmin{}
	admin := AuditGrantAdmin(store, func(_ context.Context, event AuditEvent) {
		events = append(events, event)
	})

	runMutations(t, admin)

	if !reflect.DeepEqual(events, wantedEvents(nil)) {
		t.Errorf("audit trail =\n%+v\nwant\n%+v", events, wantedEvents(nil))
	}
	if len(store.calls) != len(events) {
		t.Errorf("store received %d calls for %d events", len(store.calls), len(events))
	}
}

// TestAuditGrantAdminEmitsOnFailure proves a refused mutation is audited too
// — the very attempt an audit trail exists to record — and that the
// decorator returns the store's error untouched.
func TestAuditGrantAdminEmitsOnFailure(t *testing.T) {
	failure := errors.New("unique constraint violated")

	var events []AuditEvent
	admin := AuditGrantAdmin(&stubAdmin{err: failure}, func(_ context.Context, event AuditEvent) {
		events = append(events, event)
		if !errors.Is(event.Err, failure) {
			t.Errorf("event %q carries err = %v, want the store failure", event.Mutation, event.Err)
		}
	})

	runMutations(t, admin)

	if !reflect.DeepEqual(events, wantedEvents(failure)) {
		t.Errorf("audit trail =\n%+v\nwant\n%+v", events, wantedEvents(failure))
	}

	if err := admin.CreateRole(context.Background(), "editor"); !errors.Is(err, failure) {
		t.Errorf("CreateRole error = %v, want the store failure unchanged", err)
	}
}

// TestAuditGrantAdminReadsArePassedThrough proves the decorator stays a
// complete GrantAdmin and that reads emit nothing.
func TestAuditGrantAdminReadsArePassedThrough(t *testing.T) {
	var events []AuditEvent
	store := &stubAdmin{}
	admin := AuditGrantAdmin(store, func(_ context.Context, event AuditEvent) {
		events = append(events, event)
	})

	ctx := context.Background()
	if roles, err := admin.Roles(ctx); err != nil || len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("Roles = (%v, %v)", roles, err)
	}
	if permissions, err := admin.Permissions(ctx); err != nil || len(permissions) != 1 {
		t.Errorf("Permissions = (%v, %v)", permissions, err)
	}
	if permissions, err := admin.RolePermissions(ctx, "editor"); err != nil || len(permissions) != 1 {
		t.Errorf("RolePermissions = (%v, %v)", permissions, err)
	}
	if bindings, err := admin.Bindings(ctx, "subject-1"); err != nil || len(bindings) != 1 {
		t.Errorf("Bindings = (%v, %v)", bindings, err)
	}
	if bindings, err := admin.RoleBindings(ctx, "editor"); err != nil || len(bindings) != 1 {
		t.Errorf("RoleBindings = (%v, %v)", bindings, err)
	}

	if len(events) != 0 {
		t.Errorf("reads emitted %d audit events, want none", len(events))
	}
	if len(store.calls) != 5 {
		t.Errorf("store received %d read calls, want 5", len(store.calls))
	}
}

// TestAuditHookSeesTheContext pins the documented way to name the actor of a
// mutation: the store does not know it, so the application reads it from the
// context the hook receives.
func TestAuditHookSeesTheContext(t *testing.T) {
	var actor string
	admin := AuditGrantAdmin(&stubAdmin{}, func(ctx context.Context, _ AuditEvent) {
		subject, _ := SubjectFromContext(ctx)
		actor = subject.ID
	})

	ctx := WithSubject(context.Background(), Subject{ID: "admin-7"})
	if err := admin.CreateRole(ctx, "editor"); err != nil {
		t.Fatal(err)
	}

	if actor != "admin-7" {
		t.Errorf("actor = %q, want admin-7", actor)
	}
}

// TestAuditEventPermissionsAreACopy proves the caller's variadic slice is not
// shared with the hook: a hook keeping its events for a batched writer must
// not see them change afterwards.
func TestAuditEventPermissionsAreACopy(t *testing.T) {
	var captured AuditEvent
	admin := AuditGrantAdmin(&stubAdmin{}, func(_ context.Context, event AuditEvent) {
		captured = event
	})

	actions := []Action{"article.read", "article.update"}
	if err := admin.GrantPermissions(context.Background(), "editor", actions...); err != nil {
		t.Fatal(err)
	}

	actions[0] = "article.delete"
	if captured.Permissions[0] != "article.read" {
		t.Error("the audited permissions must be a copy of the caller's slice")
	}

	roles := []string{"editor"}
	if err := admin.BindRoles(context.Background(), "subject-1", ScopeGlobal, roles...); err != nil {
		t.Fatal(err)
	}
	roles[0] = "admin"
	if captured.Roles[0] != "editor" {
		t.Error("the audited roles must be a copy of the caller's slice")
	}
}

// TestAuditHookPanicIsContained proves a panicking audit hook neither breaks
// the mutation nor changes the error the store returned.
func TestAuditHookPanicIsContained(t *testing.T) {
	panicking := func(context.Context, AuditEvent) {
		panic("audit sink exploded")
	}

	store := &stubAdmin{}
	admin := AuditGrantAdmin(store, panicking)
	if err := admin.CreateRole(context.Background(), "editor"); err != nil {
		t.Errorf("CreateRole error = %v, want nil despite the panicking hook", err)
	}
	if len(store.calls) != 1 {
		t.Errorf("store calls = %v, want the mutation to have happened", store.calls)
	}

	failure := errors.New("store unavailable")
	failing := AuditGrantAdmin(&stubAdmin{err: failure}, panicking)
	if err := failing.BindRoles(context.Background(), "subject-1", ScopeGlobal, "editor"); !errors.Is(err, failure) {
		t.Errorf("BindRoles error = %v, want the store failure unchanged", err)
	}
}

// TestAuditGrantAdminWithoutHookOrAdmin pins the degenerate wirings: the
// decorator never becomes the thing that breaks administration.
func TestAuditGrantAdminWithoutHookOrAdmin(t *testing.T) {
	store := &stubAdmin{}
	if got := AuditGrantAdmin(store, nil); got != GrantAdmin(store) {
		t.Error("a nil hook must return the admin unchanged")
	}
	if got := AuditGrantAdmin(nil, func(context.Context, AuditEvent) {}); got != nil {
		t.Error("a nil admin must be returned as is")
	}
}
