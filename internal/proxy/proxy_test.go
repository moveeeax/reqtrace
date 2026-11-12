package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}

// verdictCases cover the rules in net/http's proxy selection. Each one is
// also checked against the real http.ProxyFromEnvironment below.
var verdictCases = []struct {
	name  string
	env   Env
	url   string
	proxy string // "" means direct
}{
	{"nothing set", Env{}, "https://example.com/", ""},
	{"https proxy", Env{"HTTPS_PROXY": "http://proxy:3128"}, "https://example.com/", "http://proxy:3128"},
	{"http proxy does not cover https", Env{"HTTP_PROXY": "http://proxy:3128"}, "https://example.com/", ""},
	{"https proxy does not cover http", Env{"HTTPS_PROXY": "http://proxy:3128"}, "http://example.com/", ""},
	{"lowercase only", Env{"https_proxy": "http://lower:3128"}, "https://example.com/", "http://lower:3128"},
	{"uppercase wins over lowercase", Env{"HTTPS_PROXY": "http://upper:3128", "https_proxy": "http://lower:3128"}, "https://example.com/", "http://upper:3128"},
	{"no scheme means http", Env{"HTTPS_PROXY": "proxy:3128"}, "https://example.com/", "http://proxy:3128"},
	{"localhost is direct", Env{"HTTP_PROXY": "proxy:3128"}, "http://localhost:8080/", ""},
	{"loopback is direct", Env{"HTTP_PROXY": "proxy:3128"}, "http://127.0.0.2/", ""},
	{"star", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "*"}, "https://example.com/", ""},
	{"domain matches itself", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com"}, "https://example.com/", ""},
	{"domain matches subdomain", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com"}, "https://api.example.com/", ""},
	{"domain is not a substring match", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com"}, "https://notexample.com/", "http://proxy:3128"},
	{"leading dot skips the apex", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": ".example.com"}, "https://example.com/", "http://proxy:3128"},
	{"leading dot matches subdomain", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": ".example.com"}, "https://a.example.com/", ""},
	{"star dot matches subdomain", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "*.example.com"}, "https://a.example.com/", ""},
	{"case and spaces", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": " Example.COM "}, "https://example.com/", ""},
	{"domain with port, same port", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com:8443"}, "https://example.com:8443/", ""},
	{"domain with port, default port", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com:443"}, "https://example.com/", ""},
	{"domain with port, other port", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "example.com:8443"}, "https://example.com/", "http://proxy:3128"},
	{"cidr matches ip", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "10.0.0.0/8"}, "https://10.1.2.3/", ""},
	{"cidr outside", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "10.0.0.0/8"}, "https://192.0.2.1/", "http://proxy:3128"},
	{"cidr never matches a name", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "10.0.0.0/8"}, "https://internal.example/", "http://proxy:3128"},
	{"bare ip", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "192.0.2.1"}, "https://192.0.2.1:8443/", ""},
	{"ip with port mismatch", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "192.0.2.1:80"}, "https://192.0.2.1/", "http://proxy:3128"},
	{"ipv6 with port", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "[2001:db8::1]:443"}, "https://[2001:db8::1]/", ""},
	{"domain never matches an ip", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "192.0.2.1.nip.io"}, "https://192.0.2.1/", "http://proxy:3128"},
	{"scheme in no_proxy never matches", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "https://example.com"}, "https://example.com/", "http://proxy:3128"},
	{"mid wildcard never matches", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "10.0.0.*"}, "https://10.0.0.5/", "http://proxy:3128"},
	{"uppercase NO_PROXY wins", Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "other.test", "no_proxy": "example.com"}, "https://example.com/", "http://proxy:3128"},
	{"lowercase no_proxy alone", Env{"HTTPS_PROXY": "proxy:3128", "no_proxy": "example.com"}, "https://example.com/", ""},
	{"ALL_PROXY is ignored", Env{"ALL_PROXY": "socks5://proxy:1080"}, "https://example.com/", ""},
	{"socks5 proxy", Env{"HTTPS_PROXY": "socks5://proxy:1080"}, "https://example.com/", "socks5://proxy:1080"},
}

