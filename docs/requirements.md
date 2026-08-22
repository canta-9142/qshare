# qshare Requirements

Version: 0.6 Draft

## 1. Overview

qshare is a local file-sharing tool for transferring files and text between a computer and a smartphone or other browser-capable device.

The primary user experience is:

```text
Computer
  ↓
start qshare
  ↓
QR code
  ↓
scan with phone
  ↓
browser
  ↓
transfer
```

The receiving device must not require a dedicated qshare application.

The system must work without cloud storage and without an Internet connection.

The final design must also support situations where no existing LAN is available, provided that both devices support Wi-Fi.

## 2. Goals

qshare prioritizes:

1. zero installation on the receiving device;
2. minimal interaction;
3. direct local transfer;
4. Internet-independent operation;
5. minimal platform assumptions on the receiving device;
6. temporary sharing sessions;
7. simple command-line operation on the computer;
8. single-binary distribution where practical.

A receiving device should require only:

* a camera capable of reading QR codes;
* Wi-Fi;
* a web browser.

## 3. Non-goals

qshare is not intended to become:

* a persistent file server;
* a cloud storage service;
* a file synchronization system;
* an account-based sharing network;
* a device-management platform;
* a replacement for a general-purpose HTTP server.

The MVP does not require:

* Internet relay;
* persistent pairing;
* Bluetooth transfer;
* Wi-Fi Direct;
* a native smartphone application;
* background transfer after the browser is closed;
* end-to-end encrypted Internet transport.

## 4. Target platforms

Initial PC target:

* Linux

Long-term PC targets:

* Linux
* Windows
* macOS

Receiving-device minimum targets:

* Android with a modern browser
* iOS with Safari

Additional browser-capable Wi-Fi devices should work where practical.

## 5. Send mode

The basic send command is:

```sh
qshare FILE
```

The session lifetime may be selected explicitly:

```sh
qshare --expire DURATION FILE
```

`DURATION` uses Go duration syntax and must be greater than zero. Phase 1 uses a
default lifetime of ten minutes.

qshare must:

1. validate the requested file;
2. create an ephemeral session;
3. generate a cryptographically secure 256-bit secret token;
4. start a temporary HTTP server;
5. determine a usable local address;
6. construct an authenticated download URL;
7. display a QR code;
8. allow the browser to download the file;
9. stream the file instead of buffering the complete content in memory;
10. stop cleanly when the session expires, the user interrupts the process, or a fatal error occurs.

Phase 1 listens on TCP port `55544`. Automatic host-firewall configuration is
not part of Phase 1; the user may need to permit inbound TCP traffic to that
port on the trusted LAN.

In Phase 1, a successful download does not end the session. `GET`, `HEAD`, requests
containing a `Range` header, and retries are independent HTTP requests within the
same session. Phase 1 does not model a "logical single download."

The Phase 1 session ends when:

* its lifetime expires;
* qshare receives SIGINT;
* qshare receives SIGTERM; or
* a fatal internal error prevents the session from continuing safely.

On expiration, qshare must stop accepting new requests and allow transfers that
are already in progress up to 30 seconds to complete. After that drain period,
qshare must close any remaining transfers. Expiration is a successful
termination unless cleanup itself fails.

The detailed lifecycle decision is recorded in
[`docs/adr/0004-phase-1-session-lifecycle.md`](adr/0004-phase-1-session-lifecycle.md).

## 6. Receive mode

When started without file arguments:

```sh
qshare
```

qshare operates in receive mode.

The receive destination may be selected explicitly:

```sh
qshare --receive-dir DIR
```

The default receive destination is `~/Downloads/qshare`. qshare resolves `~`
to the current user's home directory; this default does not depend on shell
tilde expansion.

In Phase 2, the browser UI must allow the remote device to submit:

* files;
* photos selected through the platform file picker.

Uploaded files must be stored only within the configured receive destination.

If an uploaded filename already exists, qshare must preserve the existing file
and select a new name by adding ` (n)` before the filename extension, beginning
with ` (1)`. For example, a collision on `photo.jpg` produces `photo (1).jpg`,
then `photo (2).jpg`. Selecting the name and creating the destination file must
not permit concurrent uploads to overwrite one another.

Each file-upload request is limited to 1 GiB (1,073,741,824 bytes). Requests
that exceed the limit must be rejected without leaving a partially received
file in the destination.

