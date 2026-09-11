package services

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestPasswordFailureKeyNamespacesAndHashesIdentity(t *testing.T) {
	username := "alice@example.test"
	key := passwordFailureKey(username)

	if !strings.HasPrefix(key, passwordFailureKeyPrefix) {
		t.Fatalf("key %q does not use the password failure namespace", key)
	}
	if strings.Contains(key, username) {
		t.Fatalf("key %q exposes the supplied identity", key)
	}
	if key == passwordFailureKey("bob@example.test") {
		t.Fatal("different identities must use different password failure keys")
	}
}

// This integration test needs a Redis instance because the production
// operation is a Lua script. It deliberately uses only a test-specific,
// namespaced key and removes that key when it finishes.
func TestPasswordFailureCounterWithRedis(t *testing.T) {
	address := os.Getenv("YOGOURT_TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set YOGOURT_TEST_REDIS_ADDR to run Redis password failure tests")
	}

	ctx := context.Background()
	client := redis.NewClient(&redis.Options{
		Addr:     address,
		Password: os.Getenv("YOGOURT_TEST_REDIS_PASSWORD"),
	})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("connect test Redis: %v", err)
	}

	identity := "password-failure-test/" + t.Name()
	key := passwordFailureKey(identity)
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Fatalf("clear test key: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(ctx, key) })

	const attempts = 32
	counts := make(chan int, attempts)
	errs := make(chan error, attempts)
	var group sync.WaitGroup
	for range attempts {
		group.Add(1)
		go func() {
			defer group.Done()
			count, err := recordPasswordFailure(ctx, client, identity)
			if err != nil {
				errs <- err
				return
			}
			counts <- count
		}()
	}
	group.Wait()
	close(counts)
	close(errs)

	for err := range errs {
		t.Errorf("record password failure: %v", err)
	}
	if t.Failed() {
		return
	}

	seen := make(map[int]bool, attempts)
	for count := range counts {
		if count < 1 || count > attempts {
			t.Errorf("unexpected atomic count %d", count)
			continue
		}
		if seen[count] {
			t.Errorf("count %d was returned more than once", count)
		}
		seen[count] = true
	}
	if len(seen) != attempts {
		t.Errorf("recorded counts = %d, want %d", len(seen), attempts)
	}

	count, err := getPasswordFailureCount(ctx, client, identity)
	if err != nil {
		t.Fatalf("count password failures: %v", err)
	}
	if count != attempts {
		t.Errorf("count = %d, want %d", count, attempts)
	}

	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("read key TTL: %v", err)
	}
	if ttl <= 0 || ttl > passwordFailureWindow {
		t.Errorf("key TTL = %v, want in (0, %v]", ttl, passwordFailureWindow)
	}

	staleIdentity := identity + "/stale"
	staleKey := passwordFailureKey(staleIdentity)
	t.Cleanup(func() { _ = client.Del(ctx, staleKey) })
	if err := client.ZAdd(ctx, staleKey, redis.Z{
		Score:  float64(time.Now().Add(-passwordFailureWindow - time.Second).Unix()),
		Member: "expired-event",
	}).Err(); err != nil {
		t.Fatalf("seed expired password failure: %v", err)
	}

	count, err = getPasswordFailureCount(ctx, client, staleIdentity)
	if err != nil {
		t.Fatalf("prune expired password failure: %v", err)
	}
	if count != 0 {
		t.Errorf("expired failure count = %d, want 0", count)
	}
	if exists, err := client.Exists(ctx, staleKey).Result(); err != nil {
		t.Fatalf("check pruned key: %v", err)
	} else if exists != 0 {
		t.Error("expired-only password failure key was retained")
	}

	// Scores and retention use seconds. Failures from earlier today must
	// survive both reading and recording; only entries older than a day expire.
	serverNow, err := client.Time(ctx).Result()
	if err != nil {
		t.Fatalf("read Redis clock: %v", err)
	}
	if err := client.ZAdd(ctx, staleKey,
		redis.Z{Score: float64(serverNow.Add(-25 * time.Hour).Unix()), Member: "old"},
		redis.Z{Score: float64(serverNow.Add(-12 * time.Hour).Unix()), Member: "earlier-today"},
	).Err(); err != nil {
		t.Fatal(err)
	}
	if count, err := getPasswordFailureCount(ctx, client, staleIdentity); err != nil || count != 1 {
		t.Fatalf("retained count = %d, error = %v; want one failure from today", count, err)
	}
	if count, err := recordPasswordFailure(ctx, client, staleIdentity); err != nil || count != 2 {
		t.Fatalf("count after recording = %d, error = %v; want two failures", count, err)
	}
}

func TestRecordPasswordFailureHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = client.Close() })

	_, err := recordPasswordFailure(ctx, client, "context-test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("record failure error = %v, want context cancellation", err)
	}
}
