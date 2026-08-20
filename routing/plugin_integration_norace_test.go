//go:build !race

package routing

// pluginBuildFlags carries no extra flag: the test binary is not race-enabled,
// so the route plugins must not be either.
var pluginBuildFlags []string
