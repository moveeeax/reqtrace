package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestRunFollowReportsFinalURL(t *testing.T) {
	ts := redirectServer(t)
	defer ts.Close()

	_, out, _ := exec("-json", "-follow", ts.URL+"/redirect")
	var got struct {
		URL string `json:"url"`
	}
	json.Unmarshal([]byte(out), &got)
	if !strings.HasSuffix(got.URL, "/dest") {
		t.Errorf("url = %q, want final destination ending in /dest", got.URL)
	}
}

func TestRunNoFollowReportsRequestedURL(t *testing.T) {
	ts := redirectServer(t)
	defer ts.Close()

	_, out, _ := exec("-json", ts.URL+"/redirect")
	var got struct {
		URL string `json:"url"`
	}
	json.Unmarshal([]byte(out), &got)
	if !strings.HasSuffix(got.URL, "/redirect") {
		t.Errorf("url = %q, want requested URL ending in /redirect", got.URL)
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

// TestRunRedactsURLPassword guards the headline privacy property: reqtrace
// output is routinely pasted into tickets and captured in CI logs, so a
// userinfo password must never appear in it.
func TestRunRedactsURLPassword(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
	}))
	defer ts.Close()

	withCreds := strings.Replace(ts.URL, "http://", "http://alice:hunter2@", 1)

	for _, args := range [][]string{{withCreds}, {"-json", withCreds}} {
		code, out, errOut := exec(args...)
		if code != 0 {
			t.Fatalf("exit = %d, stderr=%q", code, errOut)
		}
		if strings.Contains(out, "hunter2") || strings.Contains(errOut, "hunter2") {
			t.Errorf("args %v leaked the password:\nstdout=%s\nstderr=%s", args, out, errOut)
		}
		if !strings.Contains(out, "alice") {
			t.Errorf("args %v dropped the username; only the password should be masked:\n%s", args, out)
		}
	}
	// The credentials must still be sent, only hidden from the report.
	if gotAuth == "" {
		t.Error("expected basic auth to be sent to the server")
	}
}

func TestRunRedactsPasswordOnRedirect(t *testing.T) {
	ts := redirectServer(t)
	defer ts.Close()

	withCreds := strings.Replace(ts.URL, "http://", "http://alice:hunter2@", 1) + "/redirect"
	code, out, errOut := exec("-json", "-follow", withCreds)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("final URL leaked the password:\n%s", out)
	}
}

func TestRunInsecureWarnsLoudly(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	code, _, errOut := exec("-insecure", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if !strings.Contains(errOut, "warning") || !strings.Contains(errOut, "certificate verification") {
		t.Errorf("expected a TLS warning on stderr, got %q", errOut)
	}
}

func TestRunSecureRequestIsQuiet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	code, _, errOut := exec(ts.URL)
	if code != 0 || errOut != "" {
		t.Errorf("exit = %d, stderr = %q, want a clean run", code, errOut)
	}
}

func TestRunRejectsNegativeTimeout(t *testing.T) {
	// http.Client reads any non-positive Timeout as "no timeout", so a negative
	// value must be refused rather than silently removing the bound.
	code, _, errOut := exec("-timeout", "-1s", "http://192.0.2.1/")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errOut, "must not be negative") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestRunSendsHostHeader(t *testing.T) {
	var gotHost string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
	}))
	defer ts.Close()

	code, _, errOut := exec("-H", "Host: virtual.example", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if gotHost != "virtual.example" {
		t.Errorf("server saw Host %q, want virtual.example", gotHost)
	}
}

