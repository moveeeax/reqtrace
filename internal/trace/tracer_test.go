package trace

import (
	"crypto/tls"
	"net/http/httptrace"
	"testing"
	"time"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestClientTracePopulatesRecorder(t *testing.T) {
	r := NewRecorder(SystemClock())
	ct := NewClientTrace(r)

	ct.DNSStart(httptrace.DNSStartInfo{})
	ct.DNSDone(httptrace.DNSDoneInfo{})
	ct.ConnectStart("tcp", "1.2.3.4:443")
	ct.ConnectDone("tcp", "1.2.3.4:443", nil)
	ct.TLSHandshakeStart()
	ct.TLSHandshakeDone(tls.ConnectionState{}, nil)
	ct.GotFirstResponseByte()

	want := []Event{
		EventDNSStart, EventDNSDone,
		EventConnectStart, EventConnectDone,
		EventTLSStart, EventTLSDone,
		EventFirstByte,
	}
	for _, e := range want {
		if !r.Has(e) {
			t.Errorf("expected event %q to be recorded", e)
		}
	}
}

func TestClientTracePlainHTTPHasNoTLSEvents(t *testing.T) {
	r := NewRecorder(SystemClock())
	ct := NewClientTrace(r)

	ct.ConnectStart("tcp", "1.2.3.4:80")
	ct.ConnectDone("tcp", "1.2.3.4:80", nil)
	ct.GotFirstResponseByte()

	if r.Has(EventTLSStart) || r.Has(EventTLSDone) {
		t.Error("plain HTTP trace must not record TLS events")
	}
	if !r.Has(EventConnectDone) {
		t.Error("expected connect done event")
	}
}

func TestClientTraceKeepsFirstConnectAttempt(t *testing.T) {
	clk := NewFakeClock(
		mustTime("2021-12-01T10:00:00Z"),
		mustTime("2021-12-01T10:00:01Z"),
	)
	r := NewRecorder(clk)
	ct := NewClientTrace(r)

	ct.ConnectStart("tcp", "1.1.1.1:443") // first
	ct.ConnectStart("tcp", "2.2.2.2:443") // retry, must be ignored

	got, _ := r.Time(EventConnectStart)
	if !got.Equal(mustTime("2021-12-01T10:00:00Z")) {
		t.Errorf("connect start = %v, want first attempt", got)
	}
}
