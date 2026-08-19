package authorization_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/goyourt/yogourt/authorization"
	"github.com/goyourt/yogourt/authorization/memory"
)

// article mirrors the current application models: the resource references its
// owner by internal SQL id (CreatedById *int), while the subject is
// identified by a stable UUID.
type article struct {
	ID          int
	CreatedById *int
}

// ownsArticle is the canonical ownership restriction. The identity mismatch
// is handled explicitly: the Subject carries both the stable UUID (Subject.ID)
// and the internal SQL id (the "internal_id" attribute), and the comparison
// uses the latter. Any missing piece of identity is a policy error, never an
// authorization.
func ownsArticle(_ context.Context, input authorization.PolicyInput) (bool, error) {
	resource, ok := input.Resource.(article)
	if !ok {
		return false, fmt.Errorf("ownsArticle: unexpected resource type %T", input.Resource)
	}
	if resource.CreatedById == nil {
		return false, nil
	}

	internalID, ok := input.Subject.Attributes["internal_id"].(int)
	if !ok {
		return false, errors.New("ownsArticle: subject has no internal_id attribute")
	}

	return *resource.CreatedById == internalID, nil
}

// ExampleRestriction demonstrates the ownership pattern: RBAC grants the
// "article.update" permission to editors, and the ABAC restriction only lets
// an editor update their own articles.
func ExampleRestriction() {
	provider := memory.NewProvider()
	_ = provider.CreateRole("editor")
	_ = provider.GrantPermissions("editor", "article.update")
	_ = provider.BindRoles("6f1b0c1e-4a94-4f6f-9d3a-1c2b3d4e5f60", authorization.ScopeGlobal, "editor")

	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithRestriction("article.update", ownsArticle),
	)

	ownerID := 42
	resource := article{ID: 7, CreatedById: &ownerID}

	owner := authorization.Subject{
		ID:         "6f1b0c1e-4a94-4f6f-9d3a-1c2b3d4e5f60",
		Attributes: map[string]any{"internal_id": 42},
	}
	decision := engine.Decide(context.Background(), authorization.Request{
		Subject:  owner,
		Action:   "article.update",
		Scope:    authorization.ScopeGlobal,
		Resource: resource,
	})
	fmt.Printf("owner: allowed=%v reason=%s\n", decision.Allowed, decision.Reason)

	// Same role and permission, but not the author of the article.
	_ = provider.BindRoles("0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d", authorization.ScopeGlobal, "editor")
	other := authorization.Subject{
		ID:         "0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d",
		Attributes: map[string]any{"internal_id": 99},
	}
	decision = engine.Decide(context.Background(), authorization.Request{
		Subject:  other,
		Action:   "article.update",
		Scope:    authorization.ScopeGlobal,
		Resource: resource,
	})
	fmt.Printf("other: allowed=%v reason=%s\n", decision.Allowed, decision.Reason)

	// Output:
	// owner: allowed=true reason=allowed
	// other: allowed=false reason=policy_denied
}

// profile is a resource whose visibility is part of its own state.
type profile struct {
	OwnerID int
	Public  bool
}

// readProfile expresses the complete reading rule in a single place: a public
// profile is readable by anyone holding "profiles.read", a private one only by
// its owner or by a subject holding the explicit escalation permission
// "profiles.read_private", which PolicyInput.Grants makes available without an
// extra provider call.
//
// The escalation is a permission test, never a role test: writing
// input.Grants.HasRole("admin") would rebuild the implicit admin bypass that
// deny by default forbids, granting a power no permission row records.
func readProfile(_ context.Context, input authorization.PolicyInput) (bool, error) {
	resource, ok := input.Resource.(profile)
	if !ok {
		return false, fmt.Errorf("readProfile: unexpected resource type %T", input.Resource)
	}
	if resource.Public {
		return true, nil
	}

	internalID, ok := input.Subject.Attributes["internal_id"].(int)
	if !ok {
		return false, errors.New("readProfile: subject has no internal_id attribute")
	}
	if internalID == resource.OwnerID {
		return true, nil
	}

	return input.Grants.HasPermission("profiles.read_private"), nil
}

// ExampleRestriction_publicOrOwnerOrEscalation demonstrates the reference
// "public OR owner OR escalation permission" pattern on a private profile.
func ExampleRestriction_publicOrOwnerOrEscalation() {
	const (
		ownerID     = "6f1b0c1e-4a94-4f6f-9d3a-1c2b3d4e5f60"
		moderatorID = "0a9b8c7d-6e5f-4a3b-2c1d-0e9f8a7b6c5d"
		strangerID  = "3c4d5e6f-7a8b-49c0-a1d2-e3f4a5b6c7d8"
	)

	provider := memory.NewProvider()
	_ = provider.CreateRole("member")
	_ = provider.GrantPermissions("member", "profiles.read")
	_ = provider.CreateRole("moderator")
	// The escalation is a permission of its own, granted and revoked like any
	// other, and visible in an audit of the role.
	_ = provider.GrantPermissions("moderator", "profiles.read", "profiles.read_private")

	_ = provider.BindRoles(ownerID, authorization.ScopeGlobal, "member")
	_ = provider.BindRoles(moderatorID, authorization.ScopeGlobal, "moderator")
	_ = provider.BindRoles(strangerID, authorization.ScopeGlobal, "member")

	engine := authorization.NewEngine(
		authorization.WithProvider(provider),
		authorization.WithRestriction("profiles.read", readProfile),
	)

	subject := func(id string, internalID int) authorization.Subject {
		return authorization.Subject{ID: id, Attributes: map[string]any{"internal_id": internalID}}
	}
	decide := func(label string, caller authorization.Subject, resource profile) {
		decision := engine.Decide(context.Background(), authorization.Request{
			Subject:  caller,
			Action:   "profiles.read",
			Scope:    authorization.ScopeGlobal,
			Resource: resource,
		})
		fmt.Printf("%s: allowed=%v reason=%s\n", label, decision.Allowed, decision.Reason)
	}

	privateProfile := profile{OwnerID: 42}
	publicProfile := profile{OwnerID: 42, Public: true}

	decide("public/stranger", subject(strangerID, 99), publicProfile)
	decide("private/owner", subject(ownerID, 42), privateProfile)
	decide("private/moderator", subject(moderatorID, 7), privateProfile)
	decide("private/stranger", subject(strangerID, 99), privateProfile)

	// Output:
	// public/stranger: allowed=true reason=allowed
	// private/owner: allowed=true reason=allowed
	// private/moderator: allowed=true reason=allowed
	// private/stranger: allowed=false reason=policy_denied
}
