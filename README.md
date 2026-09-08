# quickserve

[![CI](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml/badge.svg)](https://github.com/Quenwaz/quickserve/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Quenwaz/quickserve)](https://github.com/Quenwaz/quickserve/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md)

A zero-dependency static file server in a single binary — like `python -m http.server`, but faster to deploy, with safe file upload for **any file type**, CORS support and LAN-friendly output. Written in pure Go standard library, no third-party packages. Ships as a ~7 MB binary or a ~6.5 MB `scratch` container image.

## Why quickserve?

`python -m http.server` requires a Python runtime and can only *send* files. quickserve compiles to one static binary that runs anywhere — and it can also *receive* files:

| | `python -m http.server` | quickserve |
|---|---|---|
| Runtime needed | Python | **none** (single binary) |
| File upload (any type) | ❌ | ✅ `POST` / `PUT` + browser page |
| Multipart form upload | ❌ | ✅ streaming, constant memory |
| Upload size cap / rate limit / token auth | ❌ | ✅ built in |
| CORS headers | ❌ | ✅ `-c` |
| LAN addresses on startup | ❌ | ✅ |
| Health endpoint for containers | ❌ | ✅ `/health` |
| Access log (CLF-style) | ✅ | ✅ |

## Install

**Option 1 — download a binary** from [Releases](https://github.com/Quenwaz/quickserve/releases): pick your platform, unzip, run. Every release ships with SHA-256 checksums.

**Option 2 — with Go installed:**

```sh
go install github.com/Quenwaz/quickserve@latest
```

**Option 3 — Docker:**

```sh
docker pull ghcr.io/Quenwaz/quickserve:latest
docker run --rm -p 8000:8000 -v "$PWD/data:/data" ghcr.io/Quenwaz/quickserve
```

## Quick start

```sh
quickserve                # serve current dir on :8000 (download only)
quickserve -u             # + upload enabled, browser page at /_upload
quickserve -u -p 9000     # uploads on :9000
```

Send files:

```sh
# browser: open http://<host>:8000/_upload — pick or drag files, watch progress
curl -T big.zip http://localhost:9000/big.zip                 # raw body
curl -F file=@photo.jpg http://localhost:9000/                # multipart form
```

Uploads are refused if the target file already exists with content (409) — nothing gets overwritten by accident.

## Usage

```
quickserve [port] [directory]
quickserve [options]

  -p, -port <N>          listen port (default 8000)
  -d, -dir <path>        directory to serve (default ".")
  -u, -upload            enable file upload via POST/PUT (default off)
  -c, -cors              enable CORS headers (default off)
  -t, -token <secret>    require token for upload
                         (Authorization: Bearer <secret> / X-Auth-Token / ?token=)
  -m, -max-upload <size> per-file size cap (default 1GiB; 0 = unlimited; e.g. 100MB)
  -r, -rate-limit <N>    per-IP uploads per minute (default 60; 0 = unlimited)
  -ro, -read-only        block downloads too; only /health responds
  -v, -version           print version and exit
  -h, -help              show this help
```

### Endpoints

| Path | Methods | Purpose |
|---|---|---|
| `/` and files | GET, HEAD | static files & directory listing |
| `/<path>` | POST, PUT | upload to that path (`-u`) |
| `/` | POST (multipart) | browser/form upload (`-u`) |
| `/_upload` | GET | built-in upload page (`-u`) |
| `/health` | GET | liveness probe, always on |

## Protections

Upload is opt-in and guarded by several independent layers:

- **Path traversal** — `..` segments rejected (403); names are cleaned to a single safe basename per segment; Windows reserved device names (`CON`, `NUL`, `COM1`…) rejected; invalid characters replaced.
- **Overwrite protection** — existing non-empty files are never replaced (409). Failed uploads leave **no partial files**: content is written to a temp file, fsynced, then atomically renamed.
- **Size cap** — per-file limit via `-m` (default 1 GiB). Oversized requests are rejected with 413; known-length bodies are refused before reading a byte.
- **Token auth** — `-t` requires a secret via `Authorization: Bearer`, `X-Auth-Token`, or `?token=` (constant-time comparison).
- **Rate limiting** — per-IP sliding window via `-r` (default 60/min), with bounded memory even against spoofed-source floods.
- **Slow-client hardening** — 10 s read-header timeout (Slowloris), idle-connection reaping, and a 15 min cap on stalled uploads.
- **Graceful shutdown** — SIGINT/SIGTERM stops accepting connections and waits for in-flight uploads to land on disk.

## Docker

The image is built in two stages: Go compiles a statically-linked binary (`CGO_ENABLED=0`), then it is copied into a bare **`scratch`** base image — no shell, no package manager, no libc, not even CA certificates — running as **UID 65532 (non-root)**. Typical size ≈ 6.5 MB, with a smaller attack surface than distroless.

```sh
docker build -t quickserve .
docker run --rm -p 8000:8000 -v "$PWD/data:/data" quickserve
```

Hardened compose setup ([docker-compose.yml](docker-compose.yml)): read-only root filesystem, all capabilities dropped, `no-new-privileges`, tmpfs for `/tmp`, memory/CPU limits. The container exposes `/health` and passes `HEALTHCHECK` natively — no curl needed inside the image.

Pass options via the command list, e.g.:

```yaml
command: ["-d", "/data", "-u", "-t", "s3cr3t", "-m", "500MB"]
```

## Project layout

```
cmd/quickserve/       entry point: flag parsing, exit codes
internal/config/      options, parsing, validation
internal/server/      routing, CORS, logging, lifecycle, banner
internal/upload/      upload handlers, sanitization, rate limiter
internal/web/         embedded upload page (go:embed)
```

## Development

```sh
go build -o quickserve ./cmd/quickserve   # build locally
go test -race ./...                       # run the test suite
gofmt -l . && go vet ./...                # same checks as CI
```

Binary releases are built by [release workflow](.github/workflows/release.yml); multi-arch container images are published to GHCR by [docker-release workflow](.github/workflows/docker-release.yml) when a `v*` tag is pushed.

## License

[MIT](LICENSE) © Quenwaz
