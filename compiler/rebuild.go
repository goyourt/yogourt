package compiler

import (
	"errors"
	"regexp"
	"strings"
)

func IsStalePluginVersionError(err error) bool {
	if err == nil {
		return false
	}

	candidates := []string{
		"plugin was built with a different version of package",
		"built with a previous version of package",
		"built with a previous version of the package",
		"build with a previous version of the package",
	}

	for e := err; e != nil; e = errors.Unwrap(e) {
		msg := strings.ToLower(e.Error())
		for _, candidate := range candidates {
			if strings.Contains(msg, candidate) {
				return true
			}
		}
	}

	msg := strings.ToLower(err.Error())
	for _, candidate := range candidates {
		if strings.Contains(msg, candidate) {
			return true
		}
	}

	return false
}

var pluginOpenPathRegex = regexp.MustCompile(`plugin\.Open\("([^"]+)"\)`)

func ExtractPluginPath(err error) string {
	if err == nil {
		return ""
	}

	for e := err; e != nil; e = errors.Unwrap(e) {
		matches := pluginOpenPathRegex.FindStringSubmatch(e.Error())
		if len(matches) == 2 {
			return matches[1]
		}
	}

	matches := pluginOpenPathRegex.FindStringSubmatch(err.Error())
	if len(matches) == 2 {
		return matches[1]
	}

	return ""
}
