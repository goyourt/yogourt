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
