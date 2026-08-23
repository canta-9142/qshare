# Development Guide

## Environment

qshare requires Go 1.24 or newer and Git. Confirm that both are available:

```sh
go version
git --version
```

No environment manager or system-wide service is required. All development,
testing, static analysis, and local builds use the standard Go toolchain.

## Common commands

```sh
# Build
go build ./cmd/qshare

# Run
go run ./cmd/qshare ./example.txt

# Test and analyze
go test ./...
go vet ./...

# Test the Linux installer
sh tests/install_test.sh

# Format changed Go files
gofmt -w path/to/changed.go
```

Before completing a change, `go test ./...` and `go vet ./...` must both pass.
Do not report them as successful unless they were actually run successfully.

## Project conventions

- Read `AGENTS.md` and the relevant design documents before changing behavior.
- Keep `cmd/qshare` thin and preserve the dependency direction described in
  [architecture.md](architecture.md).
- Read [security.md](security.md) before changing handlers, paths, uploads,
  tokens, archives, symlink behavior, or networking.
- Read [cli.md](cli.md) before changing arguments, output streams, exit codes,
  defaults, or signals.
- Update documentation with externally observable changes.
- Record consequential architectural decisions under `docs/adr/`.

Prefer the standard library, avoid CGO unless a documented platform requirement
justifies it, and do not add cloud services or a required background daemon.

## Testing

Tests live beside the implementation in `*_test.go` files and use the standard
`testing` package by default.

- Use table-driven tests when several cases exercise the same contract.
- Mark helpers with `t.Helper()` and use `t.Cleanup()` for owned cleanup.
- Prefer `t.TempDir()` for filesystem tests.
- Keep tests deterministic and independent of the Internet and host state.
- Assert observable behavior rather than incidental implementation details.
- Use `net/http/httptest` for handler tests.
- Do not run tests in parallel when they share process-global or host state.

Security-sensitive logic needs focused unit tests. Network mutation, elevated
privileges, dedicated hardware, or Internet access must be opt-in behind an
explicit build tag and documented beside the test.

## Dependencies and assets

The Web UI is embedded in the executable. Runtime installation must not require
separate template or static-asset files.

External dependencies are appropriate when the standard library cannot
reasonably provide the behavior. The current CLI parser and QR implementation
are deliberate dependencies; straightforward HTTP, I/O, cryptography, and
context handling should continue to use the standard library.

## Builds and packaging

Canonical release targets are:

```text
linux/amd64
linux/arm64
```

Release binaries are self-contained and built with CGO disabled. Open Build
Service uses `qshare.spec` for RPM builds and `qshare.dsc` with the `debian.*`
files for Debian builds. `PKGBUILD` contains the Arch recipe, and `flake.nix`
provides Nix packaging. Packaging should not become the only supported
installation path; release binaries and source builds remain available.

Distribution builds set the version displayed by `qshare --version` with the
Go linker, for example:

```sh
go build -ldflags "-X main.version=v0.6.0" ./cmd/qshare
```

An ordinary unstamped development build reports `qshare devel`.

## Publishing a release

Stable releases are published by `.github/workflows/release.yml`. Before
tagging, update the package versions and release documentation in the same
commit. Create and push a stable semantic-version tag:

```sh
git tag -a v0.7.0 -m "qshare v0.7.0"
git push origin v0.7.0
```

The workflow accepts tags such as `v0.7.0`; prerelease tags are not supported.
It runs the tests, vet, and race detector before building self-contained Linux
binaries for amd64 and arm64. It then publishes these assets to a GitHub
Release:

```text
install.sh
qshare-linux-amd64
qshare-linux-arm64
checksums.txt
```

The release is created as a draft and published only after all assets have
uploaded successfully. The installer downloads a binary from the selected
release, verifies its SHA-256 checksum, and replaces the destination only after
verification succeeds.

## Typical workflow

```text
read relevant docs → make a scoped change → format → test → vet → update docs
```

Do not implement future roadmap phases or speculative abstractions as part of an
unrelated task.
