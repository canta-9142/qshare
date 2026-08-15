# Roadmap

This roadmap describes intended sequencing, not a promise of release dates.

## v0.1: LAN Send MVP

Goal:

> Share one file from a Linux PC to a phone on the same LAN.

Tasks:

* [x] establish Go module
* [x] thin CLI entrypoint
* [x] regular-file validation
* [x] reject a symlink as the selected final path component
* [x] ephemeral session type
* [x] 256-bit secure token generation
* [x] temporary HTTP server
* [x] authenticated download endpoint
* [x] streaming transfer
* [x] local-address selection
* [x] terminal QR rendering
* [x] SIGINT/SIGTERM shutdown
* [x] expiration drain behavior
* [x] configurable session lifetime with a ten-minute default
* [x] session persistence across downloads and retries
* [x] security tests
* [x] basic HTTP tests
* [x] embedded minimal download page
* [x] Linux amd64 build
* [x] Linux arm64 build

Terminal QR scanning has been verified on Android. Verification on iOS is still pending.

Acceptance criteria are defined in `docs/requirements.md`.

## v0.2: Receive Mode

Goal:

> Send files from a browser-capable phone back to the PC.

Tasks:

* [x] `qshare` with no positional arguments
* [x] upload Web UI
* [x] configurable receive directory
* [x] safe filename handling
* [x] collision policy
* [x] upload size limits
* [x] receive-mode security tests (completion criteria are defined in
  `docs/security.md`)

## v0.3: Text sharing

Goal:

> Exchange text between the PC and a browser-capable device.

Tasks:

* [x] stdin text sharing
* [x] explicit `--text TEXT`
* [x] browser text submission
* [x] 1 MiB text limits
* [x] serialized per-submission handling
* [x] `--clipboard BACKEND`
* [x] Linux clipboard backends (`wl-copy`, `xclip`, and `xsel`)
* [x] automatic clipboard-backend selection
* [x] text-sharing security tests

## v0.4: Rich sharing

Potential features:

* [ ] multiple files
* [ ] directory browsing
* [ ] individual downloads
* [ ] on-demand archive download
* [ ] optional single-use sessions

## v0.5+: Additional platforms

* [ ] Windows LAN Mode
* [ ] macOS LAN Mode
* [ ] Homebrew distribution
* [ ] Windows package-manager distribution

## Explicitly deferred

Unless requirements change:

* cloud relay;
* user accounts;
* persistent pairing;
* device history;
* native mobile applications;
* persistent qshare daemon;
* file synchronization;
* Bluetooth transport;
* Direct Mode, including temporary hotspot support and platform investigation.

Direct Mode remains part of the long-term design, but no release is currently
assigned to its implementation.
