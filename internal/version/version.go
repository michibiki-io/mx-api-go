package version

import "strings"

var value = "dev"
var commit = "unknown"

func Value() string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "dev"
	}
	return trimmed
}

func Commit() string {
	trimmed := strings.TrimSpace(commit)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func ShortCommit() string {
	trimmed := Commit()
	if trimmed == "unknown" || len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}
