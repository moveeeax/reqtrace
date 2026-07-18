package main

import (
	"flag"
	"fmt"
	"io"
	"net/http/httptrace"
	"os"
	"strings"
	"time"

	"github.com/moveeeax/reqtrace/internal/httpx"
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
	fs.DurationVar(&opt.timeout, "timeout", 30*time.Second, "overall request timeout")
	fs.BoolVar(&opt.jsonOut, "json", false, "emit machine-readable JSON")
	fs.BoolVar(&opt.insecure, "insecure", false, "skip TLS certificate verification")
	fs.BoolVar(&opt.follow, "follow", false, "follow HTTP redirects")
	fs.StringVar(&opt.body, "body", "", "request body: literal string or @file")
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

	report, err := probe(fs.Arg(0), opt)
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

// probe issues the request described by opt against rawURL, capturing timings.
func probe(rawURL string, opt options) (trace.Report, error) {
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

	client := httpx.NewClient(httpx.ClientOptions{
		Timeout:        opt.timeout,
		Insecure:       opt.insecure,
		FollowRedirect: opt.follow,
	})

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

	return trace.Report{
		URL:           rawURL,
		Method:        req.Method,
		Status:        resp.StatusCode,
		Proto:         resp.Proto,
		ContentLength: n,
		Result:        trace.Summarize(rec),
	}, nil
}
