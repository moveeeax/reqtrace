package proxy

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/net/idna"
)

// Variable names in the order net/http reads them: the first non-empty one
// wins. net/http's vendored copy of httpproxy reads the uppercase name first;
// golang.org/x/net/http/httpproxy.FromEnvironment (v0.62.0) reads lowercase
// first, so this package builds its httpproxy.Config itself rather than
// calling FromEnvironment. TestVerdictMatchesNetHTTP pins the order against
// the real http.ProxyFromEnvironment.
var (
	httpVars    = []string{"HTTP_PROXY", "http_proxy"}
	httpsVars   = []string{"HTTPS_PROXY", "https_proxy"}
	noProxyVars = []string{"NO_PROXY", "no_proxy"}
	allVars     = []string{"ALL_PROXY", "all_proxy"}
)

// supportedSchemes are the proxy URL schemes http.Transport can speak.
var supportedSchemes = map[string]bool{"http": true, "https": true, "socks5": true, "socks5h": true}

// first returns the first variable among names with a non-empty value.
func first(env Env, names []string) (string, string) {
	for _, n := range names {
		if v := env.Get(n); v != "" {
			return n, v
		}
	}
	return "", ""
}

// Config returns the httpproxy.Config that net/http derives from env.
func Config(env Env) *httpproxy.Config {
	_, httpProxy := first(env, httpVars)
	_, httpsProxy := first(env, httpsVars)
	_, noProxy := first(env, noProxyVars)
	return &httpproxy.Config{
		HTTPProxy:  httpProxy,
		HTTPSProxy: httpsProxy,
		NoProxy:    noProxy,
		CGI:        env.Get("REQUEST_METHOD") != "",
	}
}

// ProxyFunc returns an http.Transport Proxy function that resolves proxies
// from env instead of the process environment.
func ProxyFunc(env Env) func(*http.Request) (*url.URL, error) {
	f := Config(env).ProxyFunc()
	return func(r *http.Request) (*url.URL, error) { return f(r.URL) }
}

// Variable is one proxy environment variable as seen by the explanation.
type Variable struct {
	Name string `json:"name"`
	// Value is the setting with any password masked; empty when unset.
	Value string `json:"value"`
	Set   bool   `json:"set"`
}

// Entry is the verdict for a single NO_PROXY item.
type Entry struct {
	Raw string `json:"entry"`
	// Kind is how net/http parsed the entry: "all", "cidr", "ip", "domain",
	// "ignored" (malformed, dropped by net/http) or "unread" (after a "*").
	Kind    string `json:"kind"`
	Matched bool   `json:"matched"`
	Reason  string `json:"reason"`
	// Warning says why the entry can never match any URL, if so.
	Warning string `json:"warning,omitempty"`
}

// Explanation describes which proxy a request to a URL would use and why.
type Explanation struct {
	URL string `json:"url"`
	// Candidates are the proxy variables for the URL's scheme, in the order
	// net/http reads them.
	Candidates []Variable `json:"candidates"`
	// Setting names the variable that supplied the proxy URL, if any.
	Setting        string  `json:"setting"`
	NoProxySetting string  `json:"no_proxy_setting"`
	NoProxy        []Entry `json:"no_proxy"`
	// Proxy is the proxy the request goes through, password masked; empty
	// when the request connects directly.
	Proxy string `json:"proxy"`
	// Error is set when net/http would abort the request instead.
	Error   string   `json:"error,omitempty"`
	Verdict string   `json:"verdict"`
	Notes   []string `json:"notes"`
}

