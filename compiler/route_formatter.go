package compiler

import "strings"

func SlugRouteFormater(route string) string {
	if len(route) >= 3 && strings.HasPrefix(route, "[") && strings.HasSuffix(route, "]") {
		return ":" + route[1:len(route)-1]
	}

	if len(route) > 1 && strings.HasSuffix(route, "_") && !strings.HasPrefix(route, "_") {
		return ":" + strings.TrimSuffix(route, "_")
	}

	// Backward compatibility with legacy _param folder format.
	if strings.HasPrefix(route, "_") && len(route) > 1 {
		return ":" + route[1:]
	}

	return route
}
