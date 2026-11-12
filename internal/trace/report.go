package trace

import (
	"encoding/json"
	"time"
)

// Report bundles the timing Result with metadata about the response so it can
// be rendered as text or JSON.
type Report struct {
	URL           string
	Method        string
	Status        int
	Proto         string
	ContentLength int64
	Result        Result

	// Proxy is the proxy the request went through, password masked; empty
	// for a direct connection.
	Proxy string

	// Stats is set for a repeated probe. The other fields then describe the
	// last run.
	Stats *Stats
}

// millis converts a duration to milliseconds with microsecond precision.
func millis(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

type jsonTimings struct {
	DNSLookup       float64 `json:"dns_lookup"`
	TCPConnect      float64 `json:"tcp_connect"`
	TLSHandshake    float64 `json:"tls_handshake"`
	TimeToFirstByte float64 `json:"time_to_first_byte"`
	Total           float64 `json:"total"`
}

type jsonPhase struct {
	Min float64 `json:"min"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

type jsonStatTimings struct {
	DNSLookup       jsonPhase `json:"dns_lookup"`
	TCPConnect      jsonPhase `json:"tcp_connect"`
	TLSHandshake    jsonPhase `json:"tls_handshake"`
	TimeToFirstByte jsonPhase `json:"time_to_first_byte"`
	Total           jsonPhase `json:"total"`
}

type jsonStats struct {
	Runs              int             `json:"runs"`
	ReusedConnections int             `json:"reused_connections"`
	TLSResumed        int             `json:"tls_resumed"`
	TimingsMS         jsonStatTimings `json:"timings_ms"`
}

type jsonReport struct {
	URL           string      `json:"url"`
	Method        string      `json:"method"`
	Status        int         `json:"status"`
	Proto         string      `json:"proto"`
	ContentLength int64       `json:"content_length"`
	TLS           bool        `json:"tls"`
	Proxy         *string     `json:"proxy"`
	TimingsMS     jsonTimings `json:"timings_ms"`
	Stats         *jsonStats  `json:"stats,omitempty"`
}

func phaseMillis(p PhaseStats) jsonPhase {
	return jsonPhase{Min: millis(p.Min), P50: millis(p.P50), P95: millis(p.P95), Max: millis(p.Max)}
}

// MarshalJSON renders the report with durations expressed in milliseconds.
//
// "proxy" is null for a direct connection, and "stats" is present only for a
// repeated probe.
func (r Report) MarshalJSON() ([]byte, error) {
	var proxy *string
	if r.Proxy != "" {
		proxy = &r.Proxy
	}
	var stats *jsonStats
	if s := r.Stats; s != nil {
		stats = &jsonStats{
			Runs:              s.Runs,
			ReusedConnections: s.Reused,
			TLSResumed:        s.TLSResumed,
			TimingsMS: jsonStatTimings{
				DNSLookup:       phaseMillis(s.DNSLookup),
				TCPConnect:      phaseMillis(s.TCPConnect),
				TLSHandshake:    phaseMillis(s.TLSHandshake),
				TimeToFirstByte: phaseMillis(s.TimeToFirstByte),
				Total:           phaseMillis(s.Total),
			},
		}
	}
	return json.Marshal(jsonReport{
		URL:           r.URL,
		Method:        r.Method,
		Status:        r.Status,
		Proto:         r.Proto,
		ContentLength: r.ContentLength,
		TLS:           r.Result.HadTLS,
		Proxy:         proxy,
		TimingsMS: jsonTimings{
			DNSLookup:       millis(r.Result.DNSLookup),
			TCPConnect:      millis(r.Result.TCPConnect),
			TLSHandshake:    millis(r.Result.TLSHandshake),
			TimeToFirstByte: millis(r.Result.TimeToFirstByte),
			Total:           millis(r.Result.Total),
		},
		Stats: stats,
	})
}
