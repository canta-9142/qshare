# Contributing to qshare

Thank you for contributing to qshare.

qshare aims to remain a small, understandable tool rather than becoming a general-purpose synchronization platform.

Before making a change, read `docs/requirements.md` and the documents relevant
to the change:

* architecture or package boundaries: `docs/architecture.md`;
* HTTP, paths, uploads, tokens, archives, symlinks, or networking:
  `docs/security.md`;
* arguments, output streams, exit codes, signals, or defaults: `docs/cli.md`;
* implementation scope or planned features: `docs/roadmap.md`.

## Development setup

See [`docs/development.md`](docs/development.md).

The basic validation commands are:

```sh
go test ./...
go vet ./...
```

All committed Go code must be formatted with `gofmt`.

CI additionally runs the test suite with the race detector and verifies the Nix
flake. These checks do not change the standard Go development workflow.

## Pull requests

Keep pull requests focused.

A pull request should ideally implement one coherent behavior or refactoring.

Avoid combining:

* unrelated refactoring;
* dependency upgrades;
* formatting changes;
* feature work

unless they are required by the same change.

A pull request that changes externally observable behavior should include corresponding documentation updates.

## Tests

Add tests for behavior that can reasonably be tested automatically.

Tests are particularly important for:

* token generation and validation
* path validation
* session expiration
* HTTP authorization
* upload filename handling
* archive generation
* parsing
* platform-independent networking decisions

Platform-specific behavior may require integration tests.

Detailed conventions for test packages, table-driven tests, helpers, fixtures,
HTTP tests, and opt-in integration tests are documented in
[`docs/development.md`](docs/development.md#testing).

## Dependencies

New dependencies should have a clear reason.

Before introducing one, consider whether the Go standard library is sufficient.

Dependencies that add any of the following require particular justification:

* CGO
* persistent background services
* cloud services
* telemetry
* external authentication
* large runtime frameworks

## Architecture

Respect package boundaries described in `docs/architecture.md`.

## Security issues

Do not report security vulnerabilities in a public issue.

Report them privately through
[GitHub Security Advisories](https://github.com/canta-9142/qshare/security/advisories/new).
