// Package memory provides an in-memory GrantProvider for tests and small
// applications.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/goyourt/yogourt/authorization"
)

type bindingKey struct {
	subjectID string
	scope     authorization.Scope
}

// Provider is a thread-safe in-memory implementation of
// authorization.GrantProvider.
type Provider struct {
	mu              sync.RWMutex
	rolePermissions map[string]map[authorization.Action]struct{}
	bindings        map[bindingKey][]string
}

// NewProvider creates an empty in-memory provider.
func NewProvider() *Provider {
	return &Provider{
		rolePermissions: make(map[string]map[authorization.Action]struct{}),
		bindings:        make(map[bindingKey][]string),
	}
}

// CreateRole registers a new role. It fails when the role already exists.
func (p *Provider) CreateRole(role string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.rolePermissions[role]; exists {
		return fmt.Errorf("memory: role %q already exists", role)
	}
	p.rolePermissions[role] = make(map[authorization.Action]struct{})

	return nil
}

// GrantPermissions adds permissions to an existing role. Granting an already
// granted permission is a no-op.
func (p *Provider) GrantPermissions(role string, actions ...authorization.Action) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	perms, exists := p.rolePermissions[role]
	if !exists {
		return fmt.Errorf("memory: unknown role %q", role)
	}
	for _, action := range actions {
		perms[action] = struct{}{}
	}

	return nil
}

// BindRoles binds roles to a subject within a scope. Binding an already bound
// role is a no-op.
func (p *Provider) BindRoles(subjectID string, scope authorization.Scope, roles ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, role := range roles {
		if _, exists := p.rolePermissions[role]; !exists {
			return fmt.Errorf("memory: unknown role %q", role)
		}
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

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}

	return false
}