A successful file submission does not end the receive session. qshare continues
accepting files until the configured session lifetime expires or another normal
session termination condition occurs.

## 7. Multiple files

Version 0.4 must support:

```sh
qshare file1 file2 file3
```

Send mode accepts from 1 through 100 file arguments. The limit of 100 keeps
the browser list practical while placing a clear bound on validation work,
open file descriptors, and per-session metadata. Supplying more than 100 files
is a CLI usage error and must be rejected before any file is opened or a
session is created.

Every selected file must be validated before qshare creates the session,
starts the HTTP server, or displays access information. If any file is invalid,
the entire operation must fail; qshare must not start a session containing a
partial subset.

The shared-file list must preserve positional-argument order. Files with the
same base name are distinct shared resources and must remain individually
downloadable. Each resource must receive an opaque ID that is independent of
its local path and filename. Browser-visible URLs and other remote input must
identify files by that ID and must not disclose or accept a local filesystem
path.

The browser must be able to:

* inspect the shared file list;
* download individual files;
* download all files where an appropriate archive mechanism is available.

Download-all is provided as an on-demand ZIP stream. qshare must not create a
temporary archive or buffer the complete archive in memory. ZIP entries retain
CLI order. If an entry name is already in use, qshare inserts ` (n)` before its
extension, beginning with ` (1)` and increasing until the name is unique.
Archive generation must stop when the HTTP request is cancelled.

## 8. Directory sharing

Version 0.5 must support:

```sh
qshare ./directory
```

Directory mode accepts exactly one positional directory. It must not be combined
with positional files or another directory. The selected directory itself must
not be a symbolic link; symbolic links in its ancestor path are permitted.

At session startup, qshare must walk the selected directory and freeze an
authorized resource tree. Only regular files and directories in that tree may
be exposed. Each authorized node must receive an opaque ID independent of its
local path and name. HTTP input must resolve those IDs and must never be treated
as a filesystem path.

Directory traversal uses these rules:

* symbolic links below the shared root are excluded and never dereferenced;
* entries whose names begin with `.` are excluded, including their descendants;
* an explicitly selected root whose own name begins with `.` is allowed;
* sockets, FIFOs, devices, and other non-regular special files are excluded;
* excluded entries do not cause startup to fail;
* a directory-read, metadata, or permission error aborts the entire operation;
  qshare must not start a partial session.

The walk is limited to 1,000 regular files, 2,000 encountered entries, and a
maximum depth of 20. The root is depth 0 and is not counted as an encountered
entry. Every child observed while walking counts toward the entry limit even
when it is excluded; excluded directories are not descended into. Exceeding a
limit aborts startup before the server or session is created. These limits
bound startup work and session metadata without limiting the size of an
explicitly authorized regular file.

The browser must preserve the directory hierarchy. Within each directory it
lists directories first and then files, with each group sorted by name. It must
support breadcrumb navigation, parent navigation, and empty directories. Local
absolute paths and excluded entries must not be displayed.

The authorized names, hierarchy, and filesystem identities are fixed when the
session starts. Files and directories added later are not exposed. Before an
individual or archive download uses an entry, qshare must safely reopen it from
the shared root and verify that it is the same filesystem object authorized at
startup. A missing, renamed, or replaced object must not be served. Changes to
the contents of the same regular-file object are allowed and the download
streams its contents at request time. qshare does not snapshot file contents.

Authorized regular files support individual `GET` and `HEAD` downloads,
including retries and Range requests. Unknown IDs, IDs from another session,
expired sessions, and entries that fail identity verification must not disclose
resource metadata or contents.

The browser must also offer the shared directory as an on-demand ZIP stream.
The archive filename is the shared root's base name with `.zip` appended. The
archive contains one top-level directory named after that base name, preserves
the authorized relative hierarchy and deterministic browser order, and includes
empty directories. Excluded entries are not included. ZIP entry names must not
be absolute, contain traversal components, or disclose local paths. qshare must
not create a temporary archive or buffer the complete archive in memory, and it
must stop generation when the request is cancelled. If an authorized object is
missing, renamed, replaced, or otherwise unreadable during generation, qshare
must stop that archive response rather than substitute another object.

Directory traversal outside the explicitly selected boundary is forbidden.

## 9. stdin and text sharing

Version 0.3 must support:

