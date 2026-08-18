package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/goyourt/yogourt/authorization"
)

func TestCreateRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.CreateRole("editor"); err != nil {
		t.Fatalf("CreateRole failed: %v", err)
	}
	if err := provider.CreateRole("editor"); err == nil {
		t.Error("expected an error when creating an existing role")
	}
}

func TestGrantPermissionsUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.GrantPermissions("ghost", "article.read"); err == nil {
		t.Error("expected an error for an unknown role")
	}
}

func TestBindRolesUnknownRole(t *testing.T) {
	provider := NewProvider()

	if err := provider.BindRoles("subject-1", authorization.ScopeGlobal, "ghost"); err == nil {
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
	mustCreateRole(t, provider, "editor")
	mustGrant(t, provider, "editor", "article.read")

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			subjectID := fmt.Sprintf("subject-%d", i)
			if err := provider.BindRoles(subjectID, authorization.ScopeGlobal, "editor"); err != nil {
				t.Errorf("BindRoles failed: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			subject := authorization.Subject{ID: fmt.Sprintf("subject-%d", i)}
			if _, err := provider.Resolve(context.Background(), subject, authorization.ScopeGlobal); err != nil {
				t.Errorf("Resolve failed: %v", err)
			}
		}()
	}
	wg.Wait()
}

func mustCreateRole(t *testing.T, provider *Provider, role string) {
	t.Helper()
	if err := provider.CreateRole(role); err != nil {
		t.Fatalf("CreateRole(%q) failed: %v", role, err)
	}
}

func mustGrant(t *testing.T, provider *Provider, role string, actions ...authorization.Action) {
	t.Helper()
	if err := provider.GrantPermissions(role, actions...); err != nil {
		t.Fatalf("GrantPermissions(%q) failed: %v", role, err)
	}
}

func mustBind(t *testing.T, provider *Provider, subjectID string, scope authorization.Scope, roles ...string) {
	t.Helper()
	if err := provider.BindRoles(subjectID, scope, roles...); err != nil {
		t.Fatalf("BindRoles(%q) failed: %v", subjectID, err)
	}
}
