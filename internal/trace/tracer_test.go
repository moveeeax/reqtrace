package trace

import (
	"crypto/tls"
	"errors"
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

// TestClientTraceIgnoresFailedConnectAttempt guards a dual-stack fallback: Go
// dials each resolved address in turn (e.g. an unreachable AAAA record before
// a working A record) and fires ConnectStart/ConnectDone for every attempt.
// The failed attempt's ConnectDone must not be the one recorded, or
// TCPConnect would report only the doomed attempt's short failure time
// instead of the time to the connection that was actually used.
func TestClientTraceIgnoresFailedConnectAttempt(t *testing.T) {
	clk := NewFakeClock(
		mustTime("2021-12-01T10:00:00.000Z"), // ConnectStart, attempt 1 (ipv6)
		mustTime("2021-12-01T10:00:00.900Z"), // ConnectDone, attempt 2 (succeeds)
	)
	// The failed ConnectDone (attempt 1) must not consume a clock reading at
	// all: it is rejected before Recorder.Mark ever calls the clock.
	r := NewRecorder(clk)
	ct := NewClientTrace(r)

	ct.ConnectStart("tcp", "[::1]:443")
	ct.ConnectDone("tcp", "[::1]:443", errors.New("connect: network is unreachable"))
	ct.ConnectStart("tcp", "127.0.0.1:443") // retry, ConnectStart ignored (kept first)
	ct.ConnectDone("tcp", "127.0.0.1:443", nil)

	got, ok := r.Time(EventConnectDone)
	if !ok {
		t.Fatal("expected EventConnectDone to be recorded")
	}
	if !got.Equal(mustTime("2021-12-01T10:00:00.900Z")) {
		t.Errorf("connect done = %v, want the successful attempt's instant, not the failed one", got)
	}
}

// TestClientTraceAllConnectAttemptsFail documents that if every dial attempt
// fails, EventConnectDone is simply never recorded (client.Do will itself
// return an error, so no report is ever summarized from this state).
func TestClientTraceAllConnectAttemptsFail(t *testing.T) {
	r := NewRecorder(SystemClock())
	ct := NewClientTrace(r)

	ct.ConnectStart("tcp", "1.1.1.1:443")
	ct.ConnectDone("tcp", "1.1.1.1:443", errors.New("connect: connection refused"))

	if r.Has(EventConnectDone) {
		t.Error("a failed connect attempt must not record EventConnectDone")
	}
}
