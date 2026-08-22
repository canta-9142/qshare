# AGENTS.md

## Project

qshare is a local file-sharing CLI written in Go.

Its primary purpose is to let a computer exchange files with a smartphone without requiring a dedicated application on the smartphone.

Before changing behavior, read `docs/requirements.md` and the documents relevant
to the change:

* architecture or package boundaries: `docs/architecture.md`;
* HTTP, paths, uploads, tokens, archives, symlinks, or networking:
  `docs/security.md`;
* arguments, output streams, exit codes, signals, or defaults: `docs/cli.md`;
* implementation scope or planned features: `docs/roadmap.md`.

## Change authorization

Do not modify files when a task only asks for analysis, explanation, review, or
suggestions. Make repository changes only when the task explicitly requests an
implementation, fix, or documentation update.

## Development environment

Prefer the repository's pinned development environment:

```sh
nix develop
```

For non-interactive commands, run tools through the environment directly:

```sh
nix develop -c go test ./...
nix develop -c go vet ./...
```

Nix is recommended but not required when the host already provides a compatible
toolchain. Use `nix shell` instead of changing `flake.nix` for a tool that is
likely to be needed only once.

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
gofmt -w path/to/changed.go
```

Before completing an implementation task, run at minimum:

```sh
go test ./...
go vet ./...
```

Do not claim successful validation if these commands were not run successfully.
If validation cannot be completed, report the exact command that failed or was
not run and the reason.

## Scope discipline

Implement only behavior requested by the current task and consistent with the
documented current requirements. Roadmap entries marked as completed describe
existing behavior that must be preserved. Entries marked as planned are not
authorized for implementation unless the current task explicitly requests them.

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
