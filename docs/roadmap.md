# Roadmap

This file records implementation progress, not promised release dates. Detailed
current behavior belongs in [requirements.md](requirements.md) and
[cli.md](cli.md).

## Completed

### v0.1: LAN file sending

- Authenticated temporary sessions and QR access
- Streaming file downloads on Linux amd64 and arm64
- Ten-minute default lifetime, signal handling, and expiration draining

### v0.2: Receive mode

- Browser file uploads
- Configurable destination, collision-safe naming, and 1 GiB limit

### v0.3: Text sharing

- stdin and `--text` sending
- Browser submissions and 1 MiB limits
- Linux clipboard backends with stdout fallback

### v0.4: Multiple files and archives

- Up to 100 ordered files with opaque resource IDs
- Individual downloads and streamed ZIP archives

### v0.5: Directory sharing and distribution

- Authorized directory browsing and streamed archives
- Symlink, replacement, and traversal protections
- Open Build Service recipes, Arch package recipe, and Nix flake

### v0.6: Maintenance and UI

- Removed unnecessary Linux dependencies
- Improved browser UI
- Reorganized project documentation
- Published Fedora COPR package

## Planned

### Distribution

- Ubuntu Launchpad PPA
- Nixpkgs
- AUR package when new package publication is available

### Additional platforms

- macOS LAN Mode
- Windows LAN Mode
- Homebrew and Windows package-manager distribution

Direct Mode through a temporary Wi-Fi hotspot remains a long-term design goal
with no assigned release.
