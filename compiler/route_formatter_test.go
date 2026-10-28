package compiler

import "testing"

func TestSlugRouteFormater(t *testing.T) {
	cases := map[string]string{
		"users":       "users",
		"[id]":        ":id",
		"id_":         ":id",
		"userId_":     ":userId",
		"postSlug_":   ":postSlug",
		"_legacySlug": ":legacySlug",
	}

	for route, want := range cases {
		if got := SlugRouteFormater(route); got != want {
			t.Errorf("SlugRouteFormater(%q) = %q, want %q", route, got, want)
		}
	}
}
