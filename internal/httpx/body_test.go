package httpx

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

func TestParseBodyEmpty(t *testing.T) {
	data, ok, err := ParseBody("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok || data != nil {
		t.Errorf("expected no body, got ok=%v data=%q", ok, data)
	}
}

func TestParseBodyLiteral(t *testing.T) {
	data, ok, err := ParseBody(`{"hello":"world"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for literal body")
	}
	if string(data) != `{"hello":"world"}` {
		t.Errorf("data = %q", data)
	}
}

func TestParseBodyFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.json")
	want := []byte("payload-from-file")
	if err := ioutil.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	data, ok, err := ParseBody("@" + path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || string(data) != string(want) {
		t.Errorf("data = %q (ok=%v), want %q", data, ok, want)
	}
}

func TestParseBodyMissingFile(t *testing.T) {
	_, _, err := ParseBody("@" + filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseBodyEmptyPath(t *testing.T) {
	if _, _, err := ParseBody("@"); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestParseBodyLiteralAtSignInside(t *testing.T) {
	// Only a leading '@' triggers file mode.
	data, ok, err := ParseBody("user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || string(data) != "user@example.com" {
		t.Errorf("data = %q (ok=%v)", data, ok)
	}
}
