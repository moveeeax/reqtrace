# reqtrace

A small command-line tool that performs an HTTP request and prints a per-phase
timing breakdown — DNS lookup, TCP connect, TLS handshake, time-to-first-byte,
and body download — using Go's `net/http/httptrace`.

## Install

```sh
go install github.com/moveeeax/reqtrace@latest
```

Or build from source:

```sh
git clone https://github.com/moveeeax/reqtrace
cd reqtrace
go build -o reqtrace .
```

## Usage

```sh
reqtrace [flags] <url>
```

### Flags

| Flag         | Default | Description                                       |
|--------------|---------|---------------------------------------------------|
| `-method`    | `GET`   | HTTP method to use                                |
| `-H`         |         | Request header `Key: Value` (repeatable)          |
| `-timeout`   | `30s`   | Overall request timeout; `0` disables it          |
| `-body`      |         | Request body: literal string or `@file`           |
| `-follow`    | `false` | Follow HTTP redirects (at most 10)                |
| `-insecure`  | `false` | Skip TLS certificate verification                 |
| `-json`      | `false` | Emit machine-readable JSON                        |
| `-n`         | `1`     | Number of requests; above 1 prints percentiles    |
| `-interval`  | `0`     | Pause between requests with `-n`                  |
| `-reuse`     | `false` | With `-n`, keep connections and TLS sessions warm |
| `-explain-proxy` | `false` | Explain which proxy applies; send no request  |
| `-env-file`  |         | Read proxy variables from a `KEY=VALUE` file      |

A `Host` header given with `-H` sets the request's `Host` field, so you can
probe a specific backend address while presenting a different virtual host.

`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` are honoured, and the report
names the proxy the request went through (`direct` when none). With
`-env-file`, those variables come from the file and the process environment
is not consulted for them.

### Examples

Basic timing of a GET request:

```sh
reqtrace https://example.com
```

POST with headers and a body from a file, following redirects:

```sh
reqtrace -method POST -H "Content-Type: application/json" \
  -body @payload.json -follow https://api.example.com/v1/things
```

Twenty requests half a second apart, with min, p50, p95 and max per phase:

```sh
reqtrace -n 20 -interval 500ms https://example.com
```

Why is this URL (not) going through the proxy?

```sh
reqtrace -explain-proxy -env-file ./proxy.env https://registry.example.com/v2/
```

Machine-readable output for scripting:

```sh
reqtrace -json https://example.com | jq .
```

## Output

Text mode prints the final URL, status, protocol, content length, and the
elapsed time of each request phase. JSON mode emits the same data as a single
object suitable for piping into `jq` or a metrics pipeline.

The response body is counted, not stored, so probing a large download costs no
memory. Request and response headers are never printed, and a password in a
`https://user:pass@host` URL is masked in every report — reqtrace output is
safe to paste into a ticket or leave in a CI log.

## Repeat mode

`-n N` sends the request N times and prints min, p50, p95 and max for each
phase. Percentiles use the nearest-rank method, so every number shown is a
measured run. `-timeout` applies to each request, and the first failed run
stops the series with an error naming it. Status, protocol and body size are
those of the last run.

Without `-reuse`, every run opens a new connection and does a full TLS
handshake, the same as N separate invocations. With `-reuse`, runs share one
client: a kept-alive connection serves the next run, and when the server has
closed it, the new handshake resumes the earlier TLS session. The `Runs:` line
counts both. A phase a run skipped (DNS, connect and TLS on a reused
connection) counts as 0 ms for that run, so with `-reuse` the p50 of those
phases is usually zero.

JSON output gains a `stats` object:

```json
"stats": {
  "runs": 20,
  "reused_connections": 19,
  "tls_resumed": 0,
  "timings_ms": {
    "dns_lookup": {"min": 0, "p50": 0, "p95": 0, "max": 11.2},
    "...": {}
  }
}
```

## Proxy explanation

`-explain-proxy` sends nothing. It prints which variable supplies the proxy
for the URL's scheme, then walks `NO_PROXY` entry by entry: how net/http
parsed the entry (domain, IP, CIDR block, `*`, or dropped as malformed),
whether it matches the URL's host and port, and why. The verdict comes from
the same code as `http.ProxyFromEnvironment`, so it is what a Go client will
do; a test compares every rule against the real function.

It also flags the usual traps:

- an entry that can never match, such as `https://example.com`,
  `10.0.0.*` or `10.0.0.0/33`;
- `.example.com` does not cover `example.com`, CIDR and IP entries never
  match a host name (nothing is resolved), and name entries never match an
  IP address;
- `HTTP_PROXY` and `http_proxy` both set with different values: Go's
  `net/http` reads the uppercase name first, while curl and
  `golang.org/x/net/http/httpproxy.FromEnvironment` prefer lowercase, and
  curl reads `http_proxy` only in lowercase;
- `HTTP_PROXY` set for an `https://` URL, which it does not cover;
- `ALL_PROXY`, which curl honours and Go ignores;
- `REQUEST_METHOD` in the environment, which makes net/http refuse
  `HTTP_PROXY` (the CGI "httpoxy" guard).

`-env-file` reads `KEY=VALUE` lines (blank lines and `#` comments skipped, an
optional `export ` prefix, surrounding quotes removed) instead of the
environment, for debugging a container or service configuration offline.
Proxy passwords are masked in all output.

## Redirects and credentials

With `-follow`, reqtrace chases at most 10 redirects. Credential headers
(`Authorization`, `Proxy-Authorization`, `Cookie`) are dropped when a hop
leaves TLS behind — for example an `https://` → `http://` redirect back to the
same host, which Go's own cross-domain check does not treat as a change of
origin. `net/http` separately drops them when the redirect crosses to a
different domain.

`-insecure` prints a warning to stderr; it turns off certificate verification
for the whole exchange, including every redirect hop.

## License

[MIT](LICENSE)
