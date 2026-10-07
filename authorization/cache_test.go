package authorization

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// countingProvider is a concurrency-safe provider counting the resolutions it
// answers, per subject and per scope. failures is decremented on each call:
// the first failures calls return an error, the next ones succeed.
type countingProvider struct {
	mu       sync.Mutex
	grants   map[string]map[Scope]Grants
	calls    map[grantCacheKey]int
	total    int
	failures int
}

func newCountingProvider() *countingProvider {
	return &countingProvider{
		grants: make(map[string]map[Scope]Grants),
		calls:  make(map[grantCacheKey]int),
	}
}

// grant binds a permission to a subject within a scope.
func (p *countingProvider) grant(subjectID string, scope Scope, actions ...Action) *countingProvider {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.grants[subjectID] == nil {
		p.grants[subjectID] = make(map[Scope]Grants)
	}
	current := p.grants[subjectID][scope]
	current.Permissions = append(current.Permissions, actions...)
	p.grants[subjectID][scope] = current

	return p
}

// revoke drops every grant of a subject within a scope.
func (p *countingProvider) revoke(subjectID string, scope Scope) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.grants[subjectID], scope)
}

func (p *countingProvider) Resolve(_ context.Context, subject Subject, scope Scope) (Grants, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls[grantCacheKey{subjectID: subject.ID, scope: scope}]++
	p.total++
	if p.failures > 0 {
		p.failures--

		return Grants{}, errors.New("store unavailable")
	}

	return p.grants[subject.ID][scope], nil
}

func (p *countingProvider) totalCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.total
}

func (p *countingProvider) callsFor(subjectID string, scope Scope) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.calls[grantCacheKey{subjectID: subjectID, scope: scope}]
}

func (p *countingProvider) resetCalls() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls = make(map[grantCacheKey]int)
	p.total = 0
}

const cachedAction Action = "article.read"

func decideRead(t *testing.T, engine *Engine, ctx context.Context, subjectID string, scope Scope) Decision {
	t.Helper()

	return engine.Decide(ctx, Request{
		Subject: Subject{ID: subjectID},
		Action:  cachedAction,
		Scope:   scope,
	})
}

// TestGrantCacheMemoizesProviderCalls covers AUTHZ-601: a request crossing
// the RBAC check and then a full decision resolves the grants once per
// (subject, scope) instead of once per check.
func TestGrantCacheMemoizesProviderCalls(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)
	engine := NewEngine(WithProvider(provider))

	ctx := WithGrantCache(context.Background())

	allowed, err := engine.HasPermission(ctx, Subject{ID: "subject-1"}, "tenant-1", cachedAction)
	if err != nil || !allowed {
		t.Fatalf("HasPermission = (%v, %v), want (true, nil)", allowed, err)
	}
	if got := provider.totalCalls(); got != 2 {
		t.Fatalf("first check made %d provider calls, want 2 (scope + global)", got)
	}

	// The same request now takes the full decision, as a handler calling
	// Context.Authorize after the middleware does.
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("decision = %+v, want allowed", decision)
	}
	// Three more checks, none of which may reach the provider.
	decideRead(t, engine, ctx, "subject-1", "tenant-1")
	engine.HasPermission(ctx, Subject{ID: "subject-1"}, "tenant-1", cachedAction)
	engine.HasPermission(ctx, Subject{ID: "subject-1"}, ScopeGlobal, cachedAction)

	if got := provider.totalCalls(); got != 2 {
		t.Errorf("total provider calls = %d, want 2: the grants must be memoized for the request", got)
	}
	if got := provider.callsFor("subject-1", ScopeGlobal); got != 1 {
		t.Errorf("ScopeGlobal resolved %d times, want 1", got)
	}
}

