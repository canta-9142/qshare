# Architecture

## Overview

qshare is a small layered CLI. The command entry point delegates to application
orchestration, which coordinates narrow domain packages and adapters.

```text
cmd/qshare
    ↓
internal/cli
    ↓
internal/app
    ├── session
    ├── share
    ├── receive
    ├── server (HTTP adapter)
    ├── qr (terminal adapter)
    └── platform
        ├── clipboard
        └── network
```

Dependencies point inward: transport, terminal, and platform packages may use
core types, but core session and resource logic must not depend on those
adapters.

## Package responsibilities

### `cmd/qshare`

Contains only process startup and exit delegation. It must not contain HTTP,
filesystem authorization, or network-selection logic.

### `internal/cli`

Parses arguments and stdin, maps them to an `app.Request`, routes stdout and
stderr, handles termination signals, and maps errors to exit codes.

### `internal/app`

Owns operation orchestration:

1. open and validate resources;
2. configure receive and platform adapters;
3. determine the advertised LAN address;
4. create a session and HTTP server;
5. render the authenticated URL as a QR code;
6. wait for expiration, a signal, or a server failure;
7. close resources and drain or stop the server.

Application code depends on constructors and interfaces so security-sensitive
logic and lifecycle behavior remain testable.

### `internal/session`

Owns the session token, expiry, operation resources, and authorization checks.
It has no HTTP, terminal, or OS-networking dependency.

### `internal/share`

Turns CLI-selected files, directories, and text into validated resources.
Files and directory nodes receive opaque IDs. Directory sessions retain a
startup-time authorization tree and filesystem identity for each included
object.

HTTP input resolves a token and opaque resource ID:

```text
CLI path → validated resource → session → opaque ID → HTTP lookup
```

It must never become:

```text
HTTP input → local filesystem path
```

### `internal/receive`

Publishes uploads safely inside one configured directory and serializes text
submission processing. It owns size limits, collision naming, temporary-file
cleanup, and text sinks.

### `internal/server`

Adapts sessions and resources to `net/http`. It parses requests, authenticates
tokens, maps errors to HTTP responses, escapes browser output, and streams
files and ZIP archives. It does not decide which local paths are shareable.

Browser templates are embedded from `internal/server/web`, keeping the binary
self-contained.

### Platform and output adapters

- `internal/platform/network` selects a usable Linux IPv4 LAN address.
- `internal/platform/clipboard` invokes supported clipboard tools directly,
  without a shell.
- `internal/qr` renders an already constructed URL to the terminal; it does not
  create credentials.

Platform-specific behavior stays behind these package boundaries. Future OS
support should use build-tagged files or explicit adapters rather than OS
checks throughout core packages.

## Lifecycle and streaming

The application passes `context.Context` through cancellable work. File and ZIP
responses stream data rather than buffering complete content. A normal download
does not mutate or complete the session, so retries, `HEAD`, and range requests
remain independent while the token is valid.

On expiration, the HTTP server drains for at most 30 seconds. Signal handling
closes it immediately through the same application lifecycle. Reusable packages
return errors instead of logging.

## Design constraints

- Prefer the Go standard library where practical.
- Avoid CGO and background daemons.
- Keep HTTP types out of session and resource logic.
- Keep network and clipboard integrations replaceable.
- Add abstractions only for current behavior or a documented roadmap item.
