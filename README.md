# reqtrace

A small command-line tool that performs an HTTP request and prints a per-phase
timing breakdown — DNS lookup, TCP connect, TLS handshake, time-to-first-byte,
and body download — using Go's `net/http/httptrace`.

## Install

```sh
go install github.com/cybercapybara/reqtrace@latest
```

Or build from source:

```sh
git clone https://github.com/cybercapybara/reqtrace
cd reqtrace
go build -o reqtrace .
```

## Usage

```sh
reqtrace [flags] <url>
```

### Flags

| Flag         | Default | Description                                    |
|--------------|---------|------------------------------------------------|
| `-method`    | `GET`   | HTTP method to use                             |
| `-H`         |         | Request header `Key: Value` (repeatable)       |
| `-timeout`   | `30s`   | Overall request timeout                        |
| `-body`      |         | Request body: literal string or `@file`        |
| `-follow`    | `false` | Follow HTTP redirects                          |
| `-insecure`  | `false` | Skip TLS certificate verification              |
| `-json`      | `false` | Emit machine-readable JSON                     |

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

## License

[MIT](LICENSE)
