package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/moveeeax/reqtrace/internal/proxy"
	"github.com/moveeeax/reqtrace/internal/trace"
)

// writeJSON renders the report as indented JSON.
func writeJSON(w io.Writer, r trace.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// writeText renders a human-readable timing breakdown.
func writeText(w io.Writer, r trace.Report) {
	fmt.Fprintf(w, "%s %s\n", r.Method, r.URL)
	fmt.Fprintf(w, "  Status:              %d (%s)\n", r.Status, r.Proto)
	fmt.Fprintf(w, "  Proxy:               %s\n", proxyLabel(r.Proxy))
	if r.Stats != nil {
		writeStatsText(w, *r.Stats)
		fmt.Fprintf(w, "  Body:                %d bytes (last run)\n", r.ContentLength)
		return
	}
	fmt.Fprintf(w, "  DNS lookup:          %s\n", fmtDur(r.Result.DNSLookup))
	fmt.Fprintf(w, "  TCP connect:         %s\n", fmtDur(r.Result.TCPConnect))
	if r.Result.HadTLS {
		fmt.Fprintf(w, "  TLS handshake:       %s\n", fmtDur(r.Result.TLSHandshake))
	} else {
		fmt.Fprintf(w, "  TLS handshake:       n/a\n")
	}
	fmt.Fprintf(w, "  Time to first byte:  %s\n", fmtDur(r.Result.TimeToFirstByte))
	fmt.Fprintf(w, "  Total:               %s\n", fmtDur(r.Result.Total))
	fmt.Fprintf(w, "  Body:                %d bytes\n", r.ContentLength)
}

// writeStatsText renders the per-phase table of a repeated probe.
func writeStatsText(w io.Writer, s trace.Stats) {
	fmt.Fprintf(w, "  Runs:                %d (%d on a reused connection, %d TLS resumed)\n", s.Runs, s.Reused, s.TLSResumed)
	fmt.Fprintf(w, "  %-20s %9s %9s %9s %9s\n", "", "min", "p50", "p95", "max")
	row := func(name string, p trace.PhaseStats) {
		fmt.Fprintf(w, "  %-20s %9s %9s %9s %9s\n", name+":", fmtDur(p.Min), fmtDur(p.P50), fmtDur(p.P95), fmtDur(p.Max))
	}
	row("DNS lookup", s.DNSLookup)
	row("TCP connect", s.TCPConnect)
	if s.HadTLS {
		row("TLS handshake", s.TLSHandshake)
	} else {
		fmt.Fprintf(w, "  %-20s %9s\n", "TLS handshake:", "n/a")
	}
	row("Time to first byte", s.TimeToFirstByte)
	row("Total", s.Total)
}

func proxyLabel(p string) string {
	if p == "" {
		return "direct"
	}
	return p
}

// fmtDur formats a duration in milliseconds with one decimal place.
func fmtDur(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000.0
	return fmt.Sprintf("%.1fms", ms)
}

// writeExplainJSON renders a proxy explanation as indented JSON.
func writeExplainJSON(w io.Writer, ex proxy.Explanation) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(ex)
}

// writeExplainText renders a proxy explanation for a terminal.
func writeExplainText(w io.Writer, ex proxy.Explanation) {
	fmt.Fprintf(w, "Proxy for %s\n", ex.URL)
	for _, v := range ex.Candidates {
		val := "(unset)"
		if v.Set {
			val = v.Value
		}
		mark := ""
		if v.Name == ex.Setting {
			mark = "  <- used"
		}
		fmt.Fprintf(w, "  %-20s %s%s\n", v.Name+":", val, mark)
	}
	if ex.NoProxySetting == "" {
		fmt.Fprintf(w, "  %-20s %s\n", "NO_PROXY:", "(unset)")
	} else {
		fmt.Fprintf(w, "  %-20s %d entries\n", ex.NoProxySetting+":", len(ex.NoProxy))
	}
	for _, e := range ex.NoProxy {
		verdict := "no match"
		if e.Matched {
			verdict = "MATCH"
		}
		fmt.Fprintf(w, "    %-22s %-7s %-8s %s\n", e.Raw, e.Kind, verdict, e.Reason)
		if e.Warning != "" {
			fmt.Fprintf(w, "    %-22s warning: %s\n", "", e.Warning)
		}
	}
	fmt.Fprintf(w, "  %-20s %s\n", "Verdict:", ex.Verdict)
	for _, n := range ex.Notes {
		fmt.Fprintf(w, "  note: %s\n", n)
	}
}
