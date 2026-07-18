package httpx

import "testing"

func TestParseHeadersValid(t *testing.T) {
	h, err := ParseHeaders([]string{
		"Accept: application/json",
		"X-Token:abc123",
		"  User-Agent :  reqtrace/1.0 ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := h.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if got := h.Get("X-Token"); got != "abc123" {
		t.Errorf("X-Token = %q", got)
	}
	if got := h.Get("User-Agent"); got != "reqtrace/1.0" {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestParseHeadersRepeatedAccumulates(t *testing.T) {
	h, err := ParseHeaders([]string{
		"X-Multi: one",
		"X-Multi: two",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	values := h.Values("X-Multi")
	if len(values) != 2 || values[0] != "one" || values[1] != "two" {
		t.Errorf("X-Multi = %v, want [one two]", values)
	}
}

func TestParseHeadersEmptyValueAllowed(t *testing.T) {
	h, err := ParseHeaders([]string{"X-Empty:"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := h["X-Empty"]; !ok {
		t.Error("expected X-Empty to be present with empty value")
	}
}

func TestParseHeadersErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"no colon", "NotAHeader"},
		{"empty name", ": value"},
		{"whitespace name", "   : value"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseHeaders([]string{c.in}); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

func TestParseHeadersEmptyInput(t *testing.T) {
	h, err := ParseHeaders(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h) != 0 {
		t.Errorf("expected empty header set, got %v", h)
	}
}
