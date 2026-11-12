// Package proxy explains which proxy setting applies to a URL and why, using
// the same rules as net/http's ProxyFromEnvironment.
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Env is a set of environment variables, keyed by exact (case-sensitive) name.
type Env map[string]string

// Get returns the value of name, or "" when it is unset.
func (e Env) Get(name string) string { return e[name] }

// Environ captures the process environment.
func Environ() Env {
	env := make(Env)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}

// ParseEnvFile reads a dotenv file: one KEY=VALUE per line, blank lines and
// lines starting with "#" skipped, an optional leading "export ", and a value
// wrapped in matching single or double quotes taken without them. That covers
// Compose-style .env files and shell snippets that export proxy
// variables.
//
// Errors name the line number only: the line itself may hold a proxy password.
func ParseEnvFile(r io.Reader) (Env, error) {
	env := make(Env)
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.ContainsAny(k, " \t") {
			return nil, fmt.Errorf("env file line %d: want KEY=VALUE", n)
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		env[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return env, nil
}

// LoadEnvFile opens path and parses it with ParseEnvFile.
func LoadEnvFile(path string) (Env, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseEnvFile(f)
}
