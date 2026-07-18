package trace

import (
	"testing"
	"time"
)

func TestSystemClockAdvances(t *testing.T) {
	c := SystemClock()
	first := c.Now()
	time.Sleep(time.Millisecond)
	second := c.Now()
	if !second.After(first) {
		t.Fatalf("expected monotonic system clock, got %v then %v", first, second)
	}
}

func TestFakeClockReturnsQueuedInstants(t *testing.T) {
	base := time.Date(2021, 11, 1, 10, 0, 0, 0, time.UTC)
	c := NewFakeClock(base, base.Add(5*time.Millisecond), base.Add(20*time.Millisecond))

	want := []time.Time{base, base.Add(5 * time.Millisecond), base.Add(20 * time.Millisecond)}
	for i, w := range want {
		if got := c.Now(); !got.Equal(w) {
			t.Errorf("call %d: got %v, want %v", i, got, w)
		}
	}
}

func TestFakeClockRepeatsLastInstant(t *testing.T) {
	base := time.Date(2021, 11, 1, 10, 0, 0, 0, time.UTC)
	c := NewFakeClock(base, base.Add(time.Second))

	c.Now()
	c.Now()
	if got := c.Now(); !got.Equal(base.Add(time.Second)) {
		t.Errorf("expected drained clock to repeat last instant, got %v", got)
	}
}

func TestFakeClockEmptyReturnsZero(t *testing.T) {
	c := NewFakeClock()
	if got := c.Now(); !got.IsZero() {
		t.Errorf("expected zero time from empty fake clock, got %v", got)
	}
}
