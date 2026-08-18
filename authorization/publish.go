package authorization

import (
	"errors"

	"github.com/goyourt/yogourt/authorization/internal/runtime"
)

// ErrNoProvider is returned when an RBAC check is attempted on an engine
// built without a grant provider.
var ErrNoProvider = errors.New("authorization: no grant provider configured")

// ErrAlreadyPublished is returned by Publish when an engine was already
// published; the shared engine is write-once and can never be replaced.
var ErrAlreadyPublished = errors.New("authorization: engine already published")

// Publish makes the engine available process-wide, for the packages that
// cannot receive it explicitly (such as the lazy ginmw.Middleware). It must
// be called once, before routes are loaded; a second call fails.
func Publish(engine *Engine) error {
	if engine == nil {
		return errors.New("authorization: cannot publish a nil engine")
	}
	if !runtime.Publish(engine) {
		return ErrAlreadyPublished
	}

	return nil
}

// Published returns the published engine, if any.
func Published() (*Engine, bool) {
	value, ok := runtime.Published()
	if !ok {
		return nil, false
	}
	engine, ok := value.(*Engine)

	return engine, ok
}
