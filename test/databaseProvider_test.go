package test

import (
	"testing"
	"time"

	"github.com/goyourt/yogourt/services/providers"
)

// The test configuration points the database at "host:1000", which nothing
// answers. InitDB used to call log.Fatalf on that failure: the process — this
// test binary included — simply exited, from wherever the first query
// happened to run.
func TestInitDBReturnsUnreachableServerError(t *testing.T) {
	start := time.Now()
	db, err := providers.InitDB()

	if err == nil {
		t.Fatal("InitDB must not return a connection to an unreachable server")
	}
	if db != nil {
		t.Errorf("a failed InitDB must not return a connection, got %v", db)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("InitDB took %v, which is long enough to hang a caller", elapsed)
	}
}

// A failed connection must not be memoized: GetDB stays retryable so a
// database that comes back up is picked up without restarting the process.
func TestGetDBDoesNotMemoizeFailure(t *testing.T) {
	for i := 0; i < 2; i++ {
		db, err := providers.GetDB()
		if err == nil {
			t.Fatalf("call %d: expected an error for an unreachable database", i)
		}
		if db != nil {
			t.Fatalf("call %d: expected a nil connection alongside the error", i)
		}
	}
}
