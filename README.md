# qshare

**Share files with any phone by scanning a QR code. No app, no cloud, no account.**

qshare is a local file-sharing CLI for transferring files between a computer and a smartphone or other browser-capable device.

The receiving device does not need qshare or any other dedicated application. A camera, Wi-Fi, and a web browser are enough.

```sh
qshare photo.jpg
```

qshare starts a temporary local server and displays a QR code. Scan it with your phone and download the file in your browser.

The current LAN implementation listens on TCP port `55544`. If the host uses a
firewall, allow inbound TCP traffic to that port from the trusted local network.
qshare does not yet change firewall rules automatically.

## Goals

qshare is designed around a small set of principles:

* Zero installation on the receiving device
* No account or external service
* No cloud upload
* No Internet connection required
* Direct transfer between devices
* Minimal interaction
* Temporary sharing sessions that leave no persistent service behind

The intended interaction is:

```text
Share
  ↓
Scan
  ↓
Transfer
```

## Status

qshare is currently under development.

Currently implemented:

* Linux
* Local LAN operation
* Single-file PC → phone transfer
* Browser-based phone → PC file upload
* QR-based access
* Temporary authenticated sessions
* Configurable session lifetime
* Configurable receive directory
* Safe upload naming, collision handling, and a 1 GiB request limit

Later versions are planned to support:

* Text sharing and per-submission clipboard integration
* Multiple files
* Directories
* Windows
* macOS

Direct Mode using a temporary Wi-Fi hotspot remains a long-term design goal,
but its implementation is currently deferred.

See [`docs/roadmap.md`](docs/roadmap.md) for details.

## Usage

### Share a file

```sh
qshare photo.jpg
```

### Receive a file from another device

```sh
qshare
```

When started without file arguments, qshare enters receive mode and exposes an
upload page to the connected device. Received files are saved under
`~/Downloads/qshare` by default. Use `--receive-dir DIR` to select another
destination.

### Share text

Planned for v0.3; these commands are not implemented yet:

```sh
printf 'hello\n' | qshare
```

or:

```sh
qshare --text "hello"
```

Version 0.3 is also planned to accept text from the browser. On Linux, each
received submission will be copyable to the system clipboard with a supported
backend while qshare remains running:

```sh
qshare --clipboard auto
```

## Network modes

qshare is designed to support two network modes.

### LAN Mode

Uses an existing local network.

```text
Laptop ── Wi-Fi/LAN ── Smartphone
```

No Internet connection is required.

### Direct Mode

Deferred; no target release is currently assigned.

If development resumes, Direct Mode is intended to create a temporary Wi-Fi
network on the computer when no suitable LAN is available.

```text
Laptop
  ├── Temporary Wi-Fi AP
  ├── Local HTTP server
  └── QR code
           │
         Wi-Fi
           │
      Smartphone
```

The phone still requires no qshare application.

## Security model

qshare is intended for temporary local transfers, not as a persistent file server.

Sharing sessions use cryptographically secure random tokens and expose only explicitly shared resources.

See [`docs/security.md`](docs/security.md) for the development security model and [`SECURITY.md`](SECURITY.md) for vulnerability reporting.

## Installation

qshare does not yet have a stable release. Build the current development version
from source with:

```sh
go build ./cmd/qshare
```

The currently configured CI build targets are:

```text
linux/amd64
linux/arm64
```

Windows, macOS, and package-manager distribution are planned for later versions.

## Development

qshare is written in Go.

```sh
go test ./...
go vet ./...
go build ./cmd/qshare
```

See [`docs/development.md`](docs/development.md).

## Contributing

Contributions are welcome once the basic architecture has stabilized.

Please read [`CONTRIBUTING.md`](CONTRIBUTING.md) before submitting a pull request.

## License

See [`LICENSE`](LICENSE).
