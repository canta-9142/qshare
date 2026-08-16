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

The Open Build Service package uses `qshare.spec` for RPM distributions and
`qshare.dsc` together with the flat `debian.*` files for Debian distributions.
Both recipes consume the same source archive and the `vendor.tar.gz` generated
by the `go_modules` source service, so package builds do not need network
access. Debian builds require Go 1.24 or newer.

Convenience distribution may also include:

* Nix package / flake;
* Homebrew;
* WinGet or Scoop;
* distribution-specific Linux packages where system integration requires them.

Do not make a package manager the only supported installation path.

## 14. Testing strategy

### General conventions

Place Go tests beside the implementation in files named `*_test.go`.

Use the standard library's `testing` package by default. Do not add an assertion,
mocking, or fixture library unless it provides a clear project-level benefit
that would be cumbersome to achieve with the standard library.

Tests normally use the same package as the code under test. A `package foo_test`
test is appropriate when the purpose is specifically to exercise only the
exported package contract.

Use table-driven tests when several inputs exercise the same behavior, especially
for parsing, validation, authorization, and security edge cases. A single
scenario does not need to be forced into a table. Give subtests names that
describe the behavior or case being checked.

Test helpers must call `t.Helper()`. Use `t.Cleanup()` for cleanup owned by a
test. Prefer `t.TempDir()` for temporary filesystem state and `testdata/` only
for small, reusable static fixtures.

Tests must be deterministic. Avoid relying on sleeps, the current wall clock,
random timing, external services, or test execution order when the relevant
input can instead be supplied explicitly. Do not run tests in parallel when they
share process-global or host state.

Assert observable behavior rather than implementation details. Compare exact
error text only when that text is part of a public CLI or protocol contract;
otherwise prefer error identity, type, or the relevant state transition.

The project does not currently require a numeric coverage threshold. Coverage
should follow meaningful behavior and security boundaries rather than line
count alone.

### Unit

Unit tests must be fast and platform-independent where practical. Prioritize:

* token generation, encoding, parsing, and comparison;
* session expiration and authorization;
* path and shared-resource validation;
* argument parsing and application request mapping;
* other security-sensitive pure logic.

### HTTP

Use `net/http/httptest` for handlers and HTTP adapters. Assert the relevant
status, headers, and body behavior. Authentication failures should also be
checked for responses that do not reveal whether a protected resource exists.

### Integration

Use integration tests for real process and server behavior such as streaming,
cancellation, binding, and graceful shutdown. Safe localhost-only integration
tests may run as part of `go test ./...` when they are deterministic and do not
depend on Internet access.

### Platform integration

Direct Mode hotspot and network lifecycle tests may require dedicated machines or CI runners.

Tests that mutate host networking must never run unexpectedly as ordinary unit tests.
Tests that require Internet access, dedicated hardware, elevated privileges, or
host-network mutation must be opt-in behind an explicit build tag. Document the
command and environmental requirements next to the test suite.

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