func TestRunReportsHTTP2(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	code, out, errOut := exec("-json", "-insecure", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		Proto string `json:"proto"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	// A hand-built Transport with a TLSClientConfig gets no HTTP/2 unless
	// ForceAttemptHTTP2 is set, which would misreport this as HTTP/1.1.
	if got.Proto != "HTTP/2.0" {
		t.Errorf("proto = %q, want HTTP/2.0", got.Proto)
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

type statsJSON struct {
	Runs      int `json:"runs"`
	Reused    int `json:"reused_connections"`
	Resumed   int `json:"tls_resumed"`
	TimingsMS map[string]struct {
		Min, P50, P95, Max float64
	} `json:"timings_ms"`
}

func runStats(t *testing.T, args ...string) statsJSON {
	t.Helper()
	code, out, errOut := exec(append([]string{"-json"}, args...)...)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		Stats *statsJSON `json:"stats"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Stats == nil {
		t.Fatalf("missing stats object:\n%s", out)
	}
	return *got.Stats
}

// connCounter is a test server that records which client connections served
// its requests.
func connCounter(t *testing.T, tls bool, closeConn bool) (*httptest.Server, func() int) {
	t.Helper()
	var mu sync.Mutex
	remotes := map[string]bool{}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		remotes[r.RemoteAddr] = true
		mu.Unlock()
		if closeConn {
			w.Header().Set("Connection", "close")
		}
		io.WriteString(w, "ok")
	}))
	if tls {
		ts.StartTLS()
	} else {
		ts.Start()
	}
	t.Cleanup(ts.Close)
	return ts, func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(remotes)
	}
}

func TestRunRepeatFreshConnections(t *testing.T) {
	ts, conns := connCounter(t, false, false)

	s := runStats(t, "-n", "4", ts.URL)
	if s.Runs != 4 || s.Reused != 0 {
		t.Errorf("runs = %d, reused = %d, want 4 and 0", s.Runs, s.Reused)
	}
	if got := conns(); got != 4 {
		t.Errorf("server saw %d connections, want one per run without -reuse", got)
	}
	for _, phase := range []string{"dns_lookup", "tcp_connect", "tls_handshake", "time_to_first_byte", "total"} {
		p, ok := s.TimingsMS[phase]
		if !ok {
			t.Errorf("stats missing phase %q", phase)
			continue
		}
		if !(p.Min <= p.P50 && p.P50 <= p.P95 && p.P95 <= p.Max) {
			t.Errorf("%s not ordered: %+v", phase, p)
		}
	}
	if s.TimingsMS["tcp_connect"].Min <= 0 {
		t.Error("every fresh run must pay for a TCP connect")
	}
}

func TestRunRepeatReuse(t *testing.T) {
	ts, conns := connCounter(t, false, false)

	s := runStats(t, "-n", "4", "-reuse", ts.URL)
	if s.Reused != 3 {
		t.Errorf("reused = %d, want 3 (all runs after the first)", s.Reused)
	}
	if got := conns(); got != 1 {
		t.Errorf("server saw %d connections, want 1 with -reuse", got)
	}
	// Three of four runs skipped the connect, so the median is zero.
	if p50 := s.TimingsMS["tcp_connect"].P50; p50 != 0 {
		t.Errorf("tcp_connect p50 = %v, want 0 with warm connections", p50)
	}
}

func TestRunRepeatReuseResumesTLS(t *testing.T) {
	// The server closes every connection, so each run needs a new handshake;
	// with -reuse the client resumes the session from the first one.
	ts, conns := connCounter(t, true, true)

	s := runStats(t, "-n", "3", "-reuse", "-insecure", ts.URL)
	if got := conns(); got != 3 {
		t.Fatalf("server saw %d connections, want 3 (it closes each one)", got)
	}
	if s.Resumed != 2 {
		t.Errorf("tls_resumed = %d, want 2", s.Resumed)
	}

	s = runStats(t, "-n", "3", "-insecure", ts.URL)
	if s.Resumed != 0 {
		t.Errorf("without -reuse tls_resumed = %d, want 0 (cold handshakes)", s.Resumed)
	}
}

func TestRunRepeatInterval(t *testing.T) {
	ts, _ := connCounter(t, false, false)

	start := time.Now()
	runStats(t, "-n", "3", "-interval", "30ms", ts.URL)
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("3 runs 30ms apart took %v, want at least 60ms", elapsed)
	}
}

func TestRunRepeatText(t *testing.T) {
	ts, _ := connCounter(t, false, false)

	code, out, errOut := exec("-n", "3", ts.URL)
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	for _, want := range []string{"Runs:                3 (0 on a reused connection", "min", "p50", "p95", "max", "Total:", "Body:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "TLS handshake:") || !strings.Contains(out, "n/a") {
		t.Errorf("plain HTTP should mark TLS as n/a\n%s", out)
	}
}

func TestRunSingleHasNoStats(t *testing.T) {
	ts, _ := connCounter(t, false, false)
	_, out, _ := exec("-json", ts.URL)
	if strings.Contains(out, `"stats"`) {
		t.Errorf("a single run must not print stats:\n%s", out)
	}
}

