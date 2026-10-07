// Package gormstore provides a PostgreSQL-backed implementation of
// authorization.GrantProvider and authorization.GrantAdmin built on GORM: the
// grants it resolves can also be administered at runtime, and each read is a
// single sorted query so an admin web interface never has to reach into the
// schema. It manages roles, permissions
// and role bindings in four dedicated tables (authz_permissions, authz_roles,
// authz_role_permissions, authz_role_bindings) created by explicit, versioned
// SQL migrations (see Migrate) — never by AutoMigrate.
//
// All mutations are transactional and idempotent, every SQL error is
// returned, and permissions are auto-registered on the fly: the application
// never has to insert a permission by hand. Like every GrantProvider, the
// store resolves the exact scope it is asked about; the union with
// ScopeGlobal is the engine's responsibility.
package gormstore

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	"github.com/goyourt/yogourt/authorization"
)

// Store is a PostgreSQL implementation of authorization.GrantProvider backed
// by GORM. Create it with New and apply the schema with Migrate before use.
type Store struct {
	db *gorm.DB
}

var _ authorization.GrantProvider = (*Store)(nil)
var _ authorization.GrantAdmin = (*Store)(nil)

// New creates a Store on top of an existing GORM connection. It does not
// touch the schema: call Migrate explicitly to create the tables.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

const upsertPermissionQuery = `INSERT INTO authz_permissions (name) VALUES (?) ON CONFLICT (name) DO NOTHING`

// CreateRole registers a role. Creating an already existing role is a no-op.
func (s *Store) CreateRole(ctx context.Context, role string) error {
	err := s.db.WithContext(ctx).
		Exec(`INSERT INTO authz_roles (name) VALUES (?) ON CONFLICT (name) DO NOTHING`, role).
		Error
	if err != nil {
		return fmt.Errorf("gormstore: create role %q: %w", role, err)
	}

	return nil
}

// DeleteRole removes a role. Its permission links and bindings are cleaned up
// by the ON DELETE CASCADE foreign keys; the permissions themselves are never
// touched. Deleting an unknown role is a no-op.
func (s *Store) DeleteRole(ctx context.Context, role string) error {
	err := s.db.WithContext(ctx).
		Exec(`DELETE FROM authz_roles WHERE name = ?`, role).
		Error
	if err != nil {
		return fmt.Errorf("gormstore: delete role %q: %w", role, err)
	}

	return nil
}

// GrantPermissions adds permissions to an existing role. Unknown permissions
// are auto-registered (additive upsert), so the application never inserts a
// permission manually; an unknown role is an error. Granting an already
// granted permission is a no-op.
func (s *Store) GrantPermissions(ctx context.Context, role string, actions ...authorization.Action) error {
	if len(actions) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roleID, err := roleIDByName(tx, role)
		if err != nil {
			return err
		}
		for _, action := range actions {
			if err := tx.Exec(upsertPermissionQuery, string(action)).Error; err != nil {
				return fmt.Errorf("gormstore: register permission %q: %w", action, err)
			}
			err := tx.Exec(
				`INSERT INTO authz_role_permissions (role_id, permission_id)
				 SELECT ?, id FROM authz_permissions WHERE name = ?
				 ON CONFLICT (role_id, permission_id) DO NOTHING`,
				roleID, string(action),
			).Error
			if err != nil {
				return fmt.Errorf("gormstore: grant permission %q to role %q: %w", action, role, err)
			}
		}

		return nil
	})
}

// RevokePermissions removes permissions from an existing role. The
// permissions themselves are never deleted. Revoking a permission that is not
// granted is a no-op; an unknown role is an error.
func (s *Store) RevokePermissions(ctx context.Context, role string, actions ...authorization.Action) error {
	if len(actions) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roleID, err := roleIDByName(tx, role)
		if err != nil {
			return err
		}
		err = tx.Exec(
			`DELETE FROM authz_role_permissions
			 WHERE role_id = ?
			   AND permission_id IN (SELECT id FROM authz_permissions WHERE name IN ?)`,
			roleID, actionNames(actions),
		).Error
		if err != nil {
			return fmt.Errorf("gormstore: revoke permissions from role %q: %w", role, err)
		}

		return nil
	})
}

// BindRoles binds roles to a subject within a scope. subjectID is the stable
// identity of the subject (UUID string), never an application SQL id. Binding
// an already bound role is a no-op; an unknown role is an error.
func (s *Store) BindRoles(ctx context.Context, subjectID string, scope authorization.Scope, roles ...string) error {
	if len(roles) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, role := range roles {
			roleID, err := roleIDByName(tx, role)
			if err != nil {
				return err
			}
			err = tx.Exec(
				`INSERT INTO authz_role_bindings (subject_id, scope, role_id)
				 VALUES (?, ?, ?)
				 ON CONFLICT (subject_id, scope, role_id) DO NOTHING`,
				subjectID, string(scope), roleID,
			).Error
			if err != nil {
				return fmt.Errorf("gormstore: bind role %q to subject %q: %w", role, subjectID, err)
			}
		}

		return nil
	})
}

