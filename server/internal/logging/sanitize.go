package logging

import (
	"encoding/json"
	"regexp"
	"strings"
)

// MaxLoggedFieldBytes caps any single string field written to the log to
// prevent log-blowup attacks via oversized payloads.
const MaxLoggedFieldBytes = 4096

// MaxLoggedJSONBytes caps the total serialized size of a log entry.
const MaxLoggedJSONBytes = 16384

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

// StripANSI removes ANSI escape sequences from s.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// SanitizeForLog strips ANSI sequences and replaces control characters
// (CR/LF/TAB and other C0 codes) with single spaces. This blocks log
// injection where attacker-controlled input contains newlines that
// would otherwise forge new log lines.
func SanitizeForLog(s string) string {
	s = StripANSI(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return truncate(b.String(), MaxLoggedFieldBytes)
}

// SanitizeMap walks a map and sanitizes any string values in place,
// recursing into nested maps and slices. Non-string scalars are
// passed through unchanged.
func SanitizeMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[SanitizeForLog(k)] = sanitizeValue(v)
	}
	return out
}

func sanitizeValue(v interface{}) interface{} {
	switch x := v.(type) {
	case string:
		return SanitizeForLog(x)
	case map[string]interface{}:
		return SanitizeMap(x)
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = sanitizeValue(item)
		}
		return out
	default:
		return v
	}
}

// ValidateJSONForLog confirms that data can be marshaled to JSON within
// the size limit. Returns the marshaled bytes (truncated marker appended
// if oversized) or an error if marshaling fails entirely.
func ValidateJSONForLog(data interface{}) ([]byte, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxLoggedJSONBytes {
		return append(b[:MaxLoggedJSONBytes], []byte(`..."truncated":true}`)...), nil
	}
	return b, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
