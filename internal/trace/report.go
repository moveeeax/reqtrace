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

type jsonReport struct {
	URL           string      `json:"url"`
	Method        string      `json:"method"`
	Status        int         `json:"status"`
	Proto         string      `json:"proto"`
	ContentLength int64       `json:"content_length"`
	TLS           bool        `json:"tls"`
	TimingsMS     jsonTimings `json:"timings_ms"`
}

// MarshalJSON renders the report with durations expressed in milliseconds.
func (r Report) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonReport{
		URL:           r.URL,
		Method:        r.Method,
		Status:        r.Status,
		Proto:         r.Proto,
		ContentLength: r.ContentLength,
		TLS:           r.Result.HadTLS,
		TimingsMS: jsonTimings{
			DNSLookup:       millis(r.Result.DNSLookup),
			TCPConnect:      millis(r.Result.TCPConnect),
			TLSHandshake:    millis(r.Result.TLSHandshake),
			TimeToFirstByte: millis(r.Result.TimeToFirstByte),
			Total:           millis(r.Result.Total),
		},
	})
}
