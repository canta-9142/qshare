# ADR-0004: Phase 1 Session Lifecycle

Status: Accepted

## Context

An HTTP file transfer may involve `HEAD`, one or more `GET` requests, range
requests, retries, or browser behavior that does not correspond to one observable
"download" operation.

Ending a session after the first successful response would make retries fragile
and would require qshare to define a logical-download abstraction that the Phase
1 requirements do not otherwise need.

## Decision

In Phase 1, a successful download does not end the session.

`GET`, `HEAD`, requests containing a `Range` header, and retries are independent
HTTP requests within the same authenticated session. Phase 1 has no concept of a
logical single download.

A session ends only because of:

* expiration;
* SIGINT;
* SIGTERM; or
* a fatal internal error that prevents safe continuation.

At expiration, qshare stops accepting new requests and permits requests already
in progress to complete. Expiration is normal completion and produces exit
status `0`.

The Phase 1 lifetime defaults to ten minutes. `--expire DURATION` overrides the
default using Go duration syntax; the value must be greater than zero.

After graceful cleanup, SIGINT produces exit status `130`, SIGTERM produces exit
status `143`, and a fatal internal error produces exit status `1`.

The meaning of a possible future `--once` flag is not decided by this ADR.

## Consequences

### Positive

* browser retries and range-based transfers remain reliable;
* the session model stays independent of browser-specific download behavior;
* session termination and process exit behavior are deterministic;
* expiration can drain in-progress transfers without admitting new work.

### Negative

* a resource remains available after a successful download until another session
  termination condition occurs;
* server shutdown needs separate states for accepting new work and draining
  existing work;
* tests must cover request admission at the expiration boundary.

## Deferred decisions

This ADR does not define:

* `--once` semantics;
* a logical download or transfer aggregate;
* whether later phases record download history or counts.
