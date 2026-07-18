package trace

import "time"

// Clock abstracts the source of time so timing logic can be exercised
// deterministically in tests.
type Clock interface {
	Now() time.Time
}

// systemClock reads the real wall clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SystemClock returns a Clock backed by time.Now.
func SystemClock() Clock { return systemClock{} }

// FakeClock is a Clock whose readings are supplied by the caller. Each call to
// Now advances to the next queued instant; once the queue is drained it keeps
// returning the last value. It is intended for tests.
type FakeClock struct {
	times []time.Time
	pos   int
}

// NewFakeClock builds a FakeClock that returns the given instants in order.
func NewFakeClock(times ...time.Time) *FakeClock {
	return &FakeClock{times: times}
}

// Now returns the next queued instant.
func (f *FakeClock) Now() time.Time {
	if len(f.times) == 0 {
		return time.Time{}
	}
	t := f.times[f.pos]
	if f.pos < len(f.times)-1 {
		f.pos++
	}
	return t
}
