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

## v0.3: Direct Mode

Goal:

> Operate without an existing LAN.

Tasks:

* [ ] platform networking interface
* [ ] Linux hotspot implementation
* [ ] temporary SSID and credentials
* [ ] IP configuration
* [ ] DHCP strategy
* [ ] DNS strategy if needed
* [ ] captive portal experiments
* [ ] Wi-Fi QR bootstrap
* [ ] fallback browser-access mechanism
* [ ] temporary firewall-rule setup and teardown
* [ ] clean teardown
* [ ] failure recovery after partial hotspot setup
* [ ] Linux integration tests

Do not assume captive portal auto-opening is reliable until tested on real Android and iOS devices.

## v0.4: Rich sharing

Potential features:

* [ ] multiple files
* [ ] directory browsing
* [ ] individual downloads
* [ ] on-demand archive download
* [ ] stdin sharing
* [ ] explicit `--text`
* [ ] received text endpoint
* [ ] per-submission text handling
* [ ] clipboard command integration
* [ ] optional single-use sessions

Clipboard integration should keep the receive session open and update the
clipboard for each text submission. A plain `qshare | wl-copy` pipeline does
not provide those semantics because `wl-copy` consumes one input stream, so the
CLI contract for per-submission command execution must be defined before this
feature is implemented.

## v0.5+: Additional platforms

* [ ] Windows LAN Mode
* [ ] macOS LAN Mode
* [ ] Windows Direct Mode investigation
* [ ] macOS Direct Mode investigation
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
* Bluetooth transport.