// Explain reports how net/http resolves the proxy for u under env. The
// verdict comes from httpproxy itself, the code behind
// http.ProxyFromEnvironment; the per-entry reasons mirror its rules.
func Explain(env Env, u *url.URL) Explanation {
	ex := Explanation{URL: u.Redacted(), Notes: []string{}}

	var names []string
	switch u.Scheme {
	case "https":
		names = httpsVars
	case "http":
		names = httpVars
	}
	for _, n := range names {
		v := env.Get(n)
		ex.Candidates = append(ex.Candidates, Variable{Name: n, Value: displayProxy(v), Set: v != ""})
	}
	setting, raw := first(env, names)
	ex.Setting = setting

	host, port, ip, loopback := target(u)

	noProxyName, noProxy := first(env, noProxyVars)
	ex.NoProxySetting = noProxyName
	ex.NoProxy = evaluate(noProxy, host, port, ip)

	got, err := Config(env).ProxyFunc()(u)
	matched := firstMatch(ex.NoProxy)
	parsed, parseErr := parseProxy(raw)

	switch {
	case err != nil:
		ex.Error = err.Error()
		ex.Verdict = fmt.Sprintf("request fails: REQUEST_METHOD is set, so net/http treats this process as a CGI handler and refuses %s", setting)
	case got != nil:
		ex.Proxy = got.Redacted()
		ex.Verdict = fmt.Sprintf("through proxy %s, from %s", ex.Proxy, setting)
		if matched != nil || loopback {
			ex.Notes = append(ex.Notes, "internal check failed: the per-entry reasons say direct but net/http picks the proxy; trust the verdict")
		}
	case raw == "":
		ex.Verdict = fmt.Sprintf("direct: neither %s is set", strings.Join(names, " nor "))
		if len(names) == 0 {
			ex.Verdict = fmt.Sprintf("direct: no proxy variable applies to scheme %q", u.Scheme)
		}
	case parseErr != nil || parsed == nil:
		ex.Verdict = fmt.Sprintf("direct: %s does not parse as a URL, and net/http ignores it", setting)
	case loopback:
		ex.Verdict = "direct: net/http never proxies localhost or a loopback address"
	case matched != nil:
		ex.Verdict = fmt.Sprintf("direct: %s entry %q matches", noProxyName, matched.Raw)
	default:
		ex.Verdict = "direct"
		ex.Notes = append(ex.Notes, "internal check failed: the per-entry reasons say proxy but net/http connects directly; trust the verdict")
	}

	ex.Notes = append(ex.Notes, notes(env, u, setting, parsed)...)
	return ex
}

// target mirrors httpproxy's canonicalAddr and the early exits in useProxy.
func target(u *url.URL) (host, port string, ip net.IP, loopback bool) {
	host = u.Hostname()
	if v, err := idnaASCII(host); err == nil {
		host = v
	}
	port = u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	if host == "localhost" {
		loopback = true
	}
	if nip, err := netip.ParseAddr(host); err == nil {
		ip = net.IP(nip.AsSlice())
		if ip.IsLoopback() {
			loopback = true
		}
	}
	return strings.ToLower(strings.TrimSpace(host)), port, ip, loopback
}

// evaluate walks a NO_PROXY list entry by entry with httpproxy's parsing
// rules (config.init) and reports whether each entry matches host:port.
func evaluate(list, host, port string, ip net.IP) []Entry {
	entries := []Entry{}
	sawAll := false
	for _, item := range strings.Split(list, ",") {
		p := strings.ToLower(strings.TrimSpace(item))
		if p == "" {
			continue
		}
		e := Entry{Raw: strings.TrimSpace(item)}
		switch {
		case sawAll:
			e.Kind = "unread"
			e.Reason = `net/http stops reading the list at "*"`
		case p == "*":
			sawAll = true
			e.Kind, e.Matched = "all", true
			e.Reason = `"*" turns the proxy off for every host`
		default:
			classify(&e, p, host, port, ip)
		}
		entries = append(entries, e)
	}
	return entries
}

// classify fills in kind, match and warnings for one non-"*" entry p, which
// is already lowercased and trimmed.
func classify(e *Entry, p, host, port string, ip net.IP) {
	const nameNotIP = "host is a name, and IP and CIDR entries only match IP addresses (names are not resolved)"

	if _, cidr, err := net.ParseCIDR(p); err == nil {
		e.Kind = "cidr"
		switch {
		case ip == nil:
			e.Reason = nameNotIP
		case cidr.Contains(ip):
			e.Matched, e.Reason = true, fmt.Sprintf("%s is inside %s", ip, cidr)
		default:
			e.Reason = fmt.Sprintf("%s is outside %s", ip, cidr)
		}
		return
	}

	phost, pport, err := net.SplitHostPort(p)
	if err == nil {
		if phost == "" {
			e.Kind, e.Reason = "ignored", "no host part"
			e.Warning = "net/http drops an entry without a host"
			return
		}
		if phost[0] == '[' && phost[len(phost)-1] == ']' {
			phost = phost[1 : len(phost)-1]
		}
	} else {
		phost = p
	}

	if pip := net.ParseIP(phost); pip != nil {
		e.Kind = "ip"
		switch {
		case ip == nil:
			e.Reason = nameNotIP
		case !pip.Equal(ip):
			e.Reason = fmt.Sprintf("%s is a different address", ip)
		case pport != "" && pport != port:
			e.Reason = fmt.Sprintf("entry is for port %s, request uses %s", pport, port)
		default:
			e.Matched, e.Reason = true, "same address"
		}
		return
	}

	if phost == "" {
		e.Kind, e.Reason = "ignored", "no host part"
		e.Warning = "net/http drops an entry without a host"
		return
	}

	e.Kind = "domain"
	e.Warning = domainWarning(p, phost, pport)

	name := phost
	if strings.HasPrefix(name, "*.") {
		name = name[1:]
	}
	matchHost := name[0] != '.'
	if matchHost {
		name = "." + name
	}
	if v, err := idnaASCII(name); err == nil {
		name = v
	}
	bare := name[1:]

	switch {
	case ip != nil:
		e.Reason = "host is an IP address, and domain entries only match names"
	case strings.HasSuffix(host, name) || (matchHost && host == bare):
		if pport != "" && pport != port {
			e.Reason = fmt.Sprintf("entry is for port %s, request uses %s", pport, port)
			return
		}
		e.Matched = true
		if host == bare {
			e.Reason = "same host"
		} else {
			e.Reason = fmt.Sprintf("subdomain of %s", bare)
		}
	case !matchHost && host == bare:
		e.Reason = fmt.Sprintf("a leading dot matches subdomains of %s only, not %s itself", bare, bare)
	default:
		e.Reason = fmt.Sprintf("%s is not %s or a subdomain of it", host, bare)
	}
}

