package trace

import (
	"testing"
	"time"
)

// mark stamps a sequence of events at offsets from a base instant.
func recorderAt(base time.Time, offsets map[Event]time.Duration) *Recorder {
	r := NewRecorder(nil)
	for e, off := range offsets {
		r.MarkAt(e, base.Add(off))
	}
	return r
}

func TestSummarizeFullHTTPSRequest(t *testing.T) {
	base := time.Date(2021, 11, 20, 9, 0, 0, 0, time.UTC)
	r := recorderAt(base, map[Event]time.Duration{
		EventStart:        0,
		EventDNSStart:     1 * time.Millisecond,
		EventDNSDone:      13 * time.Millisecond,
		EventConnectStart: 13 * time.Millisecond,
		EventConnectDone:  21 * time.Millisecond,
		EventTLSStart:     21 * time.Millisecond,
		EventTLSDone:      55 * time.Millisecond,
		EventFirstByte:    120 * time.Millisecond,
		EventDone:         140 * time.Millisecond,
	})

	got := Summarize(r)
	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"dns", got.DNSLookup, 12 * time.Millisecond},
		{"tcp", got.TCPConnect, 8 * time.Millisecond},
		{"tls", got.TLSHandshake, 34 * time.Millisecond},
		{"ttfb", got.TimeToFirstByte, 120 * time.Millisecond},
		{"total", got.Total, 140 * time.Millisecond},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if !got.HadTLS {
		t.Error("expected HadTLS to be true")
	}
}

func TestSummarizePlainHTTPHasNoTLS(t *testing.T) {
	base := time.Date(2021, 11, 20, 9, 0, 0, 0, time.UTC)
	r := recorderAt(base, map[Event]time.Duration{
		EventStart:        0,
		EventConnectStart: 2 * time.Millisecond,
		EventConnectDone:  9 * time.Millisecond,
		EventFirstByte:    40 * time.Millisecond,
		EventDone:         52 * time.Millisecond,
	})

	got := Summarize(r)
	if got.HadTLS {
		t.Error("plain HTTP must not report TLS")
	}
	if got.TLSHandshake != 0 {
		t.Errorf("TLS handshake = %v, want 0", got.TLSHandshake)
	}
	if got.TCPConnect != 7*time.Millisecond {
		t.Errorf("tcp = %v, want 7ms", got.TCPConnect)
	}
}

func TestSummarizeReusedConnectionSkipsDNS(t *testing.T) {
	base := time.Date(2021, 11, 20, 9, 0, 0, 0, time.UTC)
	r := recorderAt(base, map[Event]time.Duration{
		EventStart:     0,
		EventFirstByte: 8 * time.Millisecond,
		EventDone:      15 * time.Millisecond,
	})

	got := Summarize(r)
	if got.DNSLookup != 0 || got.TCPConnect != 0 {
		t.Errorf("expected zero dns/connect on reused conn, got %v/%v", got.DNSLookup, got.TCPConnect)
	}
	if got.TimeToFirstByte != 8*time.Millisecond {
		t.Errorf("ttfb = %v, want 8ms", got.TimeToFirstByte)
	}
	if got.Total != 15*time.Millisecond {
		t.Errorf("total = %v, want 15ms", got.Total)
	}
}

func TestSummarizeEmptyRecorder(t *testing.T) {
	got := Summarize(NewRecorder(nil))
	if got != (Result{}) {
		t.Errorf("empty recorder should summarize to zero Result, got %+v", got)
	}
}

func TestSpanRequiresBothEvents(t *testing.T) {
	r := NewRecorder(nil)
	r.MarkAt(EventDNSStart, time.Now())
	if _, ok := span(r, EventDNSStart, EventDNSDone); ok {
		t.Error("span should be absent when the end event is missing")
	}
}
