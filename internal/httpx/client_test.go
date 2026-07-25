package httpx

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"
)

func TestNewClientTimeout(t *testing.T) {
	c := NewClient(ClientOptions{Timeout: 5 * time.Second})
	if c.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", c.Timeout)
	}
}

func TestNewClientNoFollowStopsRedirects(t *testing.T) {
	c := NewClient(ClientOptions{FollowRedirect: false})
	if c.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set when not following")
	}
	if err := c.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Errorf("CheckRedirect = %v, want ErrUseLastResponse", err)
	}
}

func TestNewClientFollowAllowsRedirects(t *testing.T) {
	c := NewClient(ClientOptions{FollowRedirect: true})
	if c.CheckRedirect == nil {
		t.Fatal("expected a redirect policy when following")
	}
	req := mustReq(t, "https://example.com/next")
	if err := c.CheckRedirect(req, viaChain(t, "https://example.com/")); err != nil {
		t.Errorf("CheckRedirect = %v, want nil for an ordinary hop", err)
	}
}

func TestNewClientTransportKeepsDefaultCapabilities(t *testing.T) {
	tr := NewClient(ClientOptions{}).Transport.(*http.Transport)
	if tr.Proxy == nil {
		t.Error("transport must honour HTTP_PROXY/HTTPS_PROXY like http.DefaultTransport")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("a TLSClientConfig disables automatic HTTP/2 unless ForceAttemptHTTP2 is set")
	}
	if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want at least TLS 1.2", tr.TLSClientConfig.MinVersion)
	}
}

// mustReq builds a redirect target carrying credential headers.
func mustReq(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer topsecret")
	req.Header.Set("Cookie", "session=topsecret")
	req.Header.Set("X-Trace", "keep-me")
	return req
}

// viaChain builds the preceding-request chain CheckRedirect is handed.
func viaChain(t *testing.T, urls ...string) []*http.Request {
	t.Helper()
	var via []*http.Request
	for _, u := range urls {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		via = append(via, req)
	}
	return via
}

func TestCheckRedirectStripsCredentialsOnSchemeDowngrade(t *testing.T) {
	req := mustReq(t, "http://example.com/downgraded")
	if err := checkRedirect(req, viaChain(t, "https://example.com/")); err != nil {
		t.Fatalf("checkRedirect: %v", err)
	}
	for _, name := range []string{"Authorization", "Cookie"} {
		if got := req.Header.Get(name); got != "" {
			t.Errorf("%s survived an https->http redirect: %q", name, got)
		}
	}
	if req.Header.Get("X-Trace") != "keep-me" {
		t.Error("non-credential headers must be preserved")
	}
}

func TestCheckRedirectStripsCredentialsAfterEarlierTLSHop(t *testing.T) {
	// https -> https -> http: the downgrade is not against the immediately
	// preceding hop, but the token still reached TLS-protected hosts first.
	req := mustReq(t, "http://example.com/c")
	if err := checkRedirect(req, viaChain(t, "https://example.com/a", "https://example.com/b")); err != nil {
		t.Fatalf("checkRedirect: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Error("Authorization survived a downgrade later in the chain")
	}
}

func TestCheckRedirectKeepsCredentialsWithinTLS(t *testing.T) {
	req := mustReq(t, "https://example.com/b")
	if err := checkRedirect(req, viaChain(t, "https://example.com/a")); err != nil {
		t.Fatalf("checkRedirect: %v", err)
	}
	if req.Header.Get("Authorization") == "" {
		t.Error("https->https must not strip Authorization")
	}
}

func TestCheckRedirectKeepsCredentialsOnPlainHTTP(t *testing.T) {
	req := mustReq(t, "http://example.com/b")
	if err := checkRedirect(req, viaChain(t, "http://example.com/a")); err != nil {
		t.Fatalf("checkRedirect: %v", err)
	}
	if req.Header.Get("Authorization") == "" {
		t.Error("http->http is not a downgrade; headers must be left alone")
	}
}

func TestCheckRedirectStopsAtLimit(t *testing.T) {
	urls := make([]string, MaxRedirects)
	for i := range urls {
		urls[i] = "https://example.com/"
	}
	req := mustReq(t, "https://example.com/next")
	if err := checkRedirect(req, viaChain(t, urls...)); err == nil {
		t.Fatalf("expected an error after %d redirects", MaxRedirects)
	}
	if err := checkRedirect(req, viaChain(t, urls[:MaxRedirects-1]...)); err != nil {
		t.Errorf("redirect %d should still be allowed: %v", MaxRedirects-1, err)
	}
}

func TestNewClientInsecureConfig(t *testing.T) {
	c := NewClient(ClientOptions{Insecure: true})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", c.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}
}

func TestNewClientSecureByDefault(t *testing.T) {
	c := NewClient(ClientOptions{})
	tr := c.Transport.(*http.Transport)
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected certificate verification by default")
	}
}