// TestGrantCacheAbsentKeepsProviderCalls proves the engine keeps working with
// no cache at all — a job, a CLI, a direct call outside HTTP — and then makes
// exactly the calls it made before the memoization existed.
func TestGrantCacheAbsentKeepsProviderCalls(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)
	engine := NewEngine(WithProvider(provider))

	ctx := context.Background()
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("decision = %+v, want allowed", decision)
	}
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("decision = %+v, want allowed", decision)
	}

	if got := provider.totalCalls(); got != 4 {
		t.Errorf("total provider calls = %d, want 4: no context cache means no memoization", got)
	}
}

// TestGrantCacheDoesNotLeakBetweenRequests covers AUTHZ-602: the cache is
// strictly per request, so a revocation is visible from the next request on.
func TestGrantCacheDoesNotLeakBetweenRequests(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)
	engine := NewEngine(WithProvider(provider))

	// First request: allowed, and the grants land in its cache.
	first := WithGrantCache(context.Background())
	if decision := decideRead(t, engine, first, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("first request: decision = %+v, want allowed", decision)
	}

	provider.revoke("subject-1", "tenant-1")

	// Second request: a fresh cache, so the revocation is seen immediately.
	second := WithGrantCache(context.Background())
	decision := decideRead(t, engine, second, "subject-1", "tenant-1")
	if decision.Allowed {
		t.Error("second request: the revocation must be visible right away")
	}
	if decision.Reason != ReasonMissingPermission {
		t.Errorf("second request: reason = %q, want %q", decision.Reason, ReasonMissingPermission)
	}
}

// TestGrantCacheDoesNotLeakBetweenSubjects proves the cache key includes the
// subject: two subjects sharing one request context never share grants.
func TestGrantCacheDoesNotLeakBetweenSubjects(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)
	engine := NewEngine(WithProvider(provider))

	ctx := WithGrantCache(context.Background())
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("granted subject: decision = %+v, want allowed", decision)
	}

	decision := decideRead(t, engine, ctx, "subject-2", "tenant-1")
	if decision.Allowed {
		t.Error("a second subject must not inherit the grants memoized for the first")
	}
	if got := provider.callsFor("subject-2", "tenant-1"); got != 1 {
		t.Errorf("subject-2 resolutions = %d, want 1: its own grants must be asked for", got)
	}
}

// TestGrantCacheDoesNotLeakBetweenScopes proves the cache key includes the
// scope: grants of one tenant never answer for another.
func TestGrantCacheDoesNotLeakBetweenScopes(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)
	engine := NewEngine(WithProvider(provider))

	ctx := WithGrantCache(context.Background())
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-1"); !decision.Allowed {
		t.Fatalf("tenant-1: decision = %+v, want allowed", decision)
	}
	if decision := decideRead(t, engine, ctx, "subject-1", "tenant-2"); decision.Allowed {
		t.Error("tenant-2 must not inherit the grants memoized for tenant-1")
	}
}

// TestGrantCacheNeverMemoizesErrors proves a transient provider outage is not
// frozen for the whole request: the check that failed is denied, the next one
// asks the provider again and succeeds.
func TestGrantCacheNeverMemoizesErrors(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", ScopeGlobal, cachedAction)
	provider.failures = 1
	engine := NewEngine(WithProvider(provider))

	ctx := WithGrantCache(context.Background())

	decision := decideRead(t, engine, ctx, "subject-1", ScopeGlobal)
	if decision.Allowed || decision.Reason != ReasonProviderError {
		t.Fatalf("failing check: decision = %+v, want denied with %q", decision, ReasonProviderError)
	}

	if decision := decideRead(t, engine, ctx, "subject-1", ScopeGlobal); !decision.Allowed {
		t.Errorf("recovered check: decision = %+v, want allowed: an error must never be memoized", decision)
	}
	if got := provider.callsFor("subject-1", ScopeGlobal); got != 2 {
		t.Errorf("resolutions = %d, want 2: the failed resolution must be retried", got)
	}
}

// mutableArticle is a resource whose state changes during the request, which
// is exactly why a final decision can never be cached.
type mutableArticle struct {
	locked bool
}