// domainWarning flags domain entries that no URL can ever match.
func domainWarning(p, phost, pport string) string {
	switch {
	case strings.Contains(p, "://"):
		return "contains a URL scheme; NO_PROXY takes bare host names, so this entry never matches"
	case pport != "" && !isDigits(pport):
		return fmt.Sprintf("port %q is not a number, so this entry never matches", pport)
	case strings.Contains(phost, "/"):
		return "contains \"/\" but is not a valid CIDR block, so net/http compares it as a host name and it never matches"
	case strings.Contains(strings.TrimPrefix(phost, "*."), "*"):
		return "only a leading \"*.\" is a wildcard; any other \"*\" is compared literally, so this entry never matches"
	}
	return ""
}

// firstMatch returns the first matching entry, or nil.
func firstMatch(entries []Entry) *Entry {
	for i := range entries {
		if entries[i].Matched {
			return &entries[i]
		}
	}
	return nil
}

// notes lists the precedence and compatibility traps that apply to env.
func notes(env Env, u *url.URL, setting string, parsed *url.URL) []string {
	var out []string

	pairs := [][]string{noProxyVars}
	switch u.Scheme {
	case "https":
		pairs = append([][]string{httpsVars}, pairs...)
	case "http":
		pairs = append([][]string{httpVars}, pairs...)
	}
	for _, pair := range pairs {
		upper, lower := env.Get(pair[0]), env.Get(pair[1])
		if upper != "" && lower != "" && upper != lower {
			out = append(out, fmt.Sprintf("%s and %s are both set and differ: net/http uses %s; curl and golang.org/x/net/http/httpproxy.FromEnvironment prefer %s", pair[0], pair[1], pair[0], pair[1]))
		}
	}

	if u.Scheme == "http" && env.Get("HTTP_PROXY") != "" && env.Get("http_proxy") == "" {
		out = append(out, "only uppercase HTTP_PROXY is set: net/http uses it, but curl reads http_proxy in lowercase only")
	}
	if u.Scheme == "https" && setting == "" {
		if name, _ := first(env, httpVars); name != "" {
			out = append(out, fmt.Sprintf("%s is set but applies to http:// URLs only; https:// URLs need HTTPS_PROXY or https_proxy", name))
		}
	}
	if name, _ := first(env, allVars); name != "" {
		out = append(out, fmt.Sprintf("%s is set, but net/http ignores it; curl uses it for schemes without their own variable", name))
	}
	if env.Get("REQUEST_METHOD") != "" && u.Scheme == "https" {
		out = append(out, "REQUEST_METHOD is set: net/http treats the process as a CGI handler and refuses HTTP_PROXY for http:// URLs")
	}
	if parsed != nil && !supportedSchemes[parsed.Scheme] {
		out = append(out, fmt.Sprintf("proxy scheme %q is not one net/http supports (http, https, socks5, socks5h)", parsed.Scheme))
	}
	return out
}

// parseProxy mirrors httpproxy's parseProxy: a value without a scheme or
// host is retried with "http://" prepended.
func parseProxy(proxy string) (*url.URL, error) {
	if proxy == "" {
		return nil, nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Scheme == "" || u.Host == "" {
		if u2, err2 := url.Parse("http://" + proxy); err2 == nil {
			return u2, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// displayProxy renders a proxy setting with its password masked. A value
// that does not parse is not echoed, since it may still contain one.
func displayProxy(v string) string {
	if v == "" {
		return ""
	}
	u, err := parseProxy(v)
	if err != nil || u == nil {
		return "(does not parse)"
	}
	if u.User == nil && !strings.Contains(v, "://") {
		// Show the value as written; the "http://" was only implied.
		return v
	}
	return u.Redacted()
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

// idnaASCII mirrors httpproxy's helper of the same name.
func idnaASCII(v string) (string, error) {
	for i := 0; i < len(v); i++ {
		if v[i] >= 0x80 {
			return idna.Lookup.ToASCII(v)
		}
	}
	return v, nil
}
