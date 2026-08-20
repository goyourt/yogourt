package authorization_test

import (
	"context"
	"fmt"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
)

// ExampleWithDecisionHook shows the observability hook of the engine: one
// event per decision, made of scalars only, ready to be logged or turned into
// metric labels. The hook never receives the business resource, so no
// authorization log can accidentally spill a whole object (AUTHZ-605).
func ExampleWithDecisionHook() {
	provider := memory.NewProvider()
	_ = provider.CreateRole(context.Background(), "editor")
	_ = provider.GrantPermissions(context.Background(), "editor", "article.update")
	_ = provider.BindRoles(context.Background(), "subject-1", authorization.ScopeGlobal, "editor")

	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithDecisionHook(func(_ context.Context, event authorization.DecisionEvent) {
			// A real hook increments a counter labelled by action, reason and
			// outcome, and observes event.Duration in a histogram.
			fmt.Printf("%s subject=%s action=%s scope=%s allowed=%t reason=%s\n",
				event.Kind, event.SubjectID, event.Action, event.Scope, event.Allowed, event.Reason)
		}),
	)

	ctx := context.Background()
	subject := authorization.Subject{ID: "subject-1"}

	// What the RBAC middleware asks, before the resource is loaded.
	_, _ = engine.HasPermission(ctx, subject, authorization.ScopeGlobal, "article.update")

	// What the handler asks once the resource is loaded.
	engine.Decide(ctx, authorization.Request{
		Subject:  subject,
		Action:   "article.update",
		Scope:    authorization.ScopeGlobal,
		Resource: struct{ ID int }{ID: 7},
	})

	// A permission nobody was granted.
	engine.Decide(ctx, authorization.Request{
		Subject: subject,
		Action:  "article.delete",
		Scope:   authorization.ScopeGlobal,
	})

	// Output:
	// permission subject=subject-1 action=article.update scope=@global allowed=true reason=allowed
	// full subject=subject-1 action=article.update scope=@global allowed=true reason=allowed
	// full subject=subject-1 action=article.delete scope=@global allowed=false reason=missing_permission
}

// ExampleAuditGrantAdmin shows the audit trail of role administration: the
// decorator wraps any GrantAdmin and emits one event per mutation, successful
// or failed. The actor is not known to the store — it comes from the context
// the hook receives.
func ExampleAuditGrantAdmin() {
	store := memory.NewProvider()

	admin := authorization.AuditGrantAdmin(store, func(ctx context.Context, event authorization.AuditEvent) {
		actor, _ := authorization.SubjectFromContext(ctx)
		fmt.Printf("actor=%s %s roles=%v subject=%s scope=%s permissions=%v err=%v\n",
			actor.ID, event.Mutation, event.Roles, event.SubjectID, event.Scope, event.Permissions, event.Err)
	})

	// The context of the admin request carries the authenticated actor.
	ctx := authorization.WithSubject(context.Background(), authorization.Subject{ID: "admin-7"})

	_ = admin.CreateRole(ctx, "editor")
	_ = admin.GrantPermissions(ctx, "editor", "article.update")
	_ = admin.BindRoles(ctx, "subject-1", "tenant-1", "editor")

	// A refused mutation is audited too: binding an unknown role fails.
	_ = admin.BindRoles(ctx, "subject-1", "tenant-1", "ghost")

	// Reads pass through and emit nothing.
	roles, _ := admin.Roles(ctx)
	fmt.Println("roles:", roles)

	// Output:
	// actor=admin-7 create_role roles=[editor] subject= scope= permissions=[] err=<nil>
	// actor=admin-7 grant_permissions roles=[editor] subject= scope= permissions=[article.update] err=<nil>
	// actor=admin-7 bind_roles roles=[editor] subject=subject-1 scope=tenant-1 permissions=[] err=<nil>
	// actor=admin-7 bind_roles roles=[ghost] subject=subject-1 scope=tenant-1 permissions=[] err=memory: unknown role "ghost"
	// roles: [editor]
}

// ExampleWithGrantCache shows the per-request memoization outside HTTP: a
// context carrying a grant cache asks the provider once per (subject, scope),
// however many checks the request makes. The framework already does this in
// the Gin middleware and in the Context helpers.
func ExampleWithGrantCache() {
	provider := memory.NewProvider()
	_ = provider.CreateRole(context.Background(), "editor")
	_ = provider.GrantPermissions(context.Background(), "editor", "article.update")
	_ = provider.BindRoles(context.Background(), "subject-1", authorization.ScopeGlobal, "editor")

	engine := authorization.NewEngine(authorization.WithProvider(provider))

	// One unit of work = one cache. Nothing survives it, so a revocation is
	// visible from the next one on.
	ctx := authorization.WithGrantCache(context.Background())
	subject := authorization.Subject{ID: "subject-1"}

	for range 3 {
		allowed, _ := engine.HasPermission(ctx, subject, authorization.ScopeGlobal, "article.update")
		fmt.Println("allowed:", allowed)
	}

	// Output:
	// allowed: true
	// allowed: true
	// allowed: true
}
