package version

import "strings"

var value = "dev"

func Value() string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "dev"
	}
	return trimmed
}