func TestRunRepeatStopsOnError(t *testing.T) {
	var n int
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		slow := n == 2
		mu.Unlock()
		if slow {
			time.Sleep(200 * time.Millisecond)
		}
	}))
	defer ts.Close()

	code, _, errOut := exec("-n", "5", "-timeout", "50ms", ts.URL)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errOut, "run 2 of 5") {
		t.Errorf("stderr = %q, want the failing run named", errOut)
	}
}

func TestRunRepeatRejectsBadValues(t *testing.T) {
	for _, args := range [][]string{
		{"-n", "0", "http://a.example"},
		{"-n", "-3", "http://a.example"},
		{"-n", "2", "-interval", "-1s", "http://a.example"},
	} {
		if code, _, _ := exec(args...); code != 2 {
			t.Errorf("args %v: exit = %d, want 2", args, code)
		}
	}
}

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proxy.env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunExplainProxyText(t *testing.T) {
	path := writeEnvFile(t, "HTTPS_PROXY=http://bob:hunter2@proxy.internal:3128\nNO_PROXY=localhost,.example.com\n")

	code, out, errOut := exec("-explain-proxy", "-env-file", path, "https://api.example.com/v2/")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	for _, want := range []string{
		"Proxy for https://api.example.com/v2/",
		"HTTPS_PROXY:",
		"<- used",
		".example.com",
		"subdomain of example.com",
		`Verdict:             direct: NO_PROXY entry ".example.com" matches`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("explanation leaked the proxy password:\n%s", out)
	}
}

func TestRunExplainProxyJSON(t *testing.T) {
	path := writeEnvFile(t, "HTTPS_PROXY=proxy.internal:3128\n")

	code, out, errOut := exec("-explain-proxy", "-json", "-env-file", path, "https://example.com/")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	var got struct {
		Setting string `json:"setting"`
		Proxy   string `json:"proxy"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Setting != "HTTPS_PROXY" || got.Proxy != "http://proxy.internal:3128" {
		t.Errorf("got %+v", got)
	}
}

func TestRunExplainProxySendsNoRequest(t *testing.T) {
	hit := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer ts.Close()

	if code, _, errOut := exec("-explain-proxy", ts.URL); code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if hit {
		t.Error("-explain-proxy must not send the request")
	}
}

func TestRunExplainProxyBadInput(t *testing.T) {
	if code, _, _ := exec("-explain-proxy", "ftp://example.com/"); code != 1 {
		t.Errorf("bad URL: exit = %d, want 1", code)
	}
	if code, _, _ := exec("-explain-proxy", "-env-file", filepath.Join(t.TempDir(), "missing.env"), "https://example.com/"); code != 1 {
		t.Errorf("missing env file: exit = %d, want 1", code)
	}
}

// TestRunEnvFileRoutesLiveRequest checks that -env-file drives the real
// request too, and that the report names the proxy it went through.
func TestRunEnvFileRoutesLiveRequest(t *testing.T) {
	var gotURL string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String() // a forward proxy sees the absolute URL
		io.WriteString(w, "via proxy")
	}))
	defer proxySrv.Close()

	proxyWithCreds := strings.Replace(proxySrv.URL, "http://", "http://bob:hunter2@", 1)
	path := writeEnvFile(t, "HTTP_PROXY="+proxyWithCreds+"\n")

	code, out, errOut := exec("-env-file", path, "http://backend.example/health")
	if code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, errOut)
	}
	if gotURL != "http://backend.example/health" {
		t.Errorf("proxy saw %q, want the absolute target URL", gotURL)
	}
	if !strings.Contains(out, "Proxy:               http://bob:xxxxx@127.0.0.1") {
		t.Errorf("report should name the proxy with the password masked:\n%s", out)
	}
	if strings.Contains(out, "hunter2") {
		t.Errorf("report leaked the proxy password:\n%s", out)
	}
}

func TestRunReportsDirectConnection(t *testing.T) {
	ts, _ := connCounter(t, false, false)
	_, out, _ := exec(ts.URL)
	if !strings.Contains(out, "Proxy:               direct") {
		t.Errorf("expected a direct connection in the report:\n%s", out)
	}
}
