package authorization

import (
	"context"
	"errors"
	"testing"
)

type syncingProvider struct {
	synced []Action
	err    error
}

func (p *syncingProvider) Resolve(context.Context, Subject, Scope) (Grants, error) {
	return Grants{}, nil
}

func (p *syncingProvider) SyncPermissions(_ context.Context, permissions []Action) error {
	p.synced = append(p.synced, permissions...)

	return p.err
}

type plainProvider struct{}

func (plainProvider) Resolve(context.Context, Subject, Scope) (Grants, error) {
	return Grants{}, nil
}

func TestSyncPermissionsForwardsToSyncer(t *testing.T) {
	provider := &syncingProvider{}
	engine := NewEngine(WithProvider(provider))

	if err := engine.SyncPermissions(context.Background(), []Action{"article.read", "article.update"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(provider.synced) != 2 {
		t.Errorf("expected 2 synced permissions, got %v", provider.synced)
	}
}

func TestSyncPermissionsPropagatesError(t *testing.T) {
	provider := &syncingProvider{err: errors.New("store unavailable")}
	engine := NewEngine(WithProvider(provider))

	if err := engine.SyncPermissions(context.Background(), []Action{"article.read"}); err == nil {
		t.Fatal("a store failure must be propagated, never swallowed")
	}
}

func TestSyncPermissionsNoopWithoutSyncer(t *testing.T) {
	engine := NewEngine(WithProvider(plainProvider{}))

	if err := engine.SyncPermissions(context.Background(), []Action{"article.read"}); err != nil {
		t.Fatalf("a provider without sync support must make sync a no-op, got %v", err)
	}
}

func TestSyncPermissionsNoopWithoutProviderOrPermissions(t *testing.T) {
	if err := NewEngine().SyncPermissions(context.Background(), []Action{"a"}); err != nil {
		t.Fatalf("unexpected error without provider: %v", err)
	}
	provider := &syncingProvider{}
	engine := NewEngine(WithProvider(provider))
	if err := engine.SyncPermissions(context.Background(), nil); err != nil || len(provider.synced) != 0 {
		t.Fatalf("empty permission list must be a no-op, got err=%v synced=%v", err, provider.synced)
	}
}