// TestRestrictionsReevaluatedDespiteCache covers AUTHZ-606: only RBAC grants
// are memoized. The restriction runs on every call and sees the state of the
// resource as it is at that moment, even though the grants come from the
// cache.
func TestRestrictionsReevaluatedDespiteCache(t *testing.T) {
	provider := newCountingProvider().grant("subject-1", "tenant-1", cachedAction)

	calls := 0
	engine := NewEngine(
		WithProvider(provider),
		WithRestriction(cachedAction, func(_ context.Context, input PolicyInput) (bool, error) {
			calls++
			article, ok := input.Resource.(*mutableArticle)

			return ok && !article.locked, nil
		}),
	)

	ctx := WithGrantCache(context.Background())
	article := &mutableArticle{}
	request := Request{
		Subject:  Subject{ID: "subject-1"},
		Action:   cachedAction,
		Scope:    "tenant-1",
		Resource: article,
	}

	if decision := engine.Decide(ctx, request); !decision.Allowed {
		t.Fatalf("unlocked article: decision = %+v, want allowed", decision)
	}

	// The request itself changes the state of the resource: a cached decision
	// would keep authorizing.
	article.locked = true

	decision := engine.Decide(ctx, request)
	if decision.Allowed {
		t.Error("locked article: the restriction must be re-evaluated, never cached")
	}
	if decision.Reason != ReasonPolicyDenied {
		t.Errorf("reason = %q, want %q", decision.Reason, ReasonPolicyDenied)
	}
	if calls != 2 {
		t.Errorf("restriction calls = %d, want 2: one per decision", calls)
	}
	if got := provider.totalCalls(); got != 2 {
		t.Errorf("provider calls = %d, want 2: the grants alone are memoized", got)
	}
}

// TestGrantCacheConcurrentUse drives the cache of one request from many
// goroutines, as a handler fanning out does. Run under -race.
func TestGrantCacheConcurrentUse(t *testing.T) {
	provider := newCountingProvider().
		grant("subject-1", "tenant-1", cachedAction).
		grant("subject-2", ScopeGlobal, cachedAction)
	engine := NewEngine(
		WithProvider(provider),
		WithRestriction(cachedAction, func(_ context.Context, input PolicyInput) (bool, error) {
			// Reads the grants it received, which must be a private copy.
			return input.Grants.HasPermission(cachedAction), nil
		}),
	)

	ctx := WithGrantCache(context.Background())

	const goroutines = 64
	var wg sync.WaitGroup
	results := make([]bool, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()

			subjectID, scope := "subject-1", Scope("tenant-1")
			if i%2 == 1 {
				subjectID, scope = "subject-2", ScopeGlobal
			}
			results[i] = engine.Decide(ctx, Request{
				Subject:  Subject{ID: subjectID},
				Action:   cachedAction,
				Scope:    scope,
				Resource: &mutableArticle{},
			}).Allowed
		}()
	}
	wg.Wait()

	for i, allowed := range results {
		if !allowed {
			t.Fatalf("goroutine %d was denied: the concurrent cache must stay coherent", i)
		}
	}
}

// TestEnsureGrantCache pins the difference between the two entry points: one
// always derives a fresh cache, the other keeps an existing one so that a
// middleware and a handler share the memoization of a single request.
func TestEnsureGrantCache(t *testing.T) {
	base := context.Background()
	if _, ok := grantCacheFromContext(base); ok {
		t.Fatal("a bare context must not carry a grant cache")
	}

	first := EnsureGrantCache(base)
	cache, ok := grantCacheFromContext(first)
	if !ok {
		t.Fatal("EnsureGrantCache must derive a cache when none is present")
	}

	if second := EnsureGrantCache(first); second != first {
		t.Error("EnsureGrantCache must return the context unchanged when a cache is already present")
	}

	fresh, ok := grantCacheFromContext(WithGrantCache(first))
	if !ok {
		t.Fatal("WithGrantCache must always derive a cache")
	}
	if fresh == cache {
		t.Error("WithGrantCache must derive a fresh cache, not reuse the one in place")
	}
}
