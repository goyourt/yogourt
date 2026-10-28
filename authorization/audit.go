package authorization

import (
	"context"
	"log"
)

// GrantMutation identifies a role administration mutation.
type GrantMutation string

const (
	MutationCreateRole        GrantMutation = "create_role"
	MutationDeleteRole        GrantMutation = "delete_role"
	MutationGrantPermissions  GrantMutation = "grant_permissions"
	MutationRevokePermissions GrantMutation = "revoke_permissions"
	MutationBindRoles         GrantMutation = "bind_roles"
	MutationUnbindRoles       GrantMutation = "unbind_roles"
)

// AuditEvent describes one attempted role administration mutation. It is
// emitted for successes and for failures alike: a refused attempt to grant
// oneself a role is exactly the event an audit trail exists for, and a trail
// that only recorded successes would hide it.
//
// Which fields are set depends on the mutation:
//
//	create_role, delete_role                  Roles (one)
//	grant_permissions, revoke_permissions     Roles (one), Permissions
//	bind_roles, unbind_roles                  Roles (the list), SubjectID, Scope
//
// The actor of the mutation is absent on purpose: a store administers grants,
// it does not authenticate anyone, so it cannot know who asked. An
// application that needs the actor reads it from the context the hook
// receives — typically SubjectFromContext(ctx), the same subject the
// authorization layer used — and is free to carry any other request-scoped
// value (request id, trace, source address) the same way.
type AuditEvent struct {
	// Mutation is the operation that was attempted.
	Mutation GrantMutation
	// Roles holds the roles the mutation targets: exactly one for the
	// role-scoped mutations, the whole list passed by the caller for the
	// binding mutations.
	Roles []string
	// SubjectID is the subject whose bindings change, for the binding
	// mutations only.
	SubjectID string
	// Scope is the scope of the binding, for the binding mutations only.
	Scope Scope
	// Permissions holds the permissions granted or revoked.
	Permissions []Action
	// Err is the error the store returned, nil when the mutation succeeded.
	Err error
}

// AuditHook receives one AuditEvent after each attempted mutation.
//
// It is called synchronously, after the store answered, and its return value
// — it has none — cannot change the outcome: the mutation already happened
// (or already failed) when the hook runs. A hook doing anything slow must
// hand its work to its own queue.
type AuditHook func(ctx context.Context, event AuditEvent)

// AuditGrantAdmin wraps a GrantAdmin so that every mutation emits an audit
// event, successful or not (AUTHZ-603). Reads pass straight through and emit
// nothing: listing roles or bindings is not a change of the authorization
// state, and auditing reads would drown the trail that matters.
//
// The engine is deliberately not on this path — mutations never go through
// it, they go through the provider — so the decoration happens where the
// mutations actually are:
//
//	admin := authorization.AuditGrantAdmin(store, func(ctx context.Context, event authorization.AuditEvent) {
//	    actor, _ := authorization.SubjectFromContext(ctx)
//	    log.Printf("authz audit: actor=%s %s roles=%v subject=%s scope=%s permissions=%v err=%v",
//	        actor.ID, event.Mutation, event.Roles, event.SubjectID, event.Scope, event.Permissions, event.Err)
//	})
//
// The wrapper exposes the GrantAdmin contract and nothing more: the store
// keeps being registered as the GrantProvider (WithProvider) and as the
// PermissionSyncer, since resolving grants and synchronizing the permission
// catalogue are not mutations of who holds what. A nil hook or a nil admin
// returns the admin unchanged, so wiring the decorator in cannot become the
// thing that breaks administration.
func AuditGrantAdmin(admin GrantAdmin, hook AuditHook) GrantAdmin {
	if admin == nil || hook == nil {
		return admin
	}

	return &auditedGrantAdmin{admin: admin, hook: hook}
}

// auditedGrantAdmin is the decorator returned by AuditGrantAdmin. It holds no
// state of its own, so it is exactly as safe for concurrent use as the admin
// it wraps.
type auditedGrantAdmin struct {
	admin GrantAdmin
	hook  AuditHook
}

var _ GrantAdmin = (*auditedGrantAdmin)(nil)

