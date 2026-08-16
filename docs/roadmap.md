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

## v0.4

### Multiple-file sharing

Goal:

> Share an explicit set of files and download each file individually.

Tasks:

* [x] define a maximum of 100 shared files and reject larger selections before
  opening files or creating a session
* [x] define CLI-order preservation and duplicate-base-name behavior
* [x] define opaque resource IDs independent of paths and filenames
* [x] define all-or-nothing validation before session startup
* [x] accept multiple positional file arguments
* [x] validate the complete file set before starting a session
* [x] represent shared files as an explicit resource collection
* [x] assign opaque resource IDs independent of local paths and filenames
* [x] display the shared file list in the browser
* [x] support individual file downloads
* [x] support files with duplicate base names
* [x] enforce the shared-file count limit
* [x] add multiple-file security and HTTP tests

### Archive download

Goal:

> Download all files in a session as an on-demand archive.

Tasks:

* [x] stream ZIP generation without buffering the archive or creating a
  temporary ZIP file
* [x] define collision handling for names inside the archive
* [x] stop archive generation when the request is cancelled
* [x] add archive security and streaming tests

## v0.5

### Directory sharing

Goal:

> Share the contents of one explicitly selected directory without exposing
> files outside its boundary.

Tasks:

* [x] define directory traversal and browser navigation behavior
* [x] define handling for symlinks, hidden files, and filesystem changes during
  a session
* [x] define file-count, encountered-entry, and directory-depth limits
* [x] enforce file-count, encountered-entry, and directory-depth limits
* [x] validate and freeze the authorized resource tree when the session starts
* [x] browse shared directories in the browser
* [x] download individual files from a shared directory
* [x] download a shared directory as an on-demand archive
* [x] add directory-boundary and symlink security tests

### distribution work

Tasks:

* [ ] publish to Open Build Service
* [x] Arch Linux package recipe
* [ ] Nix flakes

## v1+: Additional platforms and distribution

Tasks:

* [ ] macOS LAN Mode
* [ ] Windows LAN Mode
* [ ] Homebrew distribution
* [ ] Windows package-manager distribution