func TestExplainVerdicts(t *testing.T) {
	for _, c := range verdictCases {
		t.Run(c.name, func(t *testing.T) {
			ex := Explain(c.env, mustURL(t, c.url))
			if ex.Proxy != c.proxy {
				t.Errorf("proxy = %q, want %q (verdict %q)", ex.Proxy, c.proxy, ex.Verdict)
			}
			for _, n := range ex.Notes {
				if strings.HasPrefix(n, "internal check failed") {
					t.Errorf("per-entry reasons disagree with the verdict: %v", ex)
				}
			}
		})
	}
}

// TestVerdictMatchesNetHTTP runs every case through the real
// http.ProxyFromEnvironment in a child process (it caches the environment
// on first use, so one process can only check one environment). This is what
// keeps the variable precedence in this package honest across Go releases.
func TestVerdictMatchesNetHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns one process per case")
	}
	for _, c := range verdictCases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProxyFromEnvironment$")
			cmd.Env = []string{"REQTRACE_PROXY_HELPER_URL=" + c.url}
			for k, v := range c.env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("helper: %v", err)
			}
			got := strings.TrimSpace(string(out))
			if got != c.proxy {
				t.Errorf("net/http picked %q, explain says %q", got, c.proxy)
			}
		})
	}
}

// TestHelperProxyFromEnvironment is the child side of
// TestVerdictMatchesNetHTTP; it does nothing in a normal test run.
func TestHelperProxyFromEnvironment(t *testing.T) {
	raw := os.Getenv("REQTRACE_PROXY_HELPER_URL")
	if raw == "" {
		return
	}
	u, err := http.ProxyFromEnvironment(&http.Request{URL: mustURL(t, raw)})
	if err != nil {
		fmt.Println("error")
	} else if u != nil {
		fmt.Println(u.String())
	} else {
		fmt.Println()
	}
	os.Exit(0)
}

func entryNamed(t *testing.T, ex Explanation, raw string) Entry {
	t.Helper()
	for _, e := range ex.NoProxy {
		if e.Raw == raw {
			return e
		}
	}
	t.Fatalf("no entry %q in %+v", raw, ex.NoProxy)
	return Entry{}
}

func TestExplainEntryReasons(t *testing.T) {
	env := Env{
		"HTTPS_PROXY": "proxy:3128",
		"NO_PROXY":    "localhost, .example.com,10.0.0.0/8,https://example.com,10.0.0.*,10.0.0.0/33,:8080,example.com:http,*,later.test",
	}
	ex := Explain(env, mustURL(t, "https://example.com/"))

	cases := []struct {
		raw     string
		kind    string
		matched bool
		reason  string
		warning string
	}{
		{"localhost", "domain", false, "is not localhost", ""},
		{".example.com", "domain", false, "subdomains of example.com only", ""},
		{"10.0.0.0/8", "cidr", false, "names are not resolved", ""},
		{"https://example.com", "domain", false, "", "URL scheme"},
		{"10.0.0.*", "domain", false, "", `leading "*."`},
		{"10.0.0.0/33", "domain", false, "", "not a valid CIDR"},
		{":8080", "ignored", false, "no host", "without a host"},
		{"example.com:http", "domain", false, "", "not a number"},
		{"*", "all", true, "every host", ""},
		{"later.test", "unread", false, "stops reading", ""},
	}
	for _, c := range cases {
		e := entryNamed(t, ex, c.raw)
		if e.Kind != c.kind || e.Matched != c.matched {
			t.Errorf("%s: kind=%s matched=%v, want %s %v", c.raw, e.Kind, e.Matched, c.kind, c.matched)
		}
		if !strings.Contains(e.Reason, c.reason) {
			t.Errorf("%s: reason %q lacks %q", c.raw, e.Reason, c.reason)
		}
		if c.warning == "" && e.Warning != "" {
			t.Errorf("%s: unexpected warning %q", c.raw, e.Warning)
		}
		if !strings.Contains(e.Warning, c.warning) {
			t.Errorf("%s: warning %q lacks %q", c.raw, e.Warning, c.warning)
		}
	}
	if ex.Verdict != `direct: NO_PROXY entry "*" matches` {
		t.Errorf("verdict = %q", ex.Verdict)
	}
}

