package httpx

import (
	"fmt"
	"os"
	"strings"
)

// ParseBody resolves a -body specification into the raw request payload.
//
//	""          -> no body
//	"@path"     -> contents of the file at path
//	anything    -> the literal string bytes
//
// The returned bool reports whether a body was supplied at all, so callers can
// distinguish an explicit empty body from no body.
func ParseBody(spec string) ([]byte, bool, error) {
	if spec == "" {
		return nil, false, nil
	}
	if strings.HasPrefix(spec, "@") {
		path := spec[1:]
		if path == "" {
			return nil, false, fmt.Errorf("empty file path after '@'")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false, fmt.Errorf("read body file: %w", err)
		}
		return data, true, nil
	}
	return []byte(spec), true, nil
}
