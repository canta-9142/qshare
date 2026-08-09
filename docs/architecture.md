# Architecture

## 1. Design objectives

The implementation should remain small enough to understand as a command-line utility while preserving clear boundaries between:

* CLI;
* application orchestration;
* sessions;
* file transfer;
* HTTP transport;
* QR rendering;
* platform networking.

qshare should avoid both extremes:

* a single `main.go` containing every responsibility;
* an elaborate enterprise architecture for a small executable.

## 2. High-level architecture

```text
cmd/qshare
    │
    ▼
internal/app
    │
    ├─────────────┬─────────────┐
    ▼             ▼             ▼
 session       transfer       bootstrap
                  │
                  ▼
               server
                  │
                  ▼
                HTTP

       platform/network
              │
              ▼
        OS integration
```

QR rendering is an output adapter used by application orchestration.

## 3. Proposed repository structure

```text
cmd/
└── qshare/
    └── main.go

internal/
├── app/
│   └── app.go
│
├── session/
│   ├── session.go
│   └── token.go
│
├── share/
│   ├── resource.go
│   └── file.go
│
├── server/
│   ├── server.go
│   ├── download.go
│   └── upload.go
│
├── qr/
│   └── render.go
│
└── platform/
    └── network/
        ├── network.go
        ├── linux.go
        ├── windows.go
        └── darwin.go

web/
├── index.html
├── style.css
└── app.js

docs/
└── ...
```

The exact package layout may evolve as actual responsibilities become clear.

Do not create empty packages merely to match this diagram.

## 4. `cmd/qshare`

Responsibilities:

* parse command-line arguments;
* construct application dependencies;
* invoke the application;
* map final errors to CLI exit behavior.

It should not contain:

* HTTP handlers;
* token algorithms;
* filesystem authorization logic;
* hotspot implementation;
* significant business rules.

## 5. Application layer

`internal/app` coordinates a sharing session.

Example responsibilities:

1. validate requested operation;
2. build a session;
3. choose a network strategy;
4. start the server;
5. construct an access URL;
6. render the QR code;
7. wait for completion, cancellation, or expiration;
8. shut everything down.

For Phase 1, successful HTTP requests do not transition the session to a
completed state. `GET`, `HEAD`, ranged requests, and retries are independent
requests authorized by the same live session.

The application layer may depend on interfaces implemented by adapters.

## 6. Session package

The session package owns session identity and authorization state.

It should understand concepts such as:

* token;
* creation time;
* expiration;
* allowed resources;
* session state.

It should not depend on:

* `net/http`;
* terminal APIs;
* QR libraries;
* NetworkManager;
* Windows APIs.

## 7. Shared resources

The server must not translate arbitrary client-controlled path strings into arbitrary filesystem access.

At session creation time, requested files should be validated and converted into explicit share resources.

HTTP routing should resolve only against those resources.

Conceptually:

```text
CLI path
   ↓
validation
   ↓
ShareResource
   ↓
Session
   ↓
opaque resource ID
   ↓
HTTP request
```

not:

```text
HTTP path
   ↓
filesystem path
```

## 8. HTTP server

The HTTP server is an adapter around session and resource behavior.

Responsibilities include:

* request parsing;
* authentication extraction;
* response headers;
* streaming bodies;
* protocol-level error mapping.

Authorization decisions should remain explicit and testable.

Use standard Go HTTP primitives unless a concrete requirement justifies another dependency.

## 9. Streaming

Large files must be transferred as streams.

Avoid:

```go
data, err := os.ReadFile(path)
```

for normal file serving.

Prefer an open file or equivalent reader whose contents are copied directly into the HTTP response.

## 10. QR rendering

QR rendering receives a final bootstrap payload, normally a URL or Wi-Fi bootstrap string.

It must not independently construct security credentials.

Token and URL construction belong elsewhere.

The terminal renderer should write UI/status output to stderr unless CLI semantics require otherwise.

## 11. Platform networking

Platform-specific networking belongs behind an interface.

Conceptually:

```go
type NetworkProvider interface {
    Prepare(ctx context.Context, request Request) (Network, error)
}
```

A resulting `Network` may expose:

* local bind address;
* public-to-peer URL address;
* cleanup method or lifecycle tied to context.

LAN discovery and Direct Mode are separate network strategies.

OS-specific implementations must not leak into transfer/session packages.

## 12. Cancellation and cleanup

Long-running operations must accept `context.Context`.

SIGINT/SIGTERM handling belongs near the CLI/application boundary.

Shutdown order should ensure that temporary resources are cleaned up.

When a session expires, the server must stop accepting new requests while
allowing requests already in progress to finish. SIGINT, SIGTERM, and fatal
errors initiate shutdown through the same application-level lifecycle, although
their final exit statuses differ as defined in `docs/cli.md`.

Direct Mode eventually requires cleanup of:

* HTTP server;
* DHCP/DNS helpers if used;
* hotspot;
* temporary network configuration.

Cleanup must be safe to call after partial initialization.

## 13. Embedded Web UI

Web assets should be bundled using Go's embedding support.

The released executable should not require a neighboring `web/` directory.

The Web UI should remain simple and should not introduce a heavy frontend build pipeline unless a concrete requirement emerges.

Plain HTML/CSS and minimal JavaScript are preferred initially.

## 14. Logging and output

Reusable packages return errors rather than choosing user-facing wording.

Application or CLI layers decide:

* status output;
* diagnostic output;
* exit code.

Avoid global loggers in core packages.

## 15. Testing layers

### Unit tests

Prioritize pure behavior:

* token handling;
* expiration;
* resource lookup;
* path validation;
* authorization;
* filename sanitization.

### HTTP tests

Use `net/http/httptest` where appropriate.

### Integration tests

Test:

* real streaming;
* process cancellation;
* LAN binding;
* platform-specific network behavior.

Direct Mode will require platform-dependent integration testing.

## 16. Architectural invariants

The following should remain true:

1. session logic does not depend on HTTP;
2. core logic does not depend on a specific OS;
3. client input never directly selects an arbitrary filesystem path;
4. large files are not fully buffered;
5. normal operation requires no persistent daemon;
6. release operation does not require external infrastructure.