// UnbindRoles removes role bindings from a subject within a scope. Unbinding
// a role that is not bound is a no-op; an unknown role is an error. Roles and
// permissions are never touched.
func (s *Store) UnbindRoles(ctx context.Context, subjectID string, scope authorization.Scope, roles ...string) error {
	if len(roles) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		roleIDs := make([]int64, 0, len(roles))
		for _, role := range roles {
			roleID, err := roleIDByName(tx, role)
			if err != nil {
				return err
			}
			roleIDs = append(roleIDs, roleID)
		}
		err := tx.Exec(
			`DELETE FROM authz_role_bindings
			 WHERE subject_id = ? AND scope = ? AND role_id IN ?`,
			subjectID, string(scope), roleIDs,
		).Error
		if err != nil {
			return fmt.Errorf("gormstore: unbind roles from subject %q: %w", subjectID, err)
		}

		return nil
	})
}

// Roles lists every role, sorted by name. An empty store returns no role and
// no error.
func (s *Store) Roles(ctx context.Context) ([]string, error) {
	var names []string
	err := s.db.WithContext(ctx).
		Raw(`SELECT name FROM authz_roles ORDER BY name`).
		Scan(&names).
		Error
	if err != nil {
		return nil, fmt.Errorf("gormstore: list roles: %w", err)
	}

	return names, nil
}

// Permissions lists every registered permission, sorted by name. Permissions
// are registered by SyncPermissions at boot and by GrantPermissions on the
// fly, so this is the closed list an admin interface offers to pick from.
func (s *Store) Permissions(ctx context.Context) ([]authorization.Action, error) {
	var names []string
	err := s.db.WithContext(ctx).
		Raw(`SELECT name FROM authz_permissions ORDER BY name`).
		Scan(&names).
		Error
	if err != nil {
		return nil, fmt.Errorf("gormstore: list permissions: %w", err)
	}

	return actions(names), nil
}

// rolePermissionsQuery lists the permissions of one role. The LEFT JOINs are
// what make an unknown role distinguishable from a role without permission: a
// known role always yields at least one row, whose permission is NULL when it
// grants nothing, while an unknown role yields none at all.
const rolePermissionsQuery = `
SELECT p.name AS permission
FROM authz_roles r
LEFT JOIN authz_role_permissions rp ON rp.role_id = r.id
LEFT JOIN authz_permissions p ON p.id = rp.permission_id
WHERE r.name = ?
ORDER BY p.name`

// RolePermissions lists the permissions of one role, sorted by name, in a
// single joined query. A role that grants nothing returns an empty list, while
// an unknown role is an error — consistent with GrantPermissions and
// RevokePermissions, which also reject a role that does not exist.
func (s *Store) RolePermissions(ctx context.Context, role string) ([]authorization.Action, error) {
	var rows []struct {
		Permission sql.NullString
	}
	err := s.db.WithContext(ctx).
		Raw(rolePermissionsQuery, role).
		Scan(&rows).
		Error
	if err != nil {
		return nil, fmt.Errorf("gormstore: list permissions of role %q: %w", role, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("gormstore: unknown role %q", role)
	}

	permissions := make([]authorization.Action, 0, len(rows))
	for _, row := range rows {
		if !row.Permission.Valid {
			continue
		}
		permissions = append(permissions, authorization.Action(row.Permission.String))
	}

	return permissions, nil
}

// bindingsQuery lists the bindings of one subject across every scope.
const bindingsQuery = `
SELECT b.subject_id, b.scope, r.name AS role
FROM authz_role_bindings b
JOIN authz_roles r ON r.id = b.role_id
WHERE b.subject_id = ?
ORDER BY b.scope, r.name`

// Bindings lists every binding of one subject, across all scopes, sorted by
// (scope, role) — the order an admin interface displays them in. A subject
// with no binding returns an empty list and no error: unlike a role, a subject
// is not a row of this store, so there is no such thing as an unknown one.
func (s *Store) Bindings(ctx context.Context, subjectID string) ([]authorization.Binding, error) {
	bindings, err := s.bindings(ctx, bindingsQuery, subjectID)
	if err != nil {
		return nil, fmt.Errorf("gormstore: list bindings of subject %q: %w", subjectID, err)
	}

	return bindings, nil
}

// roleBindingsQuery lists the subjects holding one role, in every scope.
const roleBindingsQuery = `
SELECT b.subject_id, b.scope, r.name AS role
FROM authz_role_bindings b
JOIN authz_roles r ON r.id = b.role_id
WHERE r.name = ?
ORDER BY b.subject_id, b.scope`

// RoleBindings lists every subject holding one role, with its scope, sorted by
// (subject, scope). An unknown role simply holds no binding, so it returns an
// empty list rather than an error: nothing is asserted about the role itself.
func (s *Store) RoleBindings(ctx context.Context, role string) ([]authorization.Binding, error) {
	bindings, err := s.bindings(ctx, roleBindingsQuery, role)
	if err != nil {
		return nil, fmt.Errorf("gormstore: list bindings of role %q: %w", role, err)
	}

	return bindings, nil
}

// bindings runs one of the binding queries and converts its rows. Scanning
// into an intermediate struct keeps the Scope conversion explicit instead of
// relying on GORM to assign a string column to a named string type.
func (s *Store) bindings(ctx context.Context, query string, arg any) ([]authorization.Binding, error) {
	var rows []struct {
		SubjectID string
		Scope     string
		Role      string
	}
	if err := s.db.WithContext(ctx).Raw(query, arg).Scan(&rows).Error; err != nil {
		return nil, err
	}

	bindings := make([]authorization.Binding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, authorization.Binding{
			SubjectID: row.SubjectID,
			Scope:     authorization.Scope(row.Scope),
			Role:      row.Role,
		})
	}

	return bindings, nil
}