func TestExplainVerdictReasons(t *testing.T) {
	cases := []struct {
		env  Env
		url  string
		want string
	}{
		{Env{}, "https://example.com/", "direct: neither HTTPS_PROXY nor https_proxy is set"},
		{Env{"HTTPS_PROXY": "http://proxy:3128"}, "https://example.com/", "through proxy http://proxy:3128, from HTTPS_PROXY"},
		{Env{"HTTPS_PROXY": "http://proxy:3128"}, "https://localhost/", "direct: net/http never proxies localhost or a loopback address"},
		{Env{"HTTPS_PROXY": "http://proxy:3128", "no_proxy": "example.com"}, "https://example.com/", `direct: no_proxy entry "example.com" matches`},
		{Env{"HTTPS_PROXY": "http://proxy\x7f:3128"}, "https://example.com/", "direct: HTTPS_PROXY does not parse as a URL, and net/http ignores it"},
		{Env{"HTTP_PROXY": "proxy:3128", "REQUEST_METHOD": "GET"}, "http://example.com/", "request fails: REQUEST_METHOD is set"},
	}
	for _, c := range cases {
		ex := Explain(c.env, mustURL(t, c.url))
		if !strings.HasPrefix(ex.Verdict, c.want) {
			t.Errorf("%v %s: verdict %q, want prefix %q", c.env, c.url, ex.Verdict, c.want)
		}
	}
}

func TestExplainNotes(t *testing.T) {
	has := func(ex Explanation, sub string) bool {
		for _, n := range ex.Notes {
			if strings.Contains(n, sub) {
				return true
			}
		}
		return false
	}

	ex := Explain(Env{"HTTPS_PROXY": "a:1", "https_proxy": "b:1", "ALL_PROXY": "socks5://c:1"}, mustURL(t, "https://example.com/"))
	if !has(ex, "both set and differ") || !has(ex, "ALL_PROXY is set") {
		t.Errorf("notes = %q", ex.Notes)
	}

	ex = Explain(Env{"HTTP_PROXY": "a:1"}, mustURL(t, "https://example.com/"))
	if !has(ex, "applies to http:// URLs only") {
		t.Errorf("notes = %q", ex.Notes)
	}

	ex = Explain(Env{"HTTP_PROXY": "a:1"}, mustURL(t, "http://example.com/"))
	if !has(ex, "curl reads http_proxy in lowercase only") {
		t.Errorf("notes = %q", ex.Notes)
	}

	ex = Explain(Env{"HTTPS_PROXY": "ftp://a:21"}, mustURL(t, "https://example.com/"))
	if !has(ex, `proxy scheme "ftp" is not one net/http supports`) {
		t.Errorf("notes = %q", ex.Notes)
	}

	ex = Explain(Env{"HTTPS_PROXY": "a:1", "https_proxy": "a:1"}, mustURL(t, "https://example.com/"))
	if len(ex.Notes) != 0 {
		t.Errorf("identical upper and lower values need no note: %q", ex.Notes)
	}
}

func TestExplainMasksProxyPassword(t *testing.T) {
	env := Env{"HTTPS_PROXY": "http://bob:hunter2@proxy:3128", "https_proxy": "bob:hunter2@other:3128"}
	ex := Explain(env, mustURL(t, "https://alice:pw1@example.com/"))
	dump := fmt.Sprintf("%+v", ex)
	for _, secret := range []string{"hunter2", "pw1"} {
		if strings.Contains(dump, secret) {
			t.Errorf("explanation leaked %q: %s", secret, dump)
		}
	}
	if ex.Proxy != "http://bob:xxxxx@proxy:3128" {
		t.Errorf("proxy = %q", ex.Proxy)
	}
}

func TestProxyFuncUsesEnv(t *testing.T) {
	f := ProxyFunc(Env{"HTTPS_PROXY": "proxy:3128", "NO_PROXY": "skip.test"})
	got, err := f(&http.Request{URL: mustURL(t, "https://example.com/")})
	if err != nil || got == nil || got.Host != "proxy:3128" {
		t.Errorf("proxy = %v, err = %v", got, err)
	}
	got, err = f(&http.Request{URL: mustURL(t, "https://skip.test/")})
	if err != nil || got != nil {
		t.Errorf("NO_PROXY host: proxy = %v, err = %v", got, err)
	}
}
