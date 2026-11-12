package proxy

import (
	"strings"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	in := `# proxy settings
export HTTPS_PROXY="http://proxy:3128"
http_proxy='http://lower:3128'

NO_PROXY = localhost,.internal
EMPTY=
`
	env, err := ParseEnvFile(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := Env{
		"HTTPS_PROXY": "http://proxy:3128",
		"http_proxy":  "http://lower:3128",
		"NO_PROXY":    "localhost,.internal",
		"EMPTY":       "",
	}
	if len(env) != len(want) {
		t.Errorf("got %d vars, want %d: %v", len(env), len(want), env)
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
}

func TestParseEnvFileRejectsBadLineWithoutEchoingIt(t *testing.T) {
	_, err := ParseEnvFile(strings.NewReader("A=1\nhttp://bob:hunter2@proxy:3128\n"))
	if err == nil {
		t.Fatal("expected an error for a line without '='")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should name the line: %v", err)
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaked the line content: %v", err)
	}
}

func TestEnvironReadsProcessEnv(t *testing.T) {
	t.Setenv("REQTRACE_TEST_VAR", "a=b")
	if got := Environ().Get("REQTRACE_TEST_VAR"); got != "a=b" {
		t.Errorf("got %q, want a=b", got)
	}
}
