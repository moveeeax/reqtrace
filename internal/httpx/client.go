package httpx

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"
)

// MaxRedirects bounds how many redirects are chased before giving up. It
// matches net/http's own default policy, which a custom CheckRedirect would
// otherwise replace with "unlimited".
const MaxRedirects = 10

// credentialHeaders are request headers that carry caller credentials and must
// never be replayed onto a less trustworthy hop.
var credentialHeaders = []string{
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Cookie2",
}

// ClientOptions configures the HTTP client used for a probe.
type ClientOptions struct {
	Timeout        time.Duration
	Insecure       bool
	FollowRedirect bool
}

// NewClient builds an *http.Client honouring the timeout, TLS verification, and
// redirect policy described by opts. When FollowRedirect is false the client
// stops at the first response instead of chasing Location headers.
//
// The transport deliberately restores http.DefaultTransport's proxy and HTTP/2
// behaviour: a hand-built Transport gets neither by default, and one carrying a
// TLSClientConfig has automatic HTTP/2 upgrades disabled outright, which would
// make every HTTPS probe misreport its protocol as HTTP/1.1.
func NewClient(opts ClientOptions) *http.Client {
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: opts.Insecure,
			MinVersion:         tls.VersionTLS12,
		},
	}
	client := &http.Client{
		Timeout:   opts.Timeout,
		Transport: transport,
	}
	if !opts.FollowRedirect {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		return client
	}
	client.CheckRedirect = checkRedirect
	return client
}

// checkRedirect caps the redirect chain and strips credential headers when a
// hop downgrades the transport.
//
// net/http already drops Authorization and Cookie when a redirect crosses to a
// different domain, but it compares hostnames only. An https -> http redirect
// back to the same host therefore keeps the caller's bearer token or session
// cookie and puts it on the wire in cleartext; strip them instead.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= MaxRedirects {
		return fmt.Errorf("stopped after %d redirects", MaxRedirects)
	}
	if isDowngrade(via, req) {
		for _, name := range credentialHeaders {
			req.Header.Del(name)
		}
	}
	return nil
}

// isDowngrade reports whether req leaves TLS behind after an earlier hop used
// it.
func isDowngrade(via []*http.Request, req *http.Request) bool {
	if req == nil || req.URL == nil || req.URL.Scheme == "https" {
		return false
	}
	for _, prev := range via {
		if prev != nil && prev.URL != nil && prev.URL.Scheme == "https" {
			return true
		}
	}
	return false
}
