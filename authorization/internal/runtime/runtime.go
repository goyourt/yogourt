// Package runtime holds the shared engine published by the application. The
// value is written once before routes are loaded and is never replaced or
// cleared afterwards; no mutation API exists.
package runtime

import (
	"sync"
	"sync/atomic"
)

var (
	once   sync.Once
	engine atomic.Value
)

// Publish stores the engine on the first call and reports whether the write
// happened. Any subsequent call is a no-op returning false.
func Publish(e any) bool {
	published := false
	once.Do(func() {
		engine.Store(e)
		published = true
	})

	return published
}

// Published returns the stored engine, if one was published.
func Published() (any, bool) {
	e := engine.Load()

	return e, e != nil
}
