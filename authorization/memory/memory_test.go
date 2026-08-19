package memory

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

func TestCreateRoleIsIdempotent(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	if err := provider.CreateRole(ctx, "editor"); err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	mustGrant(t, provider, "editor", "article.read")

	// Re-creating an existing role is a no-op that keeps its permissions.
	if err := provider.CreateRole(ctx, "editor"); err != nil {
		t.Fatalf("CreateRole on an existing role: want no error, got %v", err)
	}
	if got := mustRolePermissions(t, provider, "editor"); !reflect.DeepEqual(got, []authorization.Action{"article.read"}) {
		t.Errorf("RolePermissions = %v, want [article.read]", got)
	}
	if got := mustRoles(t, provider); !reflect.DeepEqual(got, []string{"editor"}) {
		t.Errorf("Roles = %v, want [editor]", got)
	}
}

func TestGrantPermissionsUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.GrantPermissions(context.Background(), "ghost", "article.read"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestBindRolesUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.BindRoles(context.Background(), "subject-1", authorization.ScopeGlobal, "ghost"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestBindRolesRejectsBatchWithUnknownRole(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")

	if err := provider.BindRoles(ctx, "subject-1", "tenant-1", "editor", "ghost"); err == nil {
		t.Fatal("expected an error when one role of the batch is unknown")
	}
	if got := mustBindings(t, provider, "subject-1"); len(got) != 0 {
		t.Errorf("Bindings = %v, want none: the batch must not be applied partially", got)
	}
}

func TestDeleteRole(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustCreateRole(t, provider, "reviewer")
	mustGrant(t, provider, "editor", "article.read", "article.update")
	mustGrant(t, provider, "reviewer", "article.review")
	mustBind(t, provider, "subject-1", "tenant-1", "editor", "reviewer")

	if err := provider.DeleteRole(ctx, "editor"); err != nil {
		t.Fatalf("DeleteRole failed: %v", err)
	}
	if got := mustRoles(t, provider); !reflect.DeepEqual(got, []string{"reviewer"}) {
		t.Errorf("Roles = %v, want [reviewer]", got)
	}
	if _, err := provider.RolePermissions(ctx, "editor"); err == nil {
		t.Error("RolePermissions on a deleted role: want error, got nil")
	}

	// The binding of the deleted role is gone, the others survive.
	want := []authorization.Binding{{SubjectID: "subject-1", Scope: "tenant-1", Role: "reviewer"}}
	if got := mustBindings(t, provider, "subject-1"); !reflect.DeepEqual(got, want) {
		t.Errorf("Bindings = %v, want %v", got, want)
	}

	// Permissions are never deleted: an admin interface keeps offering them.
	wantPermissions := []authorization.Action{"article.read", "article.review", "article.update"}
	if got := mustPermissions(t, provider); !reflect.DeepEqual(got, wantPermissions) {
		t.Errorf("Permissions = %v, want %v", got, wantPermissions)
	}
}

func TestDeleteRoleDropsEmptyBinding(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustBind(t, provider, "subject-1", "tenant-1", "editor")

	if err := provider.DeleteRole(ctx, "editor"); err != nil {
		t.Fatalf("DeleteRole failed: %v", err)
	}
	if got := mustBindings(t, provider, "subject-1"); len(got) != 0 {
		t.Errorf("Bindings = %v, want none", got)
	}
	grants, err := provider.Resolve(ctx, authorization.Subject{ID: "subject-1"}, "tenant-1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(grants.Roles) != 0 {
		t.Errorf("grants = %+v, want none after the role was deleted", grants)
	}
}

func TestDeleteRoleUnknownRoleIsNoop(t *testing.T) {
	provider := NewProvider()

	if err := provider.DeleteRole(context.Background(), "ghost"); err != nil {
		t.Errorf("DeleteRole on an unknown role: want no error, got %v", err)
	}
}

func TestRevokePermissions(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustGrant(t, provider, "editor", "article.read", "article.update")

	if err := provider.RevokePermissions(ctx, "editor", "article.update"); err != nil {
		t.Fatalf("RevokePermissions failed: %v", err)
	}
	if got := mustRolePermissions(t, provider, "editor"); !reflect.DeepEqual(got, []authorization.Action{"article.read"}) {
		t.Errorf("RolePermissions = %v, want [article.read]", got)
	}

	// Revoking a permission that is not granted is a no-op, and the permission
	// itself stays registered.
	if err := provider.RevokePermissions(ctx, "editor", "article.update"); err != nil {
		t.Errorf("RevokePermissions of a non granted permission: want no error, got %v", err)
	}
	want := []authorization.Action{"article.read", "article.update"}
	if got := mustPermissions(t, provider); !reflect.DeepEqual(got, want) {
		t.Errorf("Permissions = %v, want %v", got, want)
	}
}

func TestRevokePermissionsUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.RevokePermissions(context.Background(), "ghost", "article.read"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestUnbindRoles(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustCreateRole(t, provider, "reviewer")
	mustBind(t, provider, "subject-1", "tenant-1", "editor", "reviewer")

	if err := provider.UnbindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatalf("UnbindRoles failed: %v", err)
	}
	want := []authorization.Binding{{SubjectID: "subject-1", Scope: "tenant-1", Role: "reviewer"}}
	if got := mustBindings(t, provider, "subject-1"); !reflect.DeepEqual(got, want) {
		t.Errorf("Bindings = %v, want %v", got, want)
	}

	// Unbinding a role that is not bound, or a scope with no binding at all, is
	// a no-op.
	if err := provider.UnbindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Errorf("UnbindRoles of a non bound role: want no error, got %v", err)
	}
	if err := provider.UnbindRoles(ctx, "subject-2", "tenant-9", "editor"); err != nil {
		t.Errorf("UnbindRoles on an unbound subject: want no error, got %v", err)
	}
	if got := mustBindings(t, provider, "subject-1"); !reflect.DeepEqual(got, want) {
		t.Errorf("Bindings = %v, want %v", got, want)
	}

	// The role itself survives being unbound.
	if got := mustRoles(t, provider); !reflect.DeepEqual(got, []string{"editor", "reviewer"}) {
		t.Errorf("Roles = %v, want [editor reviewer]", got)
	}
}

func TestUnbindRolesUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.UnbindRoles(context.Background(), "subject-1", "tenant-1", "ghost"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestUnbindRolesIsScopeIsolated(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustBind(t, provider, "subject-1", "tenant-1", "editor")
	mustBind(t, provider, "subject-1", "tenant-2", "editor")

	if err := provider.UnbindRoles(ctx, "subject-1", "tenant-1", "editor"); err != nil {
		t.Fatalf("UnbindRoles failed: %v", err)
	}
	want := []authorization.Binding{{SubjectID: "subject-1", Scope: "tenant-2", Role: "editor"}}
	if got := mustBindings(t, provider, "subject-1"); !reflect.DeepEqual(got, want) {
		t.Errorf("Bindings = %v, want %v: scopes must be independent", got, want)
	}
}

func TestSyncPermissionsIsAdditive(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	if err := provider.SyncPermissions(ctx, []authorization.Action{"article.read", "article.update"}); err != nil {
		t.Fatalf("SyncPermissions failed: %v", err)
	}
	// A second, disjoint synchronization never removes what is already known.
	if err := provider.SyncPermissions(ctx, []authorization.Action{"article.update", "article.delete"}); err != nil {
		t.Fatalf("SyncPermissions failed: %v", err)
	}
	if err := provider.SyncPermissions(ctx, nil); err != nil {
		t.Fatalf("SyncPermissions with no permission failed: %v", err)
	}

	want := []authorization.Action{"article.delete", "article.read", "article.update"}
	if got := mustPermissions(t, provider); !reflect.DeepEqual(got, want) {
		t.Errorf("Permissions = %v, want %v", got, want)
	}
	// Synchronizing a permission creates no role and grants nothing.
	if got := mustRoles(t, provider); len(got) != 0 {
		t.Errorf("Roles = %v, want none", got)
	}
}

func TestPermissionsUnionsSyncedAndGranted(t *testing.T) {
	provider := NewProvider()
	mustSync(t, provider, "article.read", "article.delete")
	mustCreateRole(t, provider, "editor")
	// An unknown permission is registered on the fly by GrantPermissions.
	mustGrant(t, provider, "editor", "article.update", "article.read")

	want := []authorization.Action{"article.delete", "article.read", "article.update"}
	if got := mustPermissions(t, provider); !reflect.DeepEqual(got, want) {
		t.Errorf("Permissions = %v, want %v", got, want)
	}
}

func TestRolesAndRolePermissionsAreSorted(t *testing.T) {
	provider := NewProvider()
	mustCreateRole(t, provider, "reviewer")
	mustCreateRole(t, provider, "admin")
	mustCreateRole(t, provider, "editor")
	mustGrant(t, provider, "editor", "article.update", "article.read", "article.delete")

	if got := mustRoles(t, provider); !reflect.DeepEqual(got, []string{"admin", "editor", "reviewer"}) {
		t.Errorf("Roles = %v, want [admin editor reviewer]", got)
	}
	want := []authorization.Action{"article.delete", "article.read", "article.update"}
	if got := mustRolePermissions(t, provider, "editor"); !reflect.DeepEqual(got, want) {
		t.Errorf("RolePermissions = %v, want %v", got, want)
	}
	if got := mustRolePermissions(t, provider, "admin"); len(got) != 0 {
		t.Errorf("RolePermissions = %v, want none for a role without permission", got)
	}
}

func TestRolePermissionsUnknownRole(t *testing.T) {
	provider := NewProvider()

	if _, err := provider.RolePermissions(context.Background(), "ghost"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestBindingsAcrossScopesAreSorted(t *testing.T) {
	provider := NewProvider()
	mustCreateRole(t, provider, "editor")
	mustCreateRole(t, provider, "admin")
	mustBind(t, provider, "subject-1", "tenant-2", "editor")
	mustBind(t, provider, "subject-1", "tenant-1", "editor", "admin")
	mustBind(t, provider, "subject-1", authorization.ScopeGlobal, "admin")
	mustBind(t, provider, "subject-2", "tenant-1", "editor")

	want := []authorization.Binding{
		{SubjectID: "subject-1", Scope: authorization.ScopeGlobal, Role: "admin"},
		{SubjectID: "subject-1", Scope: "tenant-1", Role: "admin"},
		{SubjectID: "subject-1", Scope: "tenant-1", Role: "editor"},
		{SubjectID: "subject-1", Scope: "tenant-2", Role: "editor"},
	}
	if got := mustBindings(t, provider, "subject-1"); !reflect.DeepEqual(got, want) {
		t.Errorf("Bindings = %v, want %v", got, want)
	}
	if got := mustBindings(t, provider, "ghost-subject"); len(got) != 0 {
		t.Errorf("Bindings = %v, want none for an unknown subject", got)
	}
}

func TestRoleBindingsAreSorted(t *testing.T) {
	provider := NewProvider()
	mustCreateRole(t, provider, "editor")
	mustCreateRole(t, provider, "admin")
	mustBind(t, provider, "subject-2", "tenant-1", "editor")
	mustBind(t, provider, "subject-1", "tenant-2", "editor")
	mustBind(t, provider, "subject-1", "tenant-1", "editor", "admin")

	want := []authorization.Binding{
		{SubjectID: "subject-1", Scope: "tenant-1", Role: "editor"},
		{SubjectID: "subject-1", Scope: "tenant-2", Role: "editor"},
		{SubjectID: "subject-2", Scope: "tenant-1", Role: "editor"},
	}
	if got := mustRoleBindings(t, provider, "editor"); !reflect.DeepEqual(got, want) {
		t.Errorf("RoleBindings = %v, want %v", got, want)
	}
	wantAdmin := []authorization.Binding{{SubjectID: "subject-1", Scope: "tenant-1", Role: "admin"}}
	if got := mustRoleBindings(t, provider, "admin"); !reflect.DeepEqual(got, wantAdmin) {
		t.Errorf("RoleBindings = %v, want %v", got, wantAdmin)
	}
}

func TestRoleBindingsUnknownRole(t *testing.T) {
	provider := NewProvider()

	if _, err := provider.RoleBindings(context.Background(), "ghost"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestResolve(t *testing.T) {
	provider := NewProvider()
	mustCreateRole(t, provider, "editor")
	mustGrant(t, provider, "editor", "article.read", "article.update")
	mustCreateRole(t, provider, "reviewer")
	mustGrant(t, provider, "reviewer", "article.read", "article.review")
	mustBind(t, provider, "subject-1", "tenant-1", "editor", "reviewer")

	subject := authorization.Subject{ID: "subject-1"}
	grants, err := provider.Resolve(context.Background(), subject, "tenant-1")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if !grants.HasRole("editor") || !grants.HasRole("reviewer") {
		t.Errorf("roles = %v, want editor and reviewer", grants.Roles)
	}
	for _, perm := range []authorization.Action{"article.read", "article.update", "article.review"} {
		if !grants.HasPermission(perm) {
			t.Errorf("missing permission %q", perm)
		}
	}
	if len(grants.Permissions) != 3 {
		t.Errorf("permissions = %v, want 3 deduplicated entries", grants.Permissions)
	}

	// Exact scope match only: no grants outside the bound scope.
	grants, err = provider.Resolve(context.Background(), subject, "tenant-2")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if len(grants.Roles) != 0 || len(grants.Permissions) != 0 {
		t.Errorf("grants = %+v, want none for an unbound scope", grants)
	}
}

func TestResolveWithEngineUnion(t *testing.T) {
	provider := NewProvider()
	mustCreateRole(t, provider, "global-admin")
	mustGrant(t, provider, "global-admin", "article.delete")
	mustBind(t, provider, "subject-1", authorization.ScopeGlobal, "global-admin")

	engine := authorization.NewEngine(authorization.WithProvider(provider))
	decision := engine.Decide(context.Background(), authorization.Request{
		Subject: authorization.Subject{ID: "subject-1"},
		Action:  "article.delete",
		Scope:   "tenant-1",
	})
	if !decision.Allowed {
		t.Errorf("Decision = %+v, want allowed via ScopeGlobal binding", decision)
	}
}

func TestConcurrentAccess(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()
	mustCreateRole(t, provider, "editor")
	mustCreateRole(t, provider, "reviewer")
	mustGrant(t, provider, "editor", "article.read")

	var wg sync.WaitGroup
	for i := range 16 {
		subjectID := fmt.Sprintf("subject-%d", i)
		role := fmt.Sprintf("role-%d", i)

		// Writers: bindings, role lifecycle, permission synchronization.
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := provider.BindRoles(ctx, subjectID, authorization.ScopeGlobal, "editor", "reviewer"); err != nil {
				t.Errorf("BindRoles failed: %v", err)
			}
			if err := provider.UnbindRoles(ctx, subjectID, authorization.ScopeGlobal, "reviewer"); err != nil {
				t.Errorf("UnbindRoles failed: %v", err)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := provider.CreateRole(ctx, role); err != nil {
				t.Errorf("CreateRole failed: %v", err)
			}
			if err := provider.GrantPermissions(ctx, role, authorization.Action(role+".read")); err != nil {
				t.Errorf("GrantPermissions failed: %v", err)
			}
			if err := provider.RevokePermissions(ctx, role, authorization.Action(role+".read")); err != nil {
				t.Errorf("RevokePermissions failed: %v", err)
			}
			if err := provider.DeleteRole(ctx, role); err != nil {
				t.Errorf("DeleteRole failed: %v", err)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := provider.SyncPermissions(ctx, []authorization.Action{authorization.Action(role + ".sync")}); err != nil {
				t.Errorf("SyncPermissions failed: %v", err)
			}
		}()

		// Readers: every GrantAdmin read plus Resolve.
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := provider.Resolve(ctx, authorization.Subject{ID: subjectID}, authorization.ScopeGlobal); err != nil {
				t.Errorf("Resolve failed: %v", err)
			}
			if _, err := provider.Roles(ctx); err != nil {
				t.Errorf("Roles failed: %v", err)
			}
			if _, err := provider.Permissions(ctx); err != nil {
				t.Errorf("Permissions failed: %v", err)
			}
			if _, err := provider.RolePermissions(ctx, "editor"); err != nil {
				t.Errorf("RolePermissions failed: %v", err)
			}
			if _, err := provider.Bindings(ctx, subjectID); err != nil {
				t.Errorf("Bindings failed: %v", err)
			}
			if _, err := provider.RoleBindings(ctx, "editor"); err != nil {
				t.Errorf("RoleBindings failed: %v", err)
			}
		}()
	}
	wg.Wait()

	for i := range 16 {
		bindings, err := provider.Bindings(ctx, fmt.Sprintf("subject-%d", i))
		if err != nil {
			t.Fatalf("Bindings failed: %v", err)
		}
		want := []authorization.Binding{{
			SubjectID: fmt.Sprintf("subject-%d", i),
			Scope:     authorization.ScopeGlobal,
			Role:      "editor",
		}}
		if !reflect.DeepEqual(bindings, want) {
			t.Errorf("Bindings = %v, want %v", bindings, want)
		}
	}
}

func mustCreateRole(t *testing.T, provider *Provider, role string) {
	t.Helper()
	if err := provider.CreateRole(context.Background(), role); err != nil {
		t.Fatalf("CreateRole(%q) failed: %v", role, err)
	}
}

func mustGrant(t *testing.T, provider *Provider, role string, actions ...authorization.Action) {
	t.Helper()
	if err := provider.GrantPermissions(context.Background(), role, actions...); err != nil {
		t.Fatalf("GrantPermissions(%q) failed: %v", role, err)
	}
}

func mustBind(t *testing.T, provider *Provider, subjectID string, scope authorization.Scope, roles ...string) {
	t.Helper()
	if err := provider.BindRoles(context.Background(), subjectID, scope, roles...); err != nil {
		t.Fatalf("BindRoles(%q) failed: %v", subjectID, err)
	}
}

func mustSync(t *testing.T, provider *Provider, permissions ...authorization.Action) {
	t.Helper()
	if err := provider.SyncPermissions(context.Background(), permissions); err != nil {
		t.Fatalf("SyncPermissions failed: %v", err)
	}
}

func mustRoles(t *testing.T, provider *Provider) []string {
	t.Helper()
	roles, err := provider.Roles(context.Background())
	if err != nil {
		t.Fatalf("Roles failed: %v", err)
	}

	return roles
}

func mustPermissions(t *testing.T, provider *Provider) []authorization.Action {
	t.Helper()
	permissions, err := provider.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions failed: %v", err)
	}

	return permissions
}

func mustRolePermissions(t *testing.T, provider *Provider, role string) []authorization.Action {
	t.Helper()
	permissions, err := provider.RolePermissions(context.Background(), role)
	if err != nil {
		t.Fatalf("RolePermissions(%q) failed: %v", role, err)
	}

	return permissions
}

func mustBindings(t *testing.T, provider *Provider, subjectID string) []authorization.Binding {
	t.Helper()
	bindings, err := provider.Bindings(context.Background(), subjectID)
	if err != nil {
		t.Fatalf("Bindings(%q) failed: %v", subjectID, err)
	}

	return bindings
}

func mustRoleBindings(t *testing.T, provider *Provider, role string) []authorization.Binding {
	t.Helper()
	bindings, err := provider.RoleBindings(context.Background(), role)
	if err != nil {
		t.Fatalf("RoleBindings(%q) failed: %v", role, err)
	}

	return bindings
}
