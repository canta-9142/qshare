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

The initial MVP targets:

* Linux
* Local LAN transfer
* Single-file PC → phone transfer
* QR-based access
* Temporary authenticated sessions

Later versions are planned to support:

* Phone → PC transfer
* Direct Mode using a temporary Wi-Fi hotspot
* Multiple files
* Directories
* stdin/stdout
* Text sharing
* Windows
* macOS

See [`docs/roadmap.md`](docs/roadmap.md) for details.

## Usage

### Share a file

```sh
qshare photo.jpg
```

### Receive from another device

Planned:

```sh
qshare
```

When started without file arguments, qshare will enter receive mode and expose an upload page to the connected device.

### Share text

Planned:

```sh
printf 'hello\n' | qshare
```

or:

```sh
qshare --text "hello"
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

Planned.

When no suitable LAN is available, qshare will create a temporary Wi-Fi network on the computer.

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

Binary releases are the canonical distribution format.

Planned release artifacts include:

```text
qshare_<version>_linux_amd64.tar.gz
qshare_<version>_linux_arm64.tar.gz
qshare_<version>_windows_amd64.zip
qshare_<version>_darwin_amd64.tar.gz
qshare_<version>_darwin_arm64.tar.gz
```

Additional package-manager distribution may be provided later.

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
