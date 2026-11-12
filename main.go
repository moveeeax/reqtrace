package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/moveeeax/reqtrace/internal/httpx"
	"github.com/moveeeax/reqtrace/internal/proxy"
	"github.com/moveeeax/reqtrace/internal/trace"
)

// headerList collects repeated -H flags.
type headerList []string

func (h *headerList) String() string { return strings.Join(*h, ", ") }

func (h *headerList) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// options holds parsed command-line settings.
type options struct {
	method   string
	headers  headerList
	timeout  time.Duration
	jsonOut  bool
	insecure bool
	follow   bool
	body     string

	count    int
	interval time.Duration
	reuse    bool

	explainProxy bool
	envFile      string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses arguments, performs the traced request, and renders the report.
// It returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reqtrace", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var opt options
	fs.StringVar(&opt.method, "method", "GET", "HTTP method to use")
	fs.Var(&opt.headers, "H", "request header 'Key: Value' (repeatable)")
	fs.DurationVar(&opt.timeout, "timeout", 30*time.Second, "overall request timeout (0 disables it)")
	fs.BoolVar(&opt.jsonOut, "json", false, "emit machine-readable JSON")
	fs.BoolVar(&opt.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&opt.follow, "follow", false, "follow HTTP redirects")
	fs.StringVar(&opt.body, "body", "", "request body: literal string or @file")
	fs.IntVar(&opt.count, "n", 1, "number of requests; more than 1 prints min, p50, p95 and max per phase")
	fs.DurationVar(&opt.interval, "interval", 0, "pause between requests with -n")
	fs.BoolVar(&opt.reuse, "reuse", false, "with -n, keep connections alive and resume TLS sessions between requests")
	fs.BoolVar(&opt.explainProxy, "explain-proxy", false, "explain which proxy setting applies to the URL and why; sends no request")
	fs.StringVar(&opt.envFile, "env-file", "", "read HTTP_PROXY, HTTPS_PROXY, NO_PROXY and friends from this KEY=VALUE file instead of the environment")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: reqtrace [flags] <url>\n\n")
		fmt.Fprintf(stderr, "reqtrace performs an HTTP request and prints per-phase timings.\n\n")
		fmt.Fprintf(stderr, "flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "reqtrace: exactly one URL argument is required")
		fs.Usage()
		return 2
	}

	// http.Client treats any non-positive Timeout as "wait forever", so a
	// negative duration would silently disable the very bound the user asked
	// for. Only an explicit zero may mean that.
	if opt.timeout < 0 {
		fmt.Fprintf(stderr, "reqtrace: -timeout must not be negative (got %s); use 0 to disable the timeout\n", opt.timeout)
		return 2
	}
	if opt.count < 1 {
		fmt.Fprintf(stderr, "reqtrace: -n must be at least 1 (got %d)\n", opt.count)
		return 2
	}
	if opt.interval < 0 {
		fmt.Fprintf(stderr, "reqtrace: -interval must not be negative (got %s)\n", opt.interval)
		return 2
	}

	env := proxy.Environ()
	if opt.envFile != "" {
		var err error
		if env, err = proxy.LoadEnvFile(opt.envFile); err != nil {
			fmt.Fprintf(stderr, "reqtrace: %v\n", err)
			return 1
		}
	}

	if opt.explainProxy {
		return explain(fs.Arg(0), env, opt.jsonOut, stdout, stderr)
	}

	if opt.timeout == 0 {
		fmt.Fprintln(stderr, "reqtrace: warning: -timeout 0 disables the request timeout; a slow or endless response will hang this process")
	}
	if opt.insecure {
		fmt.Fprintln(stderr, "reqtrace: warning: -insecure disables TLS certificate verification; the server is NOT authenticated and the connection can be intercepted")
	}

	var proxyFunc func(*http.Request) (*url.URL, error)
	if opt.envFile != "" {
		proxyFunc = proxy.ProxyFunc(env)
	}

	report, err := probeN(fs.Arg(0), opt, proxyFunc)
	if err != nil {
		fmt.Fprintf(stderr, "reqtrace: %v\n", err)
		return 1
	}

	if opt.jsonOut {
		if err := writeJSON(stdout, report); err != nil {
			fmt.Fprintf(stderr, "reqtrace: %v\n", err)
			return 1
		}
		return 0
	}

	writeText(stdout, report)
	return 0
}

