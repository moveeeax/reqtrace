# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.1] - 2026-11-02

### Security

- A password in a `https://user:pass@host` URL is masked in text and JSON
  output and in URL validation errors. v0.1.0 printed it in full.
- With `-follow`, `Authorization`, `Proxy-Authorization` and `Cookie` are
  dropped on a redirect that leaves TLS, including an `https://` to `http://`
  hop back to the same host.

### Fixed

- HTTPS requests negotiate HTTP/2 again; the custom transport had disabled it.
- `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY` are honoured.
- `-follow` stops after 10 redirects, as `net/http` does by default.
- A negative `-timeout` is rejected instead of disabling the timeout; an
  explicit `0` prints a warning.
- `-H "Host: ..."` sets the request's Host instead of being discarded.
- The trace recorder no longer crashes with "concurrent map writes" when
  IPv4 and IPv6 dials race, and a failed dial attempt no longer sets the
  reported TCP connect time.

### Changed

- Minimum TLS version is 1.2.
- `-insecure` prints a warning to stderr.
- The module requires Go 1.26 or later (was 1.17).

## [0.1.0] - 2026-07-18

First release.

[Unreleased]: https://github.com/moveeeax/reqtrace/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/moveeeax/reqtrace/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/moveeeax/reqtrace/releases/tag/v0.1.0
