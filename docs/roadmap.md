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
* [ ] authenticated download endpoint
* [ ] streaming transfer
* [ ] local-address selection
* [ ] terminal QR rendering
* [ ] SIGINT/SIGTERM shutdown
* [ ] expiration drain behavior
* [ ] configurable session lifetime with a ten-minute default
* [ ] session persistence across downloads and retries
* [ ] security tests
* [ ] basic HTTP tests
* [ ] embedded minimal download page
* [ ] Linux amd64 build
* [ ] Linux arm64 build

Acceptance criteria are defined in `docs/requirements.md`.

## v0.2: Receive Mode

Goal:

> Send files or text from a browser-capable phone back to the PC.

Tasks:

* [ ] `qshare` with no positional arguments
* [ ] upload Web UI
* [ ] configurable receive directory
* [ ] safe filename handling
* [ ] collision policy
* [ ] upload size limits
* [ ] received text endpoint
* [ ] stdout text output
* [ ] `qshare | wl-copy` workflow
* [ ] receive-mode security tests

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
* [ ] optional single-use sessions

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
