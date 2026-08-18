package gormstore_test

import (
	"context"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/gormstore"
)

// openDB connects to the PostgreSQL instance designated by YOGOURT_TEST_DSN,
// skipping the test when the variable is not set.
func openDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv("YOGOURT_TEST_DSN")
	if dsn == "" {
		t.Skip("YOGOURT_TEST_DSN not set; skipping PostgreSQL integration tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	return db
}

// setup migrates the schema and starts each test from empty tables.
func setup(t *testing.T) (*gorm.DB, *gormstore.Store) {
	t.Helper()

	db := openDB(t)
	if err := gormstore.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := db.Exec(`TRUNCATE authz_role_bindings, authz_role_permissions, authz_roles, authz_permissions RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}

	return db, gormstore.New(db)
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()

	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM ` + table).Scan(&n).Error; err != nil {
		t.Fatalf("count rows of %s: %v", table, err)
	}

	return n
}

func resolve(t *testing.T, store *gormstore.Store, subjectID string, scope authorization.Scope) authorization.Grants {
	t.Helper()

	grants, err := store.Resolve(context.Background(), authorization.Subject{ID: subjectID}, scope)
	if err != nil {
		t.Fatalf("resolve %q in scope %q: %v", subjectID, scope, err)
	}

	return grants
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	if err := gormstore.Migrate(ctx, db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := gormstore.Migrate(ctx, db); err != nil {
		t.Fatalf("second migrate should be a no-op, got: %v", err)
	}
}

func TestFullCycle(t *testing.T) {
	_, store := setup(t)
	ctx := context.Background()

	for _, role := range []string{"editor", "viewer", "global-admin", "empty"} {
		if err := store.CreateRole(ctx, role); err != nil {
			t.Fatalf("create role %q: %v", role, err)
		}
	}

	// Permissions are auto-registered by GrantPermissions: no SyncPermissions
	// nor manual insertion beforehand.
	if err := store.GrantPermissions(ctx, "editor", "article.read", "article.update"); err != nil {
		t.Fatalf("grant to editor: %v", err)
	}
	if err := store.GrantPermissions(ctx, "viewer", "article.read"); err != nil {
		t.Fatalf("grant to viewer: %v", err)
	}
	if err := store.GrantPermissions(ctx, "global-admin", "system.admin"); err != nil {
		t.Fatalf("grant to global-admin: %v", err)
	}

	if err := store.BindRoles(ctx, "user-1", "tenant-a", "editor", "viewer", "empty"); err != nil {
		t.Fatalf("bind in tenant-a: %v", err)
	}
	if err := store.BindRoles(ctx, "user-1", authorization.ScopeGlobal, "global-admin"); err != nil {
		t.Fatalf("bind in global scope: %v", err)
	}
	if err := store.BindRoles(ctx, "user-2", "tenant-b", "viewer"); err != nil {
		t.Fatalf("bind user-2: %v", err)
	}

	// Exact-scope resolution, with role and permission deduplication
	// ("article.read" is granted by both editor and viewer).
	grants := resolve(t, store, "user-1", "tenant-a")
	wantRoles := []string{"editor", "empty", "viewer"}
	if len(grants.Roles) != len(wantRoles) {
		t.Fatalf("tenant-a roles = %v, want %v", grants.Roles, wantRoles)
	}
	for _, role := range wantRoles {
		if !grants.HasRole(role) {
			t.Errorf("tenant-a grants miss role %q: %v", role, grants.Roles)
		}
	}
	wantPermissions := []authorization.Action{"article.read", "article.update"}
	if len(grants.Permissions) != len(wantPermissions) {
		t.Fatalf("tenant-a permissions = %v, want %v (deduplicated)", grants.Permissions, wantPermissions)
	}
	for _, permission := range wantPermissions {
		if !grants.HasPermission(permission) {
			t.Errorf("tenant-a grants miss permission %q: %v", permission, grants.Permissions)
		}
	}

	// No leak across scopes: the provider resolves the exact scope only, the
	// union with ScopeGlobal is the engine's job.
	if grants.HasRole("global-admin") || grants.HasPermission("system.admin") {
		t.Errorf("global grants leaked into tenant-a: %+v", grants)
	}
	globalGrants := resolve(t, store, "user-1", authorization.ScopeGlobal)
	if !globalGrants.HasRole("global-admin") || !globalGrants.HasPermission("system.admin") {
		t.Errorf("global scope grants = %+v, want global-admin/system.admin", globalGrants)
	}
	if globalGrants.HasRole("editor") || globalGrants.HasPermission("article.update") {
		t.Errorf("tenant-a grants leaked into global scope: %+v", globalGrants)
	}

	// No leak across subjects or into unknown scopes.
	if g := resolve(t, store, "user-1", "tenant-b"); len(g.Roles) != 0 || len(g.Permissions) != 0 {
		t.Errorf("user-1 has grants in tenant-b: %+v", g)
	}
	if g := resolve(t, store, "user-2", "tenant-a"); len(g.Roles) != 0 || len(g.Permissions) != 0 {
		t.Errorf("user-2 has grants in tenant-a: %+v", g)
	}
	if g := resolve(t, store, "nobody", "tenant-a"); len(g.Roles) != 0 || len(g.Permissions) != 0 {
		t.Errorf("unknown subject has grants: %+v", g)
	}
}

func TestMutationsAreIdempotent(t *testing.T) {
	db, store := setup(t)
	ctx := context.Background()

	for range 2 {
		if err := store.CreateRole(ctx, "editor"); err != nil {
			t.Fatalf("create role: %v", err)
		}
		if err := store.GrantPermissions(ctx, "editor", "article.read", "article.read"); err != nil {
			t.Fatalf("grant: %v", err)
		}
		if err := store.BindRoles(ctx, "user-1", "tenant-a", "editor"); err != nil {
			t.Fatalf("bind: %v", err)
		}
	}

	if n := countRows(t, db, "authz_roles"); n != 1 {
		t.Errorf("authz_roles count = %d, want 1", n)
	}
	if n := countRows(t, db, "authz_permissions"); n != 1 {
		t.Errorf("authz_permissions count = %d, want 1", n)
	}
	if n := countRows(t, db, "authz_role_permissions"); n != 1 {
		t.Errorf("authz_role_permissions count = %d, want 1", n)
	}
	if n := countRows(t, db, "authz_role_bindings"); n != 1 {
		t.Errorf("authz_role_bindings count = %d, want 1", n)
	}

	// Idempotent removals: repeating them stays a no-op. DeleteRole comes
	// last, as revoking or unbinding an unknown role is an error by design.
	for range 2 {
		if err := store.RevokePermissions(ctx, "editor", "article.read", "never.granted"); err != nil {
			t.Fatalf("revoke: %v", err)
		}
	}
	for range 2 {
		if err := store.UnbindRoles(ctx, "user-1", "tenant-a", "editor"); err != nil {
			t.Fatalf("unbind: %v", err)
		}
	}
	for range 2 {
		if err := store.DeleteRole(ctx, "editor"); err != nil {
			t.Fatalf("delete role: %v", err)
		}
	}
}

func TestRevocationAndDeletion(t *testing.T) {
	db, store := setup(t)
	ctx := context.Background()

	if err := store.CreateRole(ctx, "editor"); err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := store.GrantPermissions(ctx, "editor", "article.read", "article.update"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := store.BindRoles(ctx, "user-1", "tenant-a", "editor"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	if err := store.RevokePermissions(ctx, "editor", "article.update"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	grants := resolve(t, store, "user-1", "tenant-a")
	if grants.HasPermission("article.update") {
		t.Errorf("article.update still granted after revocation: %+v", grants)
	}
	if !grants.HasPermission("article.read") {
		t.Errorf("article.read lost by revocation of another permission: %+v", grants)
	}
	// Revocation never deletes the permission itself.
	if n := countRows(t, db, "authz_permissions"); n != 2 {
		t.Errorf("authz_permissions count = %d after revoke, want 2", n)
	}

	if err := store.UnbindRoles(ctx, "user-1", "tenant-a", "editor"); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if g := resolve(t, store, "user-1", "tenant-a"); len(g.Roles) != 0 || len(g.Permissions) != 0 {
		t.Errorf("grants remain after unbind: %+v", g)
	}

	// DeleteRole cleans its links but never the permissions.
	if err := store.BindRoles(ctx, "user-1", "tenant-a", "editor"); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	if err := store.DeleteRole(ctx, "editor"); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	if n := countRows(t, db, "authz_roles"); n != 0 {
		t.Errorf("authz_roles count = %d after delete, want 0", n)
	}
	if n := countRows(t, db, "authz_role_permissions"); n != 0 {
		t.Errorf("authz_role_permissions count = %d after delete, want 0", n)
	}
	if n := countRows(t, db, "authz_role_bindings"); n != 0 {
		t.Errorf("authz_role_bindings count = %d after delete, want 0", n)
	}
	if n := countRows(t, db, "authz_permissions"); n != 2 {
		t.Errorf("authz_permissions count = %d after delete, want 2 (never deleted)", n)
	}
}

func TestSyncPermissionsIsAdditive(t *testing.T) {
	db, store := setup(t)
	ctx := context.Background()

	if err := store.SyncPermissions(ctx, []authorization.Action{"article.read", "article.update"}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if n := countRows(t, db, "authz_permissions"); n != 2 {
		t.Fatalf("authz_permissions count = %d after first sync, want 2", n)
	}

	// A second sync with an overlapping, smaller set adds the new permission
	// and never deletes the ones that are no longer declared.
	if err := store.SyncPermissions(ctx, []authorization.Action{"article.update", "article.delete"}); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if n := countRows(t, db, "authz_permissions"); n != 3 {
		t.Fatalf("authz_permissions count = %d after second sync, want 3", n)
	}

	var names []string
	if err := db.Raw(`SELECT name FROM authz_permissions ORDER BY name`).Scan(&names).Error; err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	want := []string{"article.delete", "article.read", "article.update"}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("permissions = %v, want %v", names, want)
		}
	}

	if err := store.SyncPermissions(ctx, nil); err != nil {
		t.Fatalf("empty sync: %v", err)
	}
}

func TestUnknownRoleErrors(t *testing.T) {
	_, store := setup(t)
	ctx := context.Background()

	if err := store.GrantPermissions(ctx, "ghost", "article.read"); err == nil {
		t.Error("GrantPermissions on unknown role: want error, got nil")
	}
	if err := store.RevokePermissions(ctx, "ghost", "article.read"); err == nil {
		t.Error("RevokePermissions on unknown role: want error, got nil")
	}
	if err := store.BindRoles(ctx, "user-1", "tenant-a", "ghost"); err == nil {
		t.Error("BindRoles on unknown role: want error, got nil")
	}
	if err := store.UnbindRoles(ctx, "user-1", "tenant-a", "ghost"); err == nil {
		t.Error("UnbindRoles on unknown role: want error, got nil")
	}
}

func TestSQLErrorsAreReturned(t *testing.T) {
	_, store := setup(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	if err := store.CreateRole(cancelled, "editor"); err == nil {
		t.Error("CreateRole with cancelled context: want error, got nil")
	}
	if err := store.DeleteRole(cancelled, "editor"); err == nil {
		t.Error("DeleteRole with cancelled context: want error, got nil")
	}
	if err := store.GrantPermissions(cancelled, "editor", "article.read"); err == nil {
		t.Error("GrantPermissions with cancelled context: want error, got nil")
	}
	if err := store.RevokePermissions(cancelled, "editor", "article.read"); err == nil {
		t.Error("RevokePermissions with cancelled context: want error, got nil")
	}
	if err := store.BindRoles(cancelled, "user-1", "tenant-a", "editor"); err == nil {
		t.Error("BindRoles with cancelled context: want error, got nil")
	}
	if err := store.UnbindRoles(cancelled, "user-1", "tenant-a", "editor"); err == nil {
		t.Error("UnbindRoles with cancelled context: want error, got nil")
	}
	if err := store.SyncPermissions(cancelled, []authorization.Action{"article.read"}); err == nil {
		t.Error("SyncPermissions with cancelled context: want error, got nil")
	}
	if _, err := store.Resolve(cancelled, authorization.Subject{ID: "user-1"}, "tenant-a"); err == nil {
		t.Error("Resolve with cancelled context: want error, got nil")
	}
	if err := gormstore.Migrate(cancelled, openDB(t)); err == nil {
		t.Error("Migrate with cancelled context: want error, got nil")
	}
}