```sh
cat notes.txt | qshare
```

and:

```sh
qshare --text "hello"
```

Text supplied through stdin or `--text` must be valid UTF-8 and no larger than
1 MiB (1,048,576 bytes). Invalid UTF-8 and oversized input must be rejected.
stdin behavior and incompatible option combinations must follow the CLI
contract in `docs/cli.md`.

Version 0.3 must also accept UTF-8 text submitted from the remote browser. Each
submission is limited to 1 MiB. Concurrent submissions must be processed one at
a time in arrival order, and the receive session must remain available for
later submissions.

Receive mode enables clipboard integration with the equivalent of
`--clipboard auto` when `--clipboard` is omitted. `BACKEND` may be `auto` or a
supported backend name. Version 0.3 supports the Linux backends `wl-copy`,
`xclip`, and `xsel`. Each received submission starts the selected executable
once and supplies the text through standard input. The executable must be
invoked directly without a shell. A backend failure fails only that submission
and must not terminate the session.

`auto` searches `PATH` in the order `wl-copy`, `xclip`, then `xsel`. If none of
these executables is found, qshare must notify the user on stderr, continue the
receive session, and write submitted text to stdout. Backend arguments are fixed
by qshare: no arguments for `wl-copy`, `-selection clipboard` for `xclip`, and
`--clipboard --input` for `xsel`. Version 0.3 does not accept user-defined
executables or arguments.

When automatic selection cannot find a clipboard backend, qshare writes the
exact bytes of each received value to stdout in serialized submission order. It
does not add separators or otherwise modify a submitted value. Consecutive
submissions therefore form one ordinary stdout byte stream, allowing the user
to pipe received text into an arbitrary local command. Status, QR, notices, and
diagnostic output remain on stderr.

Clipboard integration must remain behind an adapter; core text/session packages
must not depend on a particular desktop clipboard implementation. When no
supported backend is available, a pipeline such as `qshare | COMMAND` receives
the serialized stdout stream. A selected clipboard backend is invoked once per
submission and can report a failure for that specific submission.

## 10. Network modes

### 10.1 LAN Mode

LAN Mode uses an existing local network.

Explicit selection:

```sh
qshare --lan FILE
```

The PC and receiving device must be able to communicate over the local network.

Internet access is not required.

The initial MVP implements LAN Mode only.

### 10.2 Direct Mode

Direct Mode is part of the long-term design, but its implementation is deferred
and is not assigned to a release.

When no suitable existing LAN is available, qshare must be capable of creating a temporary Wi-Fi network on the PC, provided the PC has compatible Wi-Fi hardware.

Direct Mode may provide:

* temporary SoftAP;
* IP address assignment;
* DHCP;
* DNS;
* captive portal support;
* local HTTP service.

Conceptually:

```text
PC
 ├── Temporary Wi-Fi AP
 ├── Local network services
 └── qshare HTTP server
           │
         Wi-Fi
           │
      Smartphone
```

Internet connectivity is not required.

If the user explicitly requests LAN Mode and no usable LAN exists, the final network-mode policy may permit fallback to Direct Mode. Exact fallback semantics must be documented before Direct Mode is released.

## 11. QR behavior

QR codes are the primary connection bootstrap mechanism.

In LAN Mode, the QR code primarily carries the authenticated local URL.

In Direct Mode, the QR mechanism must provide or assist with:

* Wi-Fi connection information;
* session identification;
* authentication information;
* access to the Web UI.

The implementation should minimize the number of user operations required after scanning.

## 12. Captive portal behavior

Direct Mode may use captive-portal mechanisms to guide the phone from Wi-Fi association to the qshare Web UI.

Desired flow:

```text
QR scan
   ↓
Wi-Fi join
   ↓
portal detection
   ↓
qshare Web UI
   ↓
transfer
```

Because captive portal behavior varies by OS, qshare does not guarantee that the Web UI will always open automatically after one QR scan.

The design target is:

* one QR scan;
* Wi-Fi association;
* no more than approximately one additional user action to reach the transfer UI.

## 13. Web UI

The Web UI must remain small and usable on modern mobile browsers.

### Download UI

At minimum show:

* filename;
* file size;
* download action.

For multiple files, show a file list.

### Upload UI

At minimum provide:

* file selection;
* upload action;
* progress indication where practical;
* completion indication.

