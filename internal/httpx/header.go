package httpx

import (
	"fmt"
	"net/http"
	"strings"
)

// ParseHeaders converts a slice of "Key: Value" strings, as collected from
// repeatable -H flags, into an http.Header. Whitespace around the name and
// value is trimmed. Repeated names accumulate, mirroring curl's behaviour.
func ParseHeaders(raw []string) (http.Header, error) {
	h := make(http.Header)
	for _, item := range raw {
		name, value, err := splitHeader(item)
		if err != nil {
			return nil, err
		}
		h.Add(name, value)
	}
	return h, nil
}

// splitHeader parses a single header specification.
func splitHeader(item string) (name, value string, err error) {
	idx := strings.IndexByte(item, ':')
	if idx < 0 {
		return "", "", fmt.Errorf("invalid header %q: missing ':'", item)
	}
	name = strings.TrimSpace(item[:idx])
	value = strings.TrimSpace(item[idx+1:])
	if name == "" {
		return "", "", fmt.Errorf("invalid header %q: empty field name", item)
	}
	return name, value, nil
}
