package trace

import (
	"sync"
	"testing"
	"time"
)

func TestRecorderMarkUsesClock(t *testing.T) {
	base := time.Date(2021, 12, 3, 8, 0, 0, 0, time.UTC)
	clk := NewFakeClock(base, base.Add(10*time.Millisecond))
	r := NewRecorder(clk)

	r.Mark(EventStart)
	r.Mark(EventDNSStart)

	if got, ok := r.Time(EventStart); !ok || !got.Equal(base) {
		t.Fatalf("EventStart = %v (ok=%v), want %v", got, ok, base)
	}
	if got, ok := r.Time(EventDNSStart); !ok || !got.Equal(base.Add(10*time.Millisecond)) {
		t.Fatalf("EventDNSStart = %v (ok=%v), want %v", got, ok, base.Add(10*time.Millisecond))
	}
}

func TestRecorderMarkKeepsFirstInstant(t *testing.T) {
	base := time.Date(2021, 12, 3, 8, 0, 0, 0, time.UTC)
	clk := NewFakeClock(base, base.Add(time.Second))
	r := NewRecorder(clk)

	r.Mark(EventConnectStart)
	r.Mark(EventConnectStart) // second attempt must not overwrite

	got, _ := r.Time(EventConnectStart)
	if !got.Equal(base) {
		t.Errorf("repeated Mark overwrote instant: got %v, want %v", got, base)
	}
}

func TestRecorderMarkAtBypassesClock(t *testing.T) {
	r := NewRecorder(NewFakeClock(time.Now()))
	stamp := time.Date(2021, 10, 15, 12, 30, 0, 0, time.UTC)
	r.MarkAt(EventFirstByte, stamp)

	got, ok := r.Time(EventFirstByte)
	if !ok || !got.Equal(stamp) {
		t.Errorf("MarkAt = %v (ok=%v), want %v", got, ok, stamp)
	}
}

func TestRecorderHasMissingEvent(t *testing.T) {
	r := NewRecorder(nil)
	if r.Has(EventTLSStart) {
		t.Error("expected TLS start to be absent")
	}
	r.Mark(EventTLSStart)
	if !r.Has(EventTLSStart) {
		t.Error("expected TLS start to be present after Mark")
	}
}

func TestNewRecorderNilClockDefaults(t *testing.T) {
	r := NewRecorder(nil)
	r.Mark(EventStart)
	if !r.Has(EventStart) {
		t.Fatal("expected default clock to stamp event")
	}
}

// TestRecorderMarkConcurrentSafe exercises the exact pattern a dual-stack
// dial produces: net.Dialer races an IPv4 and an IPv6 attempt in separate
// goroutines, so httptrace's ConnectStart/ConnectDone callbacks can land on
// the same Recorder concurrently. Before Recorder held a mutex, this pattern
// was an unsynchronized map write, which Go's runtime treats as a fatal,
// unrecoverable error (not just flagged by -race). Run with -race to prove
// there is no data race left either.
func TestRecorderMarkConcurrentSafe(t *testing.T) {
	r := NewRecorder(SystemClock())

	var wg sync.WaitGroup
	events := []Event{EventConnectStart, EventConnectDone, EventDNSStart, EventDNSDone}
	for i := 0; i < 50; i++ {
		for _, e := range events {
			wg.Add(1)
			go func(e Event) {
				defer wg.Done()
				r.Mark(e)
				r.Has(e)
				r.Time(e)
			}(e)
		}
	}
	wg.Wait()

	for _, e := range events {
		if !r.Has(e) {
			t.Errorf("expected %q to be recorded after concurrent Mark calls", e)
		}
	}
}
