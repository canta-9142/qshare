# AGENTS.md

## Project

qshare is a local file-sharing CLI written in Go.

Its primary purpose is to let a computer exchange files with a smartphone without requiring a dedicated application on the smartphone.

Before making architectural or behavioral changes, read:

* `docs/requirements.md`
* `docs/architecture.md`
* `docs/security.md`
* `docs/cli.md`

For implementation priorities, also read:

* `docs/roadmap.md`

## Development commands

Build:

```sh
go build ./cmd/qshare
```

Test:

```sh
go test ./...
```

Static analysis:

```sh
go vet ./...
```

Format changed Go files with:

```sh
gofmt
```

Before completing a task, run at minimum:

```sh
go test ./...
go vet ./...
```

Do not claim successful validation if these commands were not run successfully.

## Scope discipline

Implement only:

1. behavior requested by the current task; and
2. behavior already specified by the current roadmap phase.

Do not proactively implement future roadmap phases.

Do not add speculative abstractions solely for hypothetical future requirements.

Prefer the smallest design that preserves the architectural boundaries described in `docs/architecture.md`.

## Design rules

* Keep `cmd/qshare` thin.
* Separate application orchestration from transport and platform code.
* Keep HTTP concerns out of session/domain logic.
* Keep platform-specific network control behind explicit interfaces.
* Prefer the Go standard library where practical.
* Avoid CGO unless there is a documented platform requirement.
* Do not require a background daemon for normal operation.
* Do not introduce cloud or external service dependencies.
* Stream file contents instead of buffering complete files in memory.
* Make cancellation explicit with `context.Context`.
* Return errors instead of logging from reusable packages.
* Keep package responsibilities narrow.
* Prefer composition over large multifunctional types.

## Dependency direction

The intended dependency direction is approximately:

```text
cmd/qshare
    ↓
internal/app
    ↓
core packages

adapters
    ↓
core interfaces
```

Core transfer/session logic must not depend on:

* CLI frameworks
* terminal rendering
* platform-specific hotspot implementations
* concrete HTTP request objects

## Security

Treat all remote input as untrusted.

Before modifying any of the following, read `docs/security.md`:

* HTTP handlers
* file path handling
* uploads
* authentication
* session tokens
* archives
* symlink behavior
* hotspot networking

Never:

* serve an arbitrary path supplied by an HTTP client;
* use filenames as authentication credentials;
* generate predictable session tokens;
* silently expose files outside the explicitly shared set;
* overwrite existing local files during upload without an explicit policy;
* weaken a security invariant merely to simplify a handler.

Security-sensitive pure logic should have unit tests.

## CLI behavior

CLI behavior is a public interface.

Before changing:

* arguments
* stdout
* stderr
* exit codes
* signal handling
* default modes

read `docs/cli.md`.

Do not print status or progress information to stdout when stdout is reserved for useful program output.

## Platform code

Platform-specific implementations belong under the platform abstraction.

Do not spread OS checks throughout core packages.

Prefer build-tagged files or explicit platform adapters where appropriate.

## Documentation

If a change alters externally observable behavior, update the relevant documentation in the same change.

Architectural decisions with meaningful long-term consequences should receive an ADR under `docs/adr/`.