func (a *auditedGrantAdmin) CreateRole(ctx context.Context, role string) error {
	err := a.admin.CreateRole(ctx, role)
	a.emit(ctx, AuditEvent{
		Mutation: MutationCreateRole,
		Roles:    []string{role},
		Err:      err,
	})

	return err
}

func (a *auditedGrantAdmin) DeleteRole(ctx context.Context, role string) error {
	err := a.admin.DeleteRole(ctx, role)
	a.emit(ctx, AuditEvent{
		Mutation: MutationDeleteRole,
		Roles:    []string{role},
		Err:      err,
	})

	return err
}

func (a *auditedGrantAdmin) GrantPermissions(ctx context.Context, role string, actions ...Action) error {
	err := a.admin.GrantPermissions(ctx, role, actions...)
	a.emit(ctx, AuditEvent{
		Mutation:    MutationGrantPermissions,
		Roles:       []string{role},
		Permissions: copyActions(actions),
		Err:         err,
	})

	return err
}

func (a *auditedGrantAdmin) RevokePermissions(ctx context.Context, role string, actions ...Action) error {
	err := a.admin.RevokePermissions(ctx, role, actions...)
	a.emit(ctx, AuditEvent{
		Mutation:    MutationRevokePermissions,
		Roles:       []string{role},
		Permissions: copyActions(actions),
		Err:         err,
	})

	return err
}

func (a *auditedGrantAdmin) BindRoles(ctx context.Context, subjectID string, scope Scope, roles ...string) error {
	err := a.admin.BindRoles(ctx, subjectID, scope, roles...)
	a.emit(ctx, AuditEvent{
		Mutation:  MutationBindRoles,
		Roles:     copyRoles(roles),
		SubjectID: subjectID,
		Scope:     scope,
		Err:       err,
	})

	return err
}

func (a *auditedGrantAdmin) UnbindRoles(ctx context.Context, subjectID string, scope Scope, roles ...string) error {
	err := a.admin.UnbindRoles(ctx, subjectID, scope, roles...)
	a.emit(ctx, AuditEvent{
		Mutation:  MutationUnbindRoles,
		Roles:     copyRoles(roles),
		SubjectID: subjectID,
		Scope:     scope,
		Err:       err,
	})

	return err
}

// Roles passes through: a read is not audited.
func (a *auditedGrantAdmin) Roles(ctx context.Context) ([]string, error) {
	return a.admin.Roles(ctx)
}

// Permissions passes through: a read is not audited.
func (a *auditedGrantAdmin) Permissions(ctx context.Context) ([]Action, error) {
	return a.admin.Permissions(ctx)
}

// RolePermissions passes through: a read is not audited.
func (a *auditedGrantAdmin) RolePermissions(ctx context.Context, role string) ([]Action, error) {
	return a.admin.RolePermissions(ctx, role)
}

// Bindings passes through: a read is not audited.
func (a *auditedGrantAdmin) Bindings(ctx context.Context, subjectID string) ([]Binding, error) {
	return a.admin.Bindings(ctx, subjectID)
}

// RoleBindings passes through: a read is not audited.
func (a *auditedGrantAdmin) RoleBindings(ctx context.Context, role string) ([]Binding, error) {
	return a.admin.RoleBindings(ctx, role)
}

// emit hands one event to the hook, containing its panics: the mutation is
// already committed when the hook runs, so a panicking audit hook must not
// turn a successful mutation into a failed request, nor change the error the
// store returned.
func (a *auditedGrantAdmin) emit(ctx context.Context, event AuditEvent) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("authorization: audit hook panicked on %s: %v", event.Mutation, recovered)
		}
	}()

	a.hook(ctx, event)
}

// copyRoles and copyActions defensively copy the variadic slices before they
// travel to a hook: the caller's array must not be observable — let alone
// mutable — from an audit hook, and a hook keeping its event for a batched
// writer must not see it change under it.
func copyRoles(roles []string) []string {
	if len(roles) == 0 {
		return nil
	}

	return append([]string(nil), roles...)
}

func copyActions(actions []Action) []Action {
	if len(actions) == 0 {
		return nil
	}

	return append([]Action(nil), actions...)
}
