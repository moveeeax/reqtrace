package httpx

import (
	"bytes"
	"errors"
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

	// net/http writes Request.Host and excludes any "Host" entry in the header
	// map, so a user-supplied -H "Host: ..." would otherwise be dropped without
	// a word. Honour it the way curl --header does.
	if host := spec.Headers.Get("Host"); host != "" {
		req.Host = host
	}

	return req, nil
}

// validateURL rejects inputs that are not absolute http(s) URLs.
//
// Error messages never echo the raw input: a URL may carry userinfo
// credentials, and these messages routinely land in CI logs.
func validateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		// *url.Error stringifies with the raw input embedded, so unwrap it and
		// report only the reason.
		var perr *url.Error
		if errors.As(err, &perr) {
			return fmt.Errorf("invalid URL: %v", perr.Err)
		}
		return fmt.Errorf("invalid URL: %v", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("unsupported URL scheme %q: want http or https", u.Scheme)
	}
	if u.Host == "" {
		// Redacted keeps any userinfo password out of the message, which may
		// well end up in a CI log.
		return fmt.Errorf("invalid URL %q: missing host", u.Redacted())
	}
	return nil
}
