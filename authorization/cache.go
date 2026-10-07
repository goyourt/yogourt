package authorization

import (
	"context"
	"sync"
)

// grantCacheKey identifies one memoized provider answer: the stable identity
// of the subject and the exact scope the provider was asked about.
//
// The key is the subject ID alone, never its Attributes: the ID is the stable
// identity of the caller (D4), while Attributes carry request-shaped data a
// map cannot be keyed on. Two Subject values sharing an ID therefore share
// their memoized grants, which is correct — grants are bound to the identity,
// not to the attributes an application chose to attach.
type grantCacheKey struct {
	subjectID string
	scope     Scope
}

// grantCache memoizes the answers of a GrantProvider for the lifetime of a
// single request (AUTHZ-601).
//
// Why it lives on the context and not on the Engine: the engine is published
// process-wide (D3) and shared by every request, so a cache held there would
// outlive the request that filled it — a revocation would stay invisible
// (AUTHZ-602) and, worse, one subject's grants would be served to another as
// soon as the key had a bug. Attaching the cache to the request context makes
// the lifetime structural: nothing has to be invalidated, because nothing
// survives the request.
//
// A handler may fan out into goroutines that all authorize, so the cache is
// safe for concurrent use. The mutex only guards the map: the provider is
// always called outside it, so a slow store never serializes the goroutines
// of a request. Two goroutines racing on the same missing key may therefore
// both call the provider and both store their answer; that is an accepted
// duplicate read, never an incoherence, since Resolve is a read and the
// values are equivalent.
type grantCache struct {
	mu     sync.Mutex
	grants map[grantCacheKey]Grants
}

func newGrantCache() *grantCache {
	return &grantCache{grants: make(map[grantCacheKey]Grants)}
}

// load returns the memoized grants for the key, if any.
func (c *grantCache) load(key grantCacheKey) (Grants, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	grants, ok := c.grants[key]

	return grants, ok
}

// store memoizes the grants of one key. Only successful resolutions are ever
// stored (see resolveScopeGrants).
//
// A stored value is treated as immutable: the engine clones the grants before
// exposing them to a restriction (see Grants.clone), which is what makes it
// safe to hand the same backing arrays to every check of the request.
func (c *grantCache) store(key grantCacheKey, grants Grants) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.grants[key] = grants
}

// WithGrantCache derives a context carrying a fresh, empty per-request grant
// cache. Every grant resolution made with the returned context — or with any
// context derived from it — asks the provider at most once per pair
// (subject, scope), so a request crossing the RBAC middleware and then
// calling Context.Authorize in its handler pays for one resolution instead of
// two.
//
// Call it once where a request begins; the framework already does it in the
// Gin middleware and in the Context helpers. Calling it twice on the same
// request is not wrong, it simply drops the memoization accumulated so far —
// use EnsureGrantCache when a cache may already be in place.
//
// Only RBAC grants are memoized. A final decision is never cached
// (AUTHZ-606): restrictions may depend on the state of the resource, which
// the request itself can change between two checks, so they are re-evaluated
// on every call even when their grants come from the cache.
//
// Nothing requires the cache: a resolution made on a context without one
// queries the provider exactly as before, which keeps the engine usable
// outside HTTP (jobs, CLI, tests) with no memoization at all.
func WithGrantCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, grantCacheContextKey, newGrantCache())
}

// EnsureGrantCache returns ctx unchanged when it already carries a grant
// cache, and derives one otherwise. Middlewares and request helpers use it so
// that a request crossing several authorization layers keeps a single cache,
// and so that the memoization of a layer that ran earlier is never thrown
// away.
func EnsureGrantCache(ctx context.Context) context.Context {
	if _, ok := grantCacheFromContext(ctx); ok {
		return ctx
	}

	return WithGrantCache(ctx)
}

func grantCacheFromContext(ctx context.Context) (*grantCache, bool) {
	cache, ok := ctx.Value(grantCacheContextKey).(*grantCache)

	return cache, ok
}

// resolveScopeGrants asks the provider for the grants bound to one exact
// scope, going through the per-request cache when the context carries one.
//
// A provider error is NEVER memoized. Caching it would freeze a transient
// outage — a lost connection, a timeout, a failover — for the whole request:
// the first failing check would then deny every later check of the same
// request, even the ones the store could have answered. Deny by default
// already covers the failing check itself (ReasonProviderError); making that
// failure sticky would turn one blip into a whole request refused, and would
// also make a retry inside the request pointless.
func resolveScopeGrants(ctx context.Context, provider GrantProvider, subject Subject, scope Scope) (Grants, error) {
	cache, ok := grantCacheFromContext(ctx)
	if !ok {
		return provider.Resolve(ctx, subject, scope)
	}

	key := grantCacheKey{subjectID: subject.ID, scope: scope}
	if grants, hit := cache.load(key); hit {
		return grants, nil
	}

	grants, err := provider.Resolve(ctx, subject, scope)
	if err != nil {
		return Grants{}, err
	}
	cache.store(key, grants)

	return grants, nil
}
