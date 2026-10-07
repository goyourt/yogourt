package authorization

import (
	"errors"
	"testing"
)

// TestPublish exercises the write-once shared engine in a single test to keep
// full control over ordering: the process-wide storage cannot be reset.
func TestPublish(t *testing.T) {
	if _, ok := Published(); ok {
		t.Fatal("expected no engine before Publish")
	}

	if err := Publish(nil); err == nil {
		t.Error("expected an error when publishing a nil engine")
	}

	engine := NewEngine()
	if err := Publish(engine); err != nil {
		t.Fatalf("first Publish failed: %v", err)
	}

	published, ok := Published()
	if !ok || published != engine {
		t.Error("Published must return the published engine")
	}

	if err := Publish(NewEngine()); !errors.Is(err, ErrAlreadyPublished) {
		t.Errorf("second Publish error = %v, want ErrAlreadyPublished", err)
	}

	// The first engine stays in place: write-once, never replaced.
	if published, _ := Published(); published != engine {
		t.Error("a failed Publish must not replace the engine")
	}
}
