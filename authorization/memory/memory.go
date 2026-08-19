// Package memory provides an in-memory GrantProvider for tests and small
// applications. The provider is a complete authorization.GrantAdmin: roles,
// permissions and bindings can be administered at runtime — typically from an
// admin web interface — and every read is offered so such an interface can be
// built without reaching into the provider's internals.
//
// Mutations follow the same semantics as the SQL-backed gormstore: they are
// idempotent, permissions are auto-registered on the fly by GrantPermissions,
// and permissions are never deleted (removing a role only drops its links).
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/goyourt/yogourt/authorization"
)

type bindingKey struct {
	subjectID string
	scope     authorization.Scope
}

// Provider is a thread-safe in-memory implementation of
// authorization.GrantProvider, authorization.GrantAdmin and
// authorization.PermissionSyncer.
type Provider struct {
	mu sync.RWMutex
	// permissions holds every permission known to the provider, whether it was
	// registered by SyncPermissions or seen for the first time through
	// GrantPermissions. It is never pruned, mirroring the additive-only
	// synchronization contract (AUTHZ-512).
	permissions     map[authorization.Action]struct{}
	rolePermissions map[string]map[authorization.Action]struct{}
	bindings        map[bindingKey][]string
}

var (
	_ authorization.GrantProvider    = (*Provider)(nil)
	_ authorization.GrantAdmin       = (*Provider)(nil)
	_ authorization.PermissionSyncer = (*Provider)(nil)
)

var (
	_ authorization.GrantProvider    = (*Provider)(nil)
	_ authorization.GrantAdmin       = (*Provider)(nil)
	_ authorization.PermissionSyncer = (*Provider)(nil)
)

// NewProvider creates an empty in-memory provider.
func NewProvider() *Provider {
	return &Provider{
		permissions:     make(map[authorization.Action]struct{}),
		rolePermissions: make(map[string]map[authorization.Action]struct{}),
		bindings:        make(map[bindingKey][]string),
	}
}

// CreateRole registers a role. Creating an already existing role is a no-op
// that keeps the permissions already granted to it: the GrantAdmin contract
// asks for idempotent mutations, so that an admin interface or a boot-time
// bootstrap can be replayed. Earlier versions of this provider returned an
// error for an existing role.
func (p *Provider) CreateRole(_ context.Context, role string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.rolePermissions[role]; !exists {
		p.rolePermissions[role] = make(map[authorization.Action]struct{})
	}

	return nil
}

// DeleteRole removes a role, the permissions it grants and every binding
// holding it. The permissions themselves are never deleted, so an admin
// interface keeps offering them. Deleting an unknown role is a no-op.
func (p *Provider) DeleteRole(_ context.Context, role string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.rolePermissions, role)
	for key, bound := range p.bindings {
		remaining := remove(bound, role)
		if len(remaining) == len(bound) {
			continue
		}
		if len(remaining) == 0 {
			delete(p.bindings, key)

			continue
		}
		p.bindings[key] = remaining
	}

	return nil
}

// GrantPermissions adds permissions to an existing role. Unknown permissions
// are registered on the fly, so the application never has to declare a
// permission by hand; an unknown role is an error. Granting an already granted
// permission is a no-op.
func (p *Provider) GrantPermissions(_ context.Context, role string, actions ...authorization.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	perms, exists := p.rolePermissions[role]
	if !exists {
		return fmt.Errorf("memory: unknown role %q", role)
	}
	for _, action := range actions {
		perms[action] = struct{}{}
		p.permissions[action] = struct{}{}
	}

	return nil
}

// RevokePermissions removes permissions from an existing role. The permissions
// themselves stay registered. Revoking a permission that is not granted is a
// no-op; an unknown role is an error.
func (p *Provider) RevokePermissions(_ context.Context, role string, actions ...authorization.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	perms, exists := p.rolePermissions[role]
	if !exists {
		return fmt.Errorf("memory: unknown role %q", role)
	}
	for _, action := range actions {
		delete(perms, action)
	}

	return nil
}

// BindRoles binds roles to a subject within a scope. subjectID is the stable
// identity of the subject, never an application SQL id. Binding an already
// bound role is a no-op; an unknown role is an error.
func (p *Provider) BindRoles(_ context.Context, subjectID string, scope authorization.Scope, roles ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkKnownRoles(roles); err != nil {
		return err
	}

	key := bindingKey{subjectID: subjectID, scope: scope}
	bound := p.bindings[key]
	for _, role := range roles {
		if !contains(bound, role) {
			bound = append(bound, role)
		}
	}
	p.bindings[key] = bound

	return nil
}

// UnbindRoles removes role bindings from a subject within a scope. Unbinding a
// role that is not bound is a no-op; an unknown role is an error. Roles and
// permissions are never touched.
func (p *Provider) UnbindRoles(_ context.Context, subjectID string, scope authorization.Scope, roles ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.checkKnownRoles(roles); err != nil {
		return err
	}

	key := bindingKey{subjectID: subjectID, scope: scope}
	bound, bindingExists := p.bindings[key]
	if !bindingExists {
		return nil
	}
	for _, role := range roles {
		bound = remove(bound, role)
	}
	if len(bound) == 0 {
		delete(p.bindings, key)

		return nil
	}
	p.bindings[key] = bound

	return nil
}

