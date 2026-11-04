package evidence

import (
	"fmt"
	"time"
)

// Query parameters arrive as decoded JSON, so numbers are float64.

func paramString(p map[string]any, key string) string {
	s, _ := p[key].(string)
	return s
}

func paramInt(p map[string]any, key string, def int) int {
	switch v := p[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func paramFloat(p map[string]any, key string) float64 {
	switch v := p[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// paramTime parses an optional RFC3339 parameter; malformed values are an error.
func paramTime(p map[string]any, key string) (time.Time, error) {
	s := paramString(p, key)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, invalid("%s must be an RFC3339 timestamp", key)
	}
	return t, nil
}

// capLimit applies def when unset and caps at maxLimit with a warning.
func capLimit(p map[string]any, def, maxLimit int, warnings *[]string) int {
	limit := paramInt(p, "limit", def)
	if limit <= 0 {
		return def
	}
	if limit > maxLimit {
		*warnings = append(*warnings, fmt.Sprintf("Limit capped at maximum allowed: %d", maxLimit))
		return maxLimit
	}
	return limit
}