Browser camera capture may be exposed through standard HTML capabilities where supported.

## 14. Session model

Sharing is session-based.

A session contains at least:

* session identifier;
* 256-bit secret token;
* permitted resources;
* creation time;
* expiration time;
* transfer state.

Sessions have a short default lifetime of ten minutes in Phase 1. The lifetime
is configurable with `--expire DURATION`, where `DURATION` uses Go duration
syntax and must be greater than zero.

Phase 1 keeps a session available after a successful download. It does not infer
session completion from HTTP request count or download completion. The future
meaning of `--once` remains intentionally unspecified.

## 15. Security requirements

The implementation must:

* generate tokens using a cryptographically secure source;
* prevent token prediction;
* prevent directory traversal;
* prevent access outside explicitly shared resources;
* validate uploaded filenames;
* define symlink behavior explicitly;
* avoid exposing arbitrary local filesystem paths;
* avoid trusting browser-supplied MIME metadata for security decisions;
* shut down temporary services when sharing ends.

Direct Mode should use a temporary WPA2 or WPA3 network where platform support permits.

HTTP is acceptable for the initial local-only implementation because deploying a trusted ad-hoc HTTPS certificate to an unprepared receiving device would substantially degrade the intended zero-install experience.

The full trust model is defined in `docs/security.md`.

For Phase 1, the selected path must resolve to a regular file, and the final path
component itself must not be a symbolic link. Symbolic links in ancestor
directories are permitted.

The detailed file-selection decision is recorded in
[`docs/adr/0005-phase-1-file-selection.md`](adr/0005-phase-1-file-selection.md).

## 16. Performance requirements

File contents must be streamed.

Memory usage must not grow proportionally with transferred file size.

The qshare transfer implementation should not become the primary throughput bottleneck under normal Wi-Fi conditions.

## 17. Service dependencies

Normal operation must not require:

* qshare-operated servers;
* cloud storage;
* third-party authentication;
* an Internet connection.

## 18. Distribution

qshare is implemented in Go.

The canonical release artifact should be a single executable wherever practical.

Web assets should be embedded into the executable.

Pure Go implementations are preferred.

CGO should not be introduced unless required for a justified platform integration.

## 19. Daemon policy

Normal qshare operation must not require a persistent daemon.

Required services should be created only for the duration of a sharing session.

## 20. MVP phases

### Phase 1: Linux LAN send

Implement:

```sh
qshare FILE
```

Requirements:

* Linux
* one regular file
* LAN Mode
* ephemeral HTTP server
* 256-bit token
* QR URL
* streaming download
* graceful shutdown
* session persistence after successful downloads
* exit status `0` on expiration, `130` on SIGINT, `143` on SIGTERM, and `1` on a fatal internal error

### Phase 2: Receive mode

Implement:

```sh
qshare
```

Add:

* browser upload;
* safe receive directory.

### Phase 3: Text sharing

Add:

* stdin text sharing;
* explicit `--text` sharing;
* browser text submission;
* serialized per-submission handling;
* automatic per-submission Linux clipboard integration, with backend selection
  configurable through `--clipboard BACKEND`.

### Phase 4 and later

Add progressively:

* multiple files;
* directory sharing;
* archive download;
* Windows;
* macOS.

Direct Mode is deferred without a target phase. Its existing architectural and
security requirements remain applicable if implementation resumes.

## 21. Phase 1 acceptance criteria

Phase 1 is complete when:

1. a Linux PC can share one regular file;
2. qshare displays a QR code in the terminal;
3. Android or iOS can scan the QR code using standard device functionality;
4. the file can be downloaded without installing a qshare application;
5. Internet access is unnecessary;
6. the file is never uploaded to an external server;
7. the session uses a cryptographically secure 256-bit token;
8. resources outside the selected file cannot be retrieved;
9. large files are streamed;
10. SIGINT causes clean shutdown;
11. automated tests cover token and access-control logic;
12. `go test ./...` and `go vet ./...` pass.
13. a successful download does not end the session;
14. expiration rejects new requests, allows in-progress transfers up to 30 seconds
    to finish, and then closes any transfers that remain;
15. a selected final path component that is a symbolic link is rejected;
16. symbolic links in ancestor directories do not by themselves prevent sharing a regular file.
17. the session lifetime defaults to ten minutes and can be set to a positive duration with `--expire`.
