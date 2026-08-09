# Development Guide

## 1. Requirements

Primary development environment:

* current stable Go toolchain
* Git

Optional:

* Nix with flakes

The implementation should remain buildable without Nix.

## 2. Clone

```sh
git clone <repository-url>
cd qshare
```

## 3. Build

```sh
go build ./cmd/qshare
```

For a local binary:

```sh
go build -o ./build/qshare ./cmd/qshare
```

## 4. Run

```sh
go run ./cmd/qshare ./example.txt
```

## 5. Test

```sh
go test ./...
```

Tests should not require Internet access unless explicitly marked as external integration tests.

## 6. Static analysis

```sh
go vet ./...
```

## 7. Formatting

Use `gofmt`.

For example:

```sh
gofmt -w ./cmd ./internal
```

Do not introduce an alternate Go formatter without a clear project-level reason.

## 8. Nix

The repository should eventually expose a development shell:

```sh
nix develop
```

and application execution:

```sh
nix run . -- FILE
```

Nix is an additional development and distribution interface, not a mandatory runtime dependency.

## 9. Dependency policy

Prefer the standard library.

Likely justified external dependencies include:

* QR encoding/rendering;
* platform APIs not reasonably accessible through the standard library.

Avoid dependencies for functionality already straightforward in:

* `net/http`;
* `crypto/rand`;
* `io`;
* `os`;
* `context`;
* `embed`.

CLI parsing uses the `go-arg` library.

## 10. CGO

The default build should avoid CGO.

Platform-specific integration may justify CGO later, but such a decision must:

1. document why a pure Go mechanism is insufficient;
2. describe distribution consequences;
3. receive an ADR if architecturally significant.

## 11. Release build

Canonical releases should be self-contained executables with embedded Web UI assets.

Initial Linux targets:

```text
linux/amd64
linux/arm64
```

Long-term targets:

```text
linux/amd64
linux/arm64
windows/amd64
darwin/amd64
darwin/arm64
```

Release archives should contain the executable and any mandatory legal notices.

Runtime Web UI files should not need to be installed separately.

## 12. Suggested release artifacts

```text
qshare_0.1.0_linux_amd64.tar.gz
qshare_0.1.0_linux_arm64.tar.gz
checksums.txt
```

Later:

```text
qshare_0.1.0_windows_amd64.zip
qshare_0.1.0_darwin_amd64.tar.gz
qshare_0.1.0_darwin_arm64.tar.gz
```

## 13. Package managers

Binary archives remain canonical.

Convenience distribution may later include:

* Nix package / flake;
* Homebrew;
* WinGet or Scoop;
* distribution-specific Linux packages where system integration requires them.

Do not make a package manager the only supported installation path.

## 14. Testing strategy

### Unit

Fast and platform-independent.

### HTTP

Use `httptest`.

### Integration

Real process/server behavior.

### Platform integration

Direct Mode hotspot and network lifecycle tests may require dedicated machines or CI runners.

Tests that mutate host networking must never run unexpectedly as ordinary unit tests.

## 15. Development workflow

For most changes:

```text
Issue/task
   ↓
read AGENTS.md
   ↓
read relevant docs
   ↓
small implementation
   ↓
tests
   ↓
go test ./...
   ↓
go vet ./...
   ↓
documentation update
```