// explain prints which proxy setting applies to rawURL under env, without
// sending a request.
func explain(rawURL string, env proxy.Env, jsonOut bool, stdout, stderr io.Writer) int {
	// BuildRequest applies the same URL validation as a real probe.
	req, err := httpx.BuildRequest(httpx.RequestSpec{URL: rawURL})
	if err != nil {
		fmt.Fprintf(stderr, "reqtrace: %v\n", err)
		return 1
	}
	ex := proxy.Explain(env, req.URL)
	if jsonOut {
		if err := writeExplainJSON(stdout, ex); err != nil {
			fmt.Fprintf(stderr, "reqtrace: %v\n", err)
			return 1
		}
		return 0
	}
	writeExplainText(stdout, ex)
	return 0
}

// probeN runs opt.count probes against rawURL. A single run returns its
// report as is; repeated runs return the last run's report with Stats over
// all of them. The first failed run aborts the series.
func probeN(rawURL string, opt options, proxyFunc func(*http.Request) (*url.URL, error)) (trace.Report, error) {
	clientOpts := httpx.ClientOptions{
		Timeout:        opt.timeout,
		Insecure:       opt.insecure,
		FollowRedirect: opt.follow,
		SessionCache:   opt.reuse && opt.count > 1,
	}
	pr := &proxyRecorder{next: proxyFunc}
	clientOpts.Proxy = pr.proxy

	if opt.count == 1 {
		client := httpx.NewClient(clientOpts)
		defer client.CloseIdleConnections()
		return probe(client, rawURL, opt, pr)
	}

	var (
		client  *http.Client
		last    trace.Report
		results = make([]trace.Result, 0, opt.count)
	)
	for i := 0; i < opt.count; i++ {
		if i > 0 && opt.interval > 0 {
			time.Sleep(opt.interval)
		}
		// Without -reuse every run gets a fresh client, so each one pays for
		// DNS, connect and a full TLS handshake, like separate invocations.
		if client == nil || !opt.reuse {
			if client != nil {
				client.CloseIdleConnections()
			}
			client = httpx.NewClient(clientOpts)
		}
		rep, err := probe(client, rawURL, opt, pr)
		if err != nil {
			client.CloseIdleConnections()
			return trace.Report{}, fmt.Errorf("run %d of %d: %w", i+1, opt.count, err)
		}
		results = append(results, rep.Result)
		last = rep
	}
	client.CloseIdleConnections()

	stats := trace.Aggregate(results)
	last.Stats = &stats
	return last, nil
}

// proxyRecorder wraps a Transport.Proxy function and remembers the proxy it
// returned last, so the report can say which path the request took.
type proxyRecorder struct {
	next func(*http.Request) (*url.URL, error)

	mu   sync.Mutex
	last *url.URL
}

func (p *proxyRecorder) proxy(req *http.Request) (*url.URL, error) {
	next := p.next
	if next == nil {
		next = http.ProxyFromEnvironment
	}
	u, err := next(req)
	p.mu.Lock()
	p.last = u
	p.mu.Unlock()
	return u, err
}

// used returns the last proxy chosen, password masked, or "" for direct.
func (p *proxyRecorder) used() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil {
		return ""
	}
	return p.last.Redacted()
}

// probe issues the request described by opt against rawURL, capturing timings.
func probe(client *http.Client, rawURL string, opt options, pr *proxyRecorder) (trace.Report, error) {
	headers, err := httpx.ParseHeaders(opt.headers)
	if err != nil {
		return trace.Report{}, err
	}

	bodyBytes, hasBody, err := httpx.ParseBody(opt.body)
	if err != nil {
		return trace.Report{}, err
	}

	req, err := httpx.BuildRequest(httpx.RequestSpec{
		Method:  opt.method,
		URL:     rawURL,
		Headers: headers,
		Body:    bodyBytes,
		HasBody: hasBody,
	})
	if err != nil {
		return trace.Report{}, err
	}

	rec := trace.NewRecorder(trace.SystemClock())
	ctx := httptrace.WithClientTrace(req.Context(), trace.NewClientTrace(rec))
	req = req.WithContext(ctx)

	rec.Mark(trace.EventStart)
	resp, err := client.Do(req)
	if err != nil {
		return trace.Report{}, err
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return trace.Report{}, fmt.Errorf("read response body: %w", err)
	}
	rec.Mark(trace.EventDone)

	// resp.Request reflects the final request after any redirects were
	// followed, so report the URL that actually served the response.
	//
	// Redacted, never String: a URL of the form https://user:secret@host puts
	// the password in every line of output, and reqtrace output habitually ends
	// up pasted into tickets and captured in CI logs.
	finalURL := req.URL.Redacted()
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.Redacted()
	}

	return trace.Report{
		URL:           finalURL,
		Method:        req.Method,
		Status:        resp.StatusCode,
		Proto:         resp.Proto,
		ContentLength: n,
		Result:        trace.Summarize(rec),
		Proxy:         pr.used(),
	}, nil
}
