package audit

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

func EncodeCursor(cursor Cursor) (string, error) {
	if cursor.ID <= 0 || cursor.CreatedAt.IsZero() {
		return "", ErrInvalidCursor
	}
	payload := struct {
		CreatedAt string `json:"created_at"`
		ID        int64  `json:"id"`
	}{
		CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:        cursor.ID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(value string) (Cursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Cursor{}, ErrInvalidCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	var payload struct {
		CreatedAt string `json:"created_at"`
		ID        int64  `json:"id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, payload.CreatedAt)
	if err != nil || payload.ID <= 0 || createdAt.IsZero() {
		return Cursor{}, ErrInvalidCursor
	}
	return Cursor{CreatedAt: createdAt.UTC(), ID: payload.ID}, nil
}
