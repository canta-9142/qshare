<h1 align="center">Qshare - Internet-free file sharing</h1>

[![GitHub release](https://img.shields.io/github/v/release/canta-9142/qshare?logo=github)](https://github.com/canta-9142/qshare/releases)
[![Go](https://img.shields.io/badge/Go-1.26-blue.svg?logo=go)](https://golang.org)
[![Fedora Copr](https://img.shields.io/badge/fedora-copr-blue.svg?logo=fedora)](https://copr.fedorainfracloud.org/coprs/canta-9142/qshare)
[![Nix Flake](https://img.shields.io/badge/nix-flake-5277C3?logo=nixos&logoColor=white)](flake.nix)
[![CI](https://github.com/canta-9142/qshare/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/canta-9142/qshare/actions/workflows/ci.yml)
[![Contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![GitHub stars](https://img.shields.io/github/stars/canta-9142/qshare.svg?style=social&label=Stars)](https://github.com/canta-9142/qshare/stargazers)

[ English | [日本語](README-ja.md) ]

Qshare is a file-sharing tool that uses QR codes to transfer files between a PC and a smartphone.

It has only two requirements:

> - The PC and smartphone must be connected to the same LAN (tethering is also supported).
> - Qshare must be running on the PC.

No smartphone app is required; everything works through the browser.

## Usage

Run the appropriate command, then scan the displayed QR code with your smartphone.

See the [CLI documentation](docs/cli.md) for details about each option.

### Transfer files from a PC to a smartphone

```sh
qshare FILE...
```

`FILE` is the path of a file to transfer.

You can specify multiple files or a single directory. All files can also be downloaded together as a ZIP archive.

### Transfer text from a PC to a smartphone

To provide text as an option:

```sh
qshare --text "Hello, World!"
```

To provide text through a pipe:

```sh
printf "Hello, World!" | qshare
```

As of v0.6, text and files cannot be shared at the same time. Support is planned for v0.7.

### Transfer files or text from a smartphone to a PC

Normally, start Qshare without any options:

```sh
qshare
```

The `--clipboard auto` option is selected automatically. Text submitted from the smartphone is copied to the PC clipboard when a supported clipboard application is available.

## Installation

### Fedora COPR

Qshare is available as a package from Fedora COPR.

```sh
sudo dnf copr enable canta-9142/qshare
sudo dnf install qshare
```

See the [Copr repository](https://copr.fedorainfracloud.org/coprs/canta-9142/qshare/) for supported Fedora versions.

### Arch Linux

Qshare cannot currently be installed from the AUR. (As of August 2026.)

An AUR package will be published when new account registration becomes available again.

For now, follow the instructions for [other Linux distributions](#other-linux-distributions).

### NixOS

Qshare supports Nix Flakes.

Run the following command, or add the repository to the `inputs` in your `flake.nix`:

```sh
nix run github:canta-9142/qshare
```

### Other Linux distributions

Executable binaries are available from the latest release on the [GitHub Releases page](https://github.com/canta-9142/qshare/releases).

Packages for Debian, Raspbian, and Arch are available from the [Open Build Service](https://build.opensuse.org/package/show/home:canta-9142/qshare).

You can also build Qshare from source:

```sh
go build ./cmd/qshare
```

A Linux installation script is planned for the near future.

### Other operating systems (Windows and macOS)

Packages for Windows and macOS are not currently available.

Support is planned, but no release date has been set.

## Development and contributing

Qshare is an open-source project written in Go.

Contributions are welcome.

See [development.md](docs/development.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## License

See [LICENSE](LICENSE).

## Author

- [GitHub](https://github.com/canta-9142)
- [Blog](https://floating-gate.com)

---

QR Code is a registered trademark of DENSO WAVE INCORPORATED.
