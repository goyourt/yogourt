//go:build race

package routing

// pluginBuildFlags mirrors the build flags of the test binary onto the route
// plugins it opens: a plugin built without -race cannot be loaded by a
// race-enabled binary (plugin.Open reports a package version mismatch), so a
// "go test -race" run has to rebuild the fixtures with -race too.
var pluginBuildFlags = []string{"-race"}
