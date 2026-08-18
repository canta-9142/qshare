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

Share UTF-8 text from stdin:

```sh
printf 'hello\n' | qshare
```

Or supply it explicitly:

```sh
qshare --text "hello"
```

In receive mode, text submitted from the browser is written unchanged to
stdout. Submissions can therefore be piped to another command:

```sh
qshare | COMMAND
```

Clipboard integration can instead be enabled with automatic Linux backend
selection:

```sh
qshare --clipboard auto
```

Automatic selection checks `wl-copy`, `xclip`, then `xsel`. A backend can also
be selected explicitly with `--clipboard wl-copy`, `--clipboard xclip`, or
`--clipboard xsel`.

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

Packaged builds for Fedora and Debian are published through the qshare Open
Build Service (OBS) repository. This is a third-party repository and is not
enabled by Fedora or Debian by default.

### Fedora

Fedora 43 and 44 are supported on x86_64 and aarch64. Add the repository that
matches the installed Fedora release, then install qshare:

```sh
fedora_version=$(rpm -E %fedora)
sudo dnf config-manager addrepo \
  --from-repofile="https://download.opensuse.org/repositories/home:/canta-9142/Fedora_${fedora_version}/home:canta-9142.repo"
sudo dnf install qshare
```

### Debian

Debian 13 is currently supported on amd64. Add the OBS signing key and package
repository, then install qshare:

```sh
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL \
  https://download.opensuse.org/repositories/home:/canta-9142/Debian_13/Release.key \
  | sudo tee /etc/apt/keyrings/qshare-obs.asc >/dev/null
echo "deb [signed-by=/etc/apt/keyrings/qshare-obs.asc] https://download.opensuse.org/repositories/home:/canta-9142/Debian_13/ /" \
  | sudo tee /etc/apt/sources.list.d/qshare-obs.list >/dev/null
sudo apt update
sudo apt install qshare
```

Packages in these OBS repositories do not make qshare available from an
otherwise unmodified Fedora or Debian installation. Inclusion in the official
distribution repositories requires each distribution's separate package review
and submission process.

### Nix

On Linux amd64 or arm64, the Nix flake can build or run the current source tree:

```sh
nix build
nix run . -- photo.jpg
```

The current `main` branch can also be run directly without cloning it first:

```sh
nix run github:canta-9142/qshare -- photo.jpg
```

### Build from source

Build the current development version with:

```sh
go build ./cmd/qshare
```

The currently configured CI build targets are:

```text
linux/amd64
linux/arm64
```

Windows, macOS, and additional package-manager distribution are planned for later versions.

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
