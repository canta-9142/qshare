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
stderr, handles termination signals, and maps errors to exit codes. Terminal initialization supplies a quit
notification and restoration function to the application; CLI code implements
terminal operations but does not decide when to restore the terminal.

### `internal/app`

Owns operation orchestration:

1. open and validate resources;
2. configure receive and platform adapters;
3. determine the advertised LAN address;
4. create the session handler, reserve a LAN listener, configure its firewall
   rule, and start a standard HTTP server;
5. render the authenticated URL as a QR code;
6. wait for expiration, a signal, or a server failure;
7. drain or stop HTTP, remove the firewall rule, finish text processing, release
   shared resources, and restore the terminal.

Tests replace listener acquisition and external platform operations. HTTP
handlers are built directly, without mode-specific server factories.

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

Mode constructors return `http.Handler`. A small `NewHTTPServer` function applies
the HTTP timeout and header limits and returns a standard `*http.Server`.
This package does not bind listeners, launch serving goroutines, or own shutdown
notifications; those belong to application orchestration.

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

Each `Application.Run` owns a concrete `sessionRun` containing its acquired
resources. Startup errors and session termination both pass through the same
cleanup in `internal/app/lifecycle.go`. Application startup selects and reserves
a LAN port, retaining the numeric port for the firewall rule and advertised URL.
After firewall setup succeeds, it runs `http.Server.Serve` on that listener and
records the result through a channel. Cleanup closes the listener even if serving
never started, waits for serving to return, and removes the firewall rule with a
separate five-second timeout. Cleanup errors are joined without skipping later
resource releases.

On expiration, HTTP drains for at most 30 seconds using a context independent
of signals, then text processing is canceled. On `q`, HTTP drains for at most
30 seconds using the session context; if HTTP and firewall cleanup succeed,
accepted text submissions are drained using that context. Signals interrupt
this interactive drain. Other exit paths close HTTP and cancel text processing.

One application goroutine restores the terminal after either cancellation or a
normal cleanup notification. Signals therefore restore it even during an
expiration drain or blocked text output. `Run` receives the restoration result
before returning and includes any error in its result.
Reusable packages return errors instead of logging.

## Design constraints

- Prefer the Go standard library where practical.
- Avoid CGO and background daemons.
- Keep HTTP types out of session and resource logic.
- Keep network and clipboard integrations replaceable.
- Add abstractions only for current behavior or a documented roadmap item.