// SyncPermissions registers the given permissions if they do not exist yet.
// It is strictly additive (ON CONFLICT DO NOTHING) and never deletes
// anything: permissions that are no longer declared stay in place. The
// framework calls it at boot with the permissions declared by the routes, so
// no permission ever has to be inserted manually.
func (s *Store) SyncPermissions(ctx context.Context, permissions []authorization.Action) error {
	if len(permissions) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, permission := range permissions {
			if err := tx.Exec(upsertPermissionQuery, string(permission)).Error; err != nil {
				return fmt.Errorf("gormstore: sync permission %q: %w", permission, err)
			}
		}

		return nil
	})
}

// resolveQuery walks bindings → roles → role_permissions → permissions in a
// single indexed query. The LEFT JOINs keep roles that have no permission.
const resolveQuery = `
SELECT r.name AS role, p.name AS permission
FROM authz_role_bindings b
JOIN authz_roles r ON r.id = b.role_id
LEFT JOIN authz_role_permissions rp ON rp.role_id = r.id
LEFT JOIN authz_permissions p ON p.id = rp.permission_id
WHERE b.subject_id = ? AND b.scope = ?
ORDER BY r.name, p.name`

// Resolve returns the grants bound to the subject for the exact given scope,
// in a single joined query. Roles and permissions are deduplicated. The union
// with ScopeGlobal is the engine's responsibility, not the provider's.
func (s *Store) Resolve(ctx context.Context, subject authorization.Subject, scope authorization.Scope) (authorization.Grants, error) {
	var rows []struct {
		Role       string
		Permission sql.NullString
	}
	err := s.db.WithContext(ctx).
		Raw(resolveQuery, subject.ID, string(scope)).
		Scan(&rows).
		Error
	if err != nil {
		return authorization.Grants{}, fmt.Errorf("gormstore: resolve grants for subject %q: %w", subject.ID, err)
	}

	grants := authorization.Grants{}
	seenRoles := make(map[string]struct{})
	seenPermissions := make(map[string]struct{})
	for _, row := range rows {
		if _, seen := seenRoles[row.Role]; !seen {
			seenRoles[row.Role] = struct{}{}
			grants.Roles = append(grants.Roles, row.Role)
		}
		if !row.Permission.Valid {
			continue
		}
		if _, seen := seenPermissions[row.Permission.String]; !seen {
			seenPermissions[row.Permission.String] = struct{}{}
			grants.Permissions = append(grants.Permissions, authorization.Action(row.Permission.String))
		}
	}

	return grants, nil
}

// roleIDByName returns the id of a role or an error when the role is unknown.
func roleIDByName(tx *gorm.DB, role string) (int64, error) {
	var id int64
	result := tx.Raw(`SELECT id FROM authz_roles WHERE name = ?`, role).Scan(&id)
	if result.Error != nil {
		return 0, fmt.Errorf("gormstore: look up role %q: %w", role, result.Error)
	}
	if result.RowsAffected == 0 {
		return 0, fmt.Errorf("gormstore: unknown role %q", role)
	}

	return id, nil
}

func actionNames(actions []authorization.Action) []string {
	names := make([]string, len(actions))
	for i, action := range actions {
		names[i] = string(action)
	}

	return names
}

func actions(names []string) []authorization.Action {
	converted := make([]authorization.Action, len(names))
	for i, name := range names {
		converted[i] = authorization.Action(name)
	}

	return converted
}
