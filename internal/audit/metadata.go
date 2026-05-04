package audit

import (
	"fmt"
	"strings"
)

func SafeMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if isSensitiveKey(key) {
			continue
		}
		switch v := value.(type) {
		case string:
			out[key] = truncate(v, 256)
		case fmt.Stringer:
			out[key] = truncate(v.String(), 256)
		case int, int8, int16, int32, int64:
			out[key] = v
		case uint, uint8, uint16, uint32, uint64:
			out[key] = v
		case float32, float64, bool, nil:
			out[key] = v
		default:
			out[key] = truncate(fmt.Sprint(v), 256)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	sensitiveParts := []string{"password", "passwd", "secret", "token", "authorization", "cookie", "set-cookie", "body", "payload", "message", "smtp", "api_key", "apikey"}
	for _, part := range sensitiveParts {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}
