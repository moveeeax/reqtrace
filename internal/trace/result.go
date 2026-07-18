package trace

import "time"

// Result holds the durations of each request phase derived from a Recorder.
// A phase whose bounding events were not both observed is reported as zero.
type Result struct {
	DNSLookup       time.Duration
	TCPConnect      time.Duration
	TLSHandshake    time.Duration
	TimeToFirstByte time.Duration
	Total           time.Duration

	// HadTLS reports whether a TLS handshake was observed, letting callers
	// distinguish a genuinely zero-length handshake from a plain HTTP request.
	HadTLS bool
}

// span returns the duration between two events when both were recorded.
func span(r *Recorder, from, to Event) (time.Duration, bool) {
	start, ok1 := r.Time(from)
	end, ok2 := r.Time(to)
	if !ok1 || !ok2 {
		return 0, false
	}
	return end.Sub(start), true
}

// Summarize reduces a Recorder's events into a Result.
func Summarize(r *Recorder) Result {
	var res Result

	if d, ok := span(r, EventDNSStart, EventDNSDone); ok {
		res.DNSLookup = d
	}
	if d, ok := span(r, EventConnectStart, EventConnectDone); ok {
		res.TCPConnect = d
	}
	if d, ok := span(r, EventTLSStart, EventTLSDone); ok {
		res.TLSHandshake = d
		res.HadTLS = true
	}
	if d, ok := span(r, EventStart, EventFirstByte); ok {
		res.TimeToFirstByte = d
	}
	if d, ok := span(r, EventStart, EventDone); ok {
		res.Total = d
	}

	return res
}
