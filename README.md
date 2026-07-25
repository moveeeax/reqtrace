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

A `Host` header given with `-H` sets the request's `Host` field, so you can
probe a specific backend address while presenting a different virtual host.

`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` are honoured.

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
