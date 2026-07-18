package httpx

import (
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
	if c.CheckRedirect != nil {
		t.Error("expected default redirect policy when following")
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
