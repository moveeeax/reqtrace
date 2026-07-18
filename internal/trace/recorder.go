package trace

import "time"

// Event identifies a point in the lifecycle of an HTTP request.
type Event string

const (
	EventStart        Event = "start"
	EventDNSStart     Event = "dns_start"
	EventDNSDone      Event = "dns_done"
	EventConnectStart Event = "connect_start"
	EventConnectDone  Event = "connect_done"
	EventTLSStart     Event = "tls_start"
	EventTLSDone      Event = "tls_done"
	EventFirstByte    Event = "first_byte"
	EventDone         Event = "done"
)

// Recorder captures the instant at which each lifecycle Event occurs. It is not
// safe for concurrent use; httptrace fires its callbacks on a single request
// goroutine, which is the intended caller.
type Recorder struct {
	clock Clock
	times map[Event]time.Time
}

// NewRecorder returns a Recorder that stamps events using the given Clock.
func NewRecorder(c Clock) *Recorder {
	if c == nil {
		c = SystemClock()
	}
	return &Recorder{clock: c, times: make(map[Event]time.Time)}
}

// Mark stamps the event with the Clock's current time. A repeated event keeps
// the first observed instant, which matters for connect and TLS phases that can
// fire more than once when multiple addresses are attempted.
func (r *Recorder) Mark(e Event) {
	if _, ok := r.times[e]; ok {
		return
	}
	r.times[e] = r.clock.Now()
}

// MarkAt stamps the event with an explicit instant, bypassing the Clock. It is
// primarily useful for tests.
func (r *Recorder) MarkAt(e Event, t time.Time) {
	r.times[e] = t
}

// Time returns the recorded instant for an event and whether it was seen.
func (r *Recorder) Time(e Event) (time.Time, bool) {
	t, ok := r.times[e]
	return t, ok
}

// Has reports whether the event was recorded.
func (r *Recorder) Has(e Event) bool {
	_, ok := r.times[e]
	return ok
}
