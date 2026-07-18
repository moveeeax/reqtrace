package httpx

import (
	"crypto/tls"
	"net/http"
	"time"
)

// ClientOptions configures the HTTP client used for a probe.
type ClientOptions struct {
	Timeout        time.Duration
	Insecure       bool
	FollowRedirect bool
}

// NewClient builds an *http.Client honouring the timeout, TLS verification, and
// redirect policy described by opts. When FollowRedirect is false the client
// stops at the first response instead of chasing Location headers.
func NewClient(opts ClientOptions) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: opts.Insecure},
	}
	client := &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
	if !opts.FollowRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}
