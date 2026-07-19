package main

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

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

// fmtDur formats a duration in milliseconds with one decimal place.
func fmtDur(d time.Duration) string {
	ms := float64(d.Microseconds()) / 1000.0
	return fmt.Sprintf("%.1fms", ms)
}