// SyncPermissions registers the given permissions if they are not known yet.
// It is strictly additive and never deletes anything: permissions that are no
// longer declared stay in place (AUTHZ-512). The framework calls it at boot
// with the permissions the routes declare, which is what makes Permissions
// able to offer a closed list to an admin interface.
func (p *Provider) SyncPermissions(_ context.Context, permissions []authorization.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, permission := range permissions {
		p.permissions[permission] = struct{}{}
	}

	return nil
}

// Roles lists every role, sorted by name.
func (p *Provider) Roles(_ context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	roles := make([]string, 0, len(p.rolePermissions))
	for role := range p.rolePermissions {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	return roles, nil
}

// Permissions lists every permission known to the provider, sorted by name:
// those registered by SyncPermissions as well as those seen through
// GrantPermissions.
func (p *Provider) Permissions(_ context.Context) ([]authorization.Action, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return sortedActions(p.permissions), nil
}

// RolePermissions lists the permissions of one role, sorted by name. An
// unknown role is an error, as it is for every other role-scoped operation.
func (p *Provider) RolePermissions(_ context.Context, role string) ([]authorization.Action, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	perms, exists := p.rolePermissions[role]
	if !exists {
		return nil, fmt.Errorf("memory: unknown role %q", role)
	}

	return sortedActions(perms), nil
}

// Bindings lists every binding of one subject, across all scopes, sorted by
// scope then role.
func (p *Provider) Bindings(_ context.Context, subjectID string) ([]authorization.Binding, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var bindings []authorization.Binding
	for key, roles := range p.bindings {
		if key.subjectID != subjectID {
			continue
		}
		for _, role := range roles {
			bindings = append(bindings, authorization.Binding{
				SubjectID: key.subjectID,
				Scope:     key.scope,
				Role:      role,
			})
		}
	}
	sortBindings(bindings)

	return bindings, nil
}

// RoleBindings lists every subject holding one role, with its scope, sorted by
// subject then scope. An unknown role is an error.
func (p *Provider) RoleBindings(_ context.Context, role string) ([]authorization.Binding, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if _, exists := p.rolePermissions[role]; !exists {
		return nil, fmt.Errorf("memory: unknown role %q", role)
	}

	var bindings []authorization.Binding
	for key, roles := range p.bindings {
		if !contains(roles, role) {
			continue
		}
		bindings = append(bindings, authorization.Binding{
			SubjectID: key.subjectID,
			Scope:     key.scope,
			Role:      role,
		})
	}
	sortBindings(bindings)

	return bindings, nil
}

// Resolve returns the grants bound to the subject for the exact given scope.
// The union with ScopeGlobal is the engine's responsibility, not the
// provider's.
func (p *Provider) Resolve(_ context.Context, subject authorization.Subject, scope authorization.Scope) (authorization.Grants, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	grants := authorization.Grants{}
	for _, role := range p.bindings[bindingKey{subjectID: subject.ID, scope: scope}] {
		grants.Roles = append(grants.Roles, role)
		for permission := range p.rolePermissions[role] {
			if !grants.HasPermission(permission) {
				grants.Permissions = append(grants.Permissions, permission)
			}
		}
	}

	return grants, nil
}

// checkKnownRoles reports the first unknown role, so that a binding operation
// fails as a whole instead of applying half of it. The caller must hold the
// lock.
func (p *Provider) checkKnownRoles(roles []string) error {
	for _, role := range roles {
		if _, exists := p.rolePermissions[role]; !exists {
			return fmt.Errorf("memory: unknown role %q", role)
		}
	}

	return nil
}

// sortBindings orders bindings deterministically: subject, then scope, then
// role. Callers list either one subject or one role, so the leading fields are
// constant and the meaningful order is the remaining one.
func sortBindings(bindings []authorization.Binding) {
	sort.Slice(bindings, func(i, j int) bool {
		a, b := bindings[i], bindings[j]
		if a.SubjectID != b.SubjectID {
			return a.SubjectID < b.SubjectID
		}
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}

		return a.Role < b.Role
	})
}

func sortedActions(actions map[authorization.Action]struct{}) []authorization.Action {
	sorted := make([]authorization.Action, 0, len(actions))
	for action := range actions {
		sorted = append(sorted, action)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	return sorted
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}

// remove returns values without value, preserving the order of the rest. It
// leaves the input untouched when the value is absent.
func remove(values []string, value string) []string {
	if !contains(values, value) {
		return values
	}

	remaining := make([]string, 0, len(values)-1)
	for _, v := range values {
		if v != value {
			remaining = append(remaining, v)
		}
	}

	return remaining
}
