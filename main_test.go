package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// exec runs the CLI with the given args and returns exit code and captured out.
func exec(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRunAgainstHTTPServer(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello world")
	}))
	defer ts.Close()

	code, out, errOut := exec(ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	for _, want := range []string{"GET " + ts.URL, "Status:", "TCP connect:", "Total:", "Body:", "11 bytes"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "n/a") == false {
		t.Errorf("plain HTTP should mark TLS as n/a\n%s", out)
	}
}

func TestRunJSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		io.WriteString(w, "created")
	}))
	defer ts.Close()

	code, out, errOut := exec("-json", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}

	var got struct {
		URL       string `json:"url"`
		Method    string `json:"method"`
		Status    int    `json:"status"`
		TLS       bool   `json:"tls"`
		TimingsMS struct {
			Total float64 `json:"total"`
		} `json:"timings_ms"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Status != 201 {
		t.Errorf("status = %d, want 201", got.Status)
	}
	if got.Method != "GET" || got.TLS {
		t.Errorf("unexpected report: %+v", got)
	}
	if got.TimingsMS.Total < 0 {
		t.Errorf("total should be non-negative, got %v", got.TimingsMS.Total)
	}
}

func TestRunTLSInsecure(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secure")
	}))
	defer ts.Close()

	code, out, errOut := exec("-json", "-insecure", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		TLS       bool `json:"tls"`
		TimingsMS struct {
			TLSHandshake float64 `json:"tls_handshake"`
		} `json:"timings_ms"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !got.TLS {
		t.Error("expected tls=true for HTTPS server")
	}
}

func TestRunTLSWithoutInsecureFails(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	code, _, errOut := exec(ts.URL)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (cert should not verify)", code)
	}
	if !strings.Contains(errOut, "reqtrace:") {
		t.Errorf("expected error message, got %q", errOut)
	}
}

func redirectServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dest", http.StatusFound)
	})
	mux.HandleFunc("/dest", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "arrived")
	})
	return httptest.NewServer(mux)
}

func TestRunRedirectNotFollowedByDefault(t *testing.T) {
	ts := redirectServer(t)
	defer ts.Close()

	code, out, errOut := exec("-json", ts.URL+"/redirect")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		Status int `json:"status"`
	}
	json.Unmarshal([]byte(out), &got)
	if got.Status != http.StatusFound {
		t.Errorf("status = %d, want 302 without -follow", got.Status)
	}
}

func TestRunRedirectFollowed(t *testing.T) {
	ts := redirectServer(t)
	defer ts.Close()

	code, out, errOut := exec("-json", "-follow", ts.URL+"/redirect")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		Status int `json:"status"`
	}
	json.Unmarshal([]byte(out), &got)
	if got.Status != http.StatusOK {
		t.Errorf("status = %d, want 200 with -follow", got.Status)
	}
}

func TestRunSendsBody(t *testing.T) {
	var gotBody string
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotMethod = r.Method
	}))
	defer ts.Close()

	code, _, errOut := exec("-method", "POST", "-body", "ping", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if gotMethod != "POST" {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotBody != "ping" {
		t.Errorf("body = %q, want ping", gotBody)
	}
}

func TestRunSendsHeader(t *testing.T) {
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Trace")
	}))
	defer ts.Close()

	code, _, _ := exec("-H", "X-Trace: yes", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if gotHeader != "yes" {
		t.Errorf("X-Trace = %q, want yes", gotHeader)
	}
}

func TestRunBadURL(t *testing.T) {
	code, _, errOut := exec("://not-a-url")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "reqtrace:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		io.WriteString(w, "slow")
	}))
	defer ts.Close()

	code, _, errOut := exec("-timeout", "20ms", ts.URL)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 on timeout", code)
	}
	if !strings.Contains(errOut, "reqtrace:") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunMissingURL(t *testing.T) {
	code, _, _ := exec()
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRunTooManyArgs(t *testing.T) {
	code, _, _ := exec("http://a.example", "http://b.example")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	code, _, _ := exec("-nope", "http://a.example")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestRunInvalidHeader(t *testing.T) {
	code, _, errOut := exec("-H", "bogus", "http://a.example")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "reqtrace:") {
		t.Errorf("stderr = %q", errOut)
	}
}
