# ADR-0001: Use Go

Status: Accepted

## Context

qshare requires:

* a cross-platform command-line executable;
* local HTTP serving;
* streaming file I/O;
* cryptographically secure random generation;
* concurrency;
* graceful process cancellation;
* single-binary distribution;
* eventual platform-specific network adapters.

The project values implementation clarity and distribution simplicity more than low-level performance optimization.

## Decision

qshare will be implemented in Go.

The project will prefer pure Go code and avoid CGO unless a platform integration provides a concrete reason to use it.

## Consequences

### Positive

* strong standard library support for the core problem;
* simple HTTP implementation;
* straightforward concurrency model;
* easy static-ish self-contained deployment;
* convenient cross-compilation for pure-Go components;
* simple binary distribution;
* good fit for CLI/network utilities.

### Negative

* native platform integrations may require adapter libraries or external APIs;
* some OS-specific functionality may be more cumbersome than in lower-level native languages;
* binary size will be larger than a very small hand-written native program.

## Alternatives considered

### Rust

Rust provides excellent memory safety, native interoperability, and low-level control.

It was not selected because qshare's initial bottlenecks are network and user interaction rather than CPU or memory management, while Go offers a shorter path to the MVP.

Rust may be reconsidered only if future platform integration requirements materially invalidate the current assumptions.
