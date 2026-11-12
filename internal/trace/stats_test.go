package trace

import (
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestPercentileNearestRank(t *testing.T) {
	// 1..20 ms: nearest rank puts p50 at the 10th sample and p95 at the 19th.
	var ds []time.Duration
	for i := 1; i <= 20; i++ {
		ds = append(ds, ms(i))
	}
	cases := []struct {
		p    float64
		want time.Duration
	}{
		{0, ms(1)},
		{50, ms(10)},
		{95, ms(19)},
		{100, ms(20)},
	}
	for _, c := range cases {
		if got := percentile(ds, c.p); got != c.want {
			t.Errorf("p%v = %v, want %v", c.p, got, c.want)
		}
	}
}

func TestPercentileSmallSample(t *testing.T) {
	ds := []time.Duration{ms(3), ms(7), ms(9)}
	if got := percentile(ds, 50); got != ms(7) {
		t.Errorf("p50 = %v, want 7ms", got)
	}
	if got := percentile(ds, 95); got != ms(9) {
		t.Errorf("p95 of 3 samples = %v, want the max", got)
	}
}

func TestAggregate(t *testing.T) {
	results := []Result{
		{DNSLookup: ms(12), TCPConnect: ms(8), TLSHandshake: ms(30), TimeToFirstByte: ms(90), Total: ms(100), HadTLS: true},
		{TimeToFirstByte: ms(20), Total: ms(25), Reused: true},
		{TimeToFirstByte: ms(40), Total: ms(45), Reused: true},
		{TCPConnect: ms(7), TLSHandshake: ms(10), TimeToFirstByte: ms(30), Total: ms(35), HadTLS: true, TLSResumed: true},
	}
	s := Aggregate(results)

	if s.Runs != 4 || s.Reused != 2 || s.TLSResumed != 1 || !s.HadTLS {
		t.Errorf("counts = %+v", s)
	}
	want := PhaseStats{Min: ms(20), P50: ms(30), P95: ms(90), Max: ms(90)}
	if s.TimeToFirstByte != want {
		t.Errorf("ttfb = %+v, want %+v", s.TimeToFirstByte, want)
	}
	// Reused runs skipped DNS and count as zero.
	if s.DNSLookup.Min != 0 || s.DNSLookup.P50 != 0 || s.DNSLookup.Max != ms(12) {
		t.Errorf("dns = %+v", s.DNSLookup)
	}
	if s.Total.Min != ms(25) || s.Total.Max != ms(100) {
		t.Errorf("total = %+v", s.Total)
	}
}

func TestAggregateEmpty(t *testing.T) {
	if got := Aggregate(nil); got != (Stats{}) {
		t.Errorf("Aggregate(nil) = %+v, want zero", got)
	}
}
