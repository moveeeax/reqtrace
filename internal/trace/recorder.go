package trace

import (
	"sync"
	"time"
)

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

// Recorder captures the instant at which each lifecycle Event occurs.
//
// It is safe for concurrent use. That is load-bearing, not defensive: for a
// dual-stack target, net.Dialer.DialContext races an IPv4 and an IPv6 dial
// attempt in two separate goroutines (see net.Dialer's "Happy Eyeballs"
// fallback, net/dial.go's dialParallel), so httptrace's ConnectStart and
// ConnectDone hooks can fire concurrently from different goroutines for a
// single request. httptrace's own doc comment says as much: "Functions may
// be called concurrently from different goroutines." An unsynchronized
// check-then-set on the underlying map would be a concurrent map write,
// which is a fatal, unrecoverable runtime error in Go, not just a data race.
type Recorder struct {
	mu    sync.Mutex
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
// fire more than once when multiple addresses are attempted, including
// concurrently (see the Recorder doc comment).
func (r *Recorder) Mark(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.times[e]; ok {
		return
	}
	r.times[e] = r.clock.Now()
}

// MarkAt stamps the event with an explicit instant, bypassing the Clock. It is
// primarily useful for tests.
func (r *Recorder) MarkAt(e Event, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.times[e] = t
}

// Time returns the recorded instant for an event and whether it was seen.
func (r *Recorder) Time(e Event) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.times[e]
	return t, ok
}

// Has reports whether the event was recorded.
func (r *Recorder) Has(e Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.times[e]
	return ok
}
