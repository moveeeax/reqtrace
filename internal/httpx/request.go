package httpx

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RequestSpec describes everything needed to construct an outbound request.
type RequestSpec struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
	HasBody bool
}

// BuildRequest validates the spec and constructs an *http.Request. The URL must
// be absolute and use the http or https scheme.
func BuildRequest(spec RequestSpec) (*http.Request, error) {
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}

	if err := validateURL(spec.URL); err != nil {
		return nil, err
	}

	var body io.Reader
	if spec.HasBody {
		body = bytes.NewReader(spec.Body)
	}

	req, err := http.NewRequest(method, spec.URL, body)
	if err != nil {
		return nil, err
	}

	for name, values := range spec.Headers {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}

	return req, nil
}

// validateURL rejects inputs that are not absolute http(s) URLs.
func validateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported URL scheme %q: want http or https", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid URL %q: missing host", raw)
	}
	return nil
}
