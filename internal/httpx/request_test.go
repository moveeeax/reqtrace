package httpx

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildRequestDefaultsToGET(t *testing.T) {
	req, err := BuildRequest(RequestSpec{URL: "http://example.com/path"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", req.Method)
	}
	if req.URL.Host != "example.com" {
		t.Errorf("host = %q", req.URL.Host)
	}
}

func TestBuildRequestAppliesHeaders(t *testing.T) {
	headers, _ := ParseHeaders([]string{"Accept: text/plain", "X-A: 1", "X-A: 2"})
	req, err := BuildRequest(RequestSpec{
		Method:  "POST",
		URL:     "https://example.com/",
		Headers: headers,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("method = %q", req.Method)
	}
	if req.Header.Get("Accept") != "text/plain" {
		t.Errorf("Accept = %q", req.Header.Get("Accept"))
	}
	if v := req.Header.Values("X-A"); len(v) != 2 {
		t.Errorf("X-A values = %v, want two", v)
	}
}

func TestBuildRequestBody(t *testing.T) {
	req, err := BuildRequest(RequestSpec{
		Method:  "PUT",
		URL:     "http://example.com/",
		Body:    []byte("payload"),
		HasBody: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body == nil {
		t.Fatal("expected non-nil body")
	}
	data, _ := io.ReadAll(req.Body)
	if string(data) != "payload" {
		t.Errorf("body = %q", data)
	}
	if req.ContentLength != int64(len("payload")) {
		t.Errorf("content length = %d, want %d", req.ContentLength, len("payload"))
	}
}

func TestBuildRequestNoBody(t *testing.T) {
	req, err := BuildRequest(RequestSpec{URL: "http://example.com/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Body != nil {
		t.Errorf("expected nil body, got %v", req.Body)
	}
}

func TestBuildRequestHonoursHostHeader(t *testing.T) {
	headers, _ := ParseHeaders([]string{"Host: virtual.example"})
	req, err := BuildRequest(RequestSpec{URL: "http://192.0.2.10/", Headers: headers})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// net/http writes Request.Host and ignores any Host entry in the header
	// map, so this must land on the field or it is silently dropped.
	if req.Host != "virtual.example" {
		t.Errorf("req.Host = %q, want virtual.example", req.Host)
	}
	if req.URL.Host != "192.0.2.10" {
		t.Errorf("URL host = %q, want the dialled address to be unchanged", req.URL.Host)
	}
}

func TestBuildRequestWithoutHostHeaderUsesURL(t *testing.T) {
	req, err := BuildRequest(RequestSpec{URL: "http://example.com/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Host != "example.com" {
		t.Errorf("req.Host = %q, want the URL host when no Host header is given", req.Host)
	}
}

func TestValidateURLErrorsDoNotEchoCredentials(t *testing.T) {
	cases := []string{
		"http://user:hunter2@\x7f/",
		"http:// user:hunter2@example.com/",
		"http://user:hunter2@",
	}
	for _, raw := range cases {
		_, err := BuildRequest(RequestSpec{URL: raw})
		if err == nil {
			t.Errorf("expected an error for %q", raw)
			continue
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("error message leaked the URL password: %v", err)
		}
	}
}

func TestBuildRequestInvalidURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"scheme", "ftp://example.com"},
		{"relative", "/just/a/path"},
		{"no host", "http://"},
		{"garbage", "http://%zz"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := BuildRequest(RequestSpec{URL: c.url}); err == nil {
				t.Errorf("expected error for %q", c.url)
			}
		})
	}
}
