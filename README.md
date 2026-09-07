# quickserve

[![CI](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml/badge.svg)](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Quenwaz/quickserve)](https://github.com/Quenwaz/quickserve/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md)

A zero-dependency static file server in a single binary — like `python -m http.server`, but faster to deploy, with file upload, CORS support and LAN-friendly output. Written in pure Go standard library, no third-party packages.

## Why quickserve?

`python -m http.server` requires a Python runtime and can only *send* files. quickserve compiles to one static binary that runs anywhere — and it can also *receive* files:

| | `python -m http.server` | quickserve |
|---|---|---|
| Runtime needed | Python | **none** (single binary) |
| File upload | ❌ | ✅ `POST` / `PUT` |
| CORS headers | ❌ | ✅ `-c` |
| LAN addresses on startup | ❌ | ✅ |
| Windows / macOS / Linux / ARM | ✅ (if Python installed) | ✅ prebuilt binaries |
| Access log (CLF-style) | ✅ | ✅ |

## Install

**Option 1 — download a binary** from [Releases](https://github.com/Quenwaz/quickserve/releases): pick your platform, unzip, run. Every release ships with SHA-256 checksums.

**Option 2 — with Go installed:**

```sh
go install github.com/Quenwaz/quickserve@latest
```

## Quick start

```sh
quickserve                # serve current dir on :8000
quickserve 9999           # positional port, same as python -m http.server
quickserve -u -p 9000     # also accept uploads on :9000
```

Upload a file (requires `-u`):

```sh
curl -T big.zip http://localhost:9000/big.zip
# or from any device on the LAN, using the address printed at startup
```

## Usage

```
quickserve [port] [directory]
quickserve [options]

  -p, -port <N>      listen port (default 8000)
  -d, -dir <path>    directory to serve (default ".")
  -u, -upload        enable file upload via POST/PUT (default off)
  -c, -cors          enable CORS headers (default off)
  -v, -version       print version and exit
  -h, -help          show this help
```

## Security notes

- Upload is **off by default**; enable it explicitly with `-u`.
- Upload paths containing `..` are rejected (403) — requests cannot escape the served directory.
- Upload only accepts `POST`/`PUT`; directory listings never allow writing.
- Like `python -m http.server`, traffic is plain HTTP — don't serve secrets over untrusted networks.

## Development

```sh
go build -o quickserve .          # build locally
go test ./...                     # run the test suite
gofmt -l . && go vet ./...        # same checks as CI
```

Releases are built automatically by GitHub Actions when a `v*` tag is pushed — see [release workflow](.github/workflows/release.yml).

## License

[MIT](LICENSE) © Quenwaz
