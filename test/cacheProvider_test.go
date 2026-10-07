package test

import (
	"testing"
	"time"

	"github.com/goyourt/yogourt/services"
	"github.com/goyourt/yogourt/services/providers"
)

// The test configuration points the cache at "host:1000", which nothing
// answers. InitCache used to hand back a perfectly usable-looking client for
// that address: the misconfiguration only showed up later, inside whichever
// request first touched the cache.
func TestInitCacheRejectsUnreachableInstance(t *testing.T) {
	start := time.Now()
	client, err := providers.InitCache()

	if err == nil {
		client.Close()
		t.Fatalf("InitCache must not return a client for an unreachable instance")
	}
	if client != nil {
		t.Errorf("a failed InitCache must not return a client, got %v", client)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("InitCache must fail fast, took %v", elapsed)
	}
}

// A failed connection must not be memoized: GetCache stays retryable so a
// cache that comes back up is picked up without restarting the process.
func TestGetCacheDoesNotMemoizeFailure(t *testing.T) {
	for i := 0; i < 2; i++ {
		cache, err := providers.GetCache()
		if err == nil {
			t.Fatalf("call %d: expected an error for an unreachable cache", i)
		}
		if cache != nil {
			t.Fatalf("call %d: expected a nil client alongside the error", i)
		}
	}
}

// The password failure counters are the only consumers of the cache: they must
// surface the connection error instead of panicking on a nil client.
func TestPasswordFailureTrackingReportsCacheError(t *testing.T) {
	count, err := services.GetPasswordFailureCount("user")
	if err == nil {
		t.Errorf("GetPasswordFailureCount must report the cache error")
	}
	if count != 0 {
		t.Errorf("expected no attempt counted, got %d", count)
	}

	if err := services.SavePasswordFailure("user"); err == nil {
		t.Errorf("SavePasswordFailure must report the cache error")
	}
}
