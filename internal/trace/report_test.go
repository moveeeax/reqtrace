package trace

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReportMarshalJSONShape(t *testing.T) {
	rep := Report{
		URL:           "https://example.com/",
		Method:        "GET",
		Status:        200,
		Proto:         "HTTP/1.1",
		ContentLength: 1256,
		Result: Result{
			DNSLookup:       12 * time.Millisecond,
			TCPConnect:      8 * time.Millisecond,
			TLSHandshake:    34*time.Millisecond + 500*time.Microsecond,
			TimeToFirstByte: 120 * time.Millisecond,
			Total:           140 * time.Millisecond,
			HadTLS:          true,
		},
	}

	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out["url"] != "https://example.com/" {
		t.Errorf("url = %v", out["url"])
	}
	if out["status"].(float64) != 200 {
		t.Errorf("status = %v", out["status"])
	}
	if out["tls"] != true {
		t.Errorf("tls = %v", out["tls"])
	}

	timings, ok := out["timings_ms"].(map[string]interface{})
	if !ok {
		t.Fatalf("timings_ms missing or wrong type: %v", out["timings_ms"])
	}
	if timings["dns_lookup"].(float64) != 12 {
		t.Errorf("dns_lookup = %v, want 12", timings["dns_lookup"])
	}
	if timings["tls_handshake"].(float64) != 34.5 {
		t.Errorf("tls_handshake = %v, want 34.5", timings["tls_handshake"])
	}
	if timings["total"].(float64) != 140 {
		t.Errorf("total = %v, want 140", timings["total"])
	}
}

func TestMillisPrecision(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want float64
	}{
		{0, 0},
		{time.Millisecond, 1},
		{1500 * time.Microsecond, 1.5},
		{time.Second, 1000},
	}
	for _, c := range cases {
		if got := millis(c.in); got != c.want {
			t.Errorf("millis(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestReportJSONOmitsNothing(t *testing.T) {
	raw, err := json.Marshal(Report{Method: "GET"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"url", "method", "status", "proto", "content_length", "tls", "proxy", "timings_ms"} {
		if !containsKey(raw, key) {
			t.Errorf("expected key %q in output %s", key, raw)
		}
	}
}

func containsKey(raw []byte, key string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func TestReportJSONProxyAndStats(t *testing.T) {
	raw, err := json.Marshal(Report{Method: "GET"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var single map[string]json.RawMessage
	json.Unmarshal(raw, &single)
	if string(single["proxy"]) != "null" {
		t.Errorf("direct connection: proxy = %s, want null", single["proxy"])
	}
	if _, ok := single["stats"]; ok {
		t.Errorf("a single run must not carry stats: %s", raw)
	}

	raw, err = json.Marshal(Report{
		Proxy: "http://proxy.internal:3128",
		Stats: &Stats{
			Runs:   3,
			Reused: 2,
			Total:  PhaseStats{Min: 10 * time.Millisecond, P50: 12 * time.Millisecond, P95: 30 * time.Millisecond, Max: 30 * time.Millisecond},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		Proxy string `json:"proxy"`
		Stats struct {
			Runs      int `json:"runs"`
			Reused    int `json:"reused_connections"`
			TimingsMS struct {
				Total struct {
					Min, P50, P95, Max float64
				} `json:"total"`
			} `json:"timings_ms"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Proxy != "http://proxy.internal:3128" {
		t.Errorf("proxy = %q", got.Proxy)
	}
	tot := got.Stats.TimingsMS.Total
	if got.Stats.Runs != 3 || got.Stats.Reused != 2 || tot.Min != 10 || tot.P50 != 12 || tot.P95 != 30 || tot.Max != 30 {
		t.Errorf("stats = %+v\n%s", got.Stats, raw)
	}
}
