package trace

import (
	"math"
	"slices"
	"time"
)

// PhaseStats summarizes one phase across repeated runs.
type PhaseStats struct {
	Min time.Duration
	P50 time.Duration
	P95 time.Duration
	Max time.Duration
}

// Stats aggregates the Results of repeated runs against one URL.
//
// Every run counts towards every phase. A phase a run skipped, such as DNS on
// a reused connection, counts as zero: that is what the phase cost that run.
type Stats struct {
	Runs       int
	Reused     int // runs served on a kept-alive connection
	TLSResumed int // runs whose TLS handshake resumed a session
	HadTLS     bool

	DNSLookup       PhaseStats
	TCPConnect      PhaseStats
	TLSHandshake    PhaseStats
	TimeToFirstByte PhaseStats
	Total           PhaseStats
}

// Aggregate computes Stats over results. It returns the zero Stats for no
// results.
func Aggregate(results []Result) Stats {
	s := Stats{Runs: len(results)}
	if len(results) == 0 {
		return s
	}
	for _, r := range results {
		if r.Reused {
			s.Reused++
		}
		if r.TLSResumed {
			s.TLSResumed++
		}
		if r.HadTLS {
			s.HadTLS = true
		}
	}
	phase := func(get func(Result) time.Duration) PhaseStats {
		ds := make([]time.Duration, len(results))
		for i, r := range results {
			ds[i] = get(r)
		}
		slices.Sort(ds)
		return PhaseStats{
			Min: ds[0],
			P50: percentile(ds, 50),
			P95: percentile(ds, 95),
			Max: ds[len(ds)-1],
		}
	}
	s.DNSLookup = phase(func(r Result) time.Duration { return r.DNSLookup })
	s.TCPConnect = phase(func(r Result) time.Duration { return r.TCPConnect })
	s.TLSHandshake = phase(func(r Result) time.Duration { return r.TLSHandshake })
	s.TimeToFirstByte = phase(func(r Result) time.Duration { return r.TimeToFirstByte })
	s.Total = phase(func(r Result) time.Duration { return r.Total })
	return s
}

// percentile returns the p-th percentile of sorted by the nearest-rank
// method: the smallest sample with at least p percent of samples at or below
// it. It always returns a measured value, never an interpolated one.
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}
