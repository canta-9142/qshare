# ADR 0001: Application ownership of session cleanup

Status: Accepted

## Context

HTTP shutdown, firewall cleanup, text processing, and terminal restoration had
separate owners. Startup failures also closed the server directly, making the
complete cleanup order difficult to follow. Text cleanup can block on output,
so placing terminal restoration last in a single sequential path would delay
restoration after a termination signal.

## Decision

Each application run keeps its acquired resources in a concrete `sessionRun`.
One deferred cleanup handles both partial startup and session termination in
this order: HTTP, firewall, text processing, shared files, terminal restoration.
HTTP drain policy depends on the exit reason; firewall cleanup has its own
bounded context. Errors accumulate while remaining cleanup continues.

CLI code supplies the quit notification and terminal restoration function.
One application goroutine waits for cancellation or normal cleanup notification,
restores the terminal, and returns the result through a channel. Cleanup receives
that result before returning. This keeps signal-triggered restoration independent
of blocking cleanup.

## Consequences

The existing `q`, expiration, and signal drain policies remain distinct.
Platform terminal operations and signal-to-exit-code mapping stay in CLI code.
HTTP shutdown no longer implicitly removes a firewall rule. No generic lifecycle
framework or cleanup registry is introduced.
