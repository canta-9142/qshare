# qshare Requirements

Version: 0.2 Draft

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

In Phase 1, a successful download does not end the session. `GET`, `HEAD`, requests
containing a `Range` header, and retries are independent HTTP requests within the
same session. Phase 1 does not model a "logical single download."

The Phase 1 session ends when:

* its lifetime expires;
* qshare receives SIGINT;
* qshare receives SIGTERM; or
* a fatal internal error prevents the session from continuing safely.

On expiration, qshare must stop accepting new requests and allow transfers that
are already in progress to complete. Expiration is a successful termination.

The detailed lifecycle decision is recorded in
[`docs/adr/0004-phase-1-session-lifecycle.md`](adr/0004-phase-1-session-lifecycle.md).

## 6. Receive mode

When started without file arguments:

```sh
qshare
```

qshare will eventually operate in receive mode.

The browser UI must allow the remote device to submit:

* files;
* photos selected through the platform file picker;
* text.

Uploaded files must be stored only within an explicitly configured receive destination.

Text received from the remote device should be capable of being emitted to stdout.

This enables workflows such as:

```sh
qshare | wl-copy
```

## 7. Multiple files

A later version must support:

```sh
qshare file1 file2 file3
```

The browser must be able to:

* inspect the shared file list;
* download individual files;
* download all files where an appropriate archive mechanism is available.

## 8. Directory sharing

A later version must support:

```sh
qshare ./directory
```

Only files within the explicitly shared directory boundary may be exposed.

Directory traversal outside that boundary is forbidden.

## 9. stdin and text sharing

A later version must support:

```sh
cat notes.txt | qshare
```

and:

```sh
qshare --text "hello"
```

stdin behavior must follow the CLI contract in `docs/cli.md`.

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

Direct Mode is the intended default network strategy once implemented.

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
* safe receive directory;
* text submission;
* stdout text output.

### Phase 3: Direct Mode

Add:

* temporary Wi-Fi AP;
* local IP assignment;
* browser access without an existing LAN;
* captive-portal experiments where useful;
* clean network teardown.

### Phase 4 and later

Add progressively:

* multiple files;
* directory sharing;
* archive download;
* stdin;
* explicit text sharing;
* Windows;
* macOS.

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
14. expiration rejects new requests while allowing in-progress transfers to finish;
15. a selected final path component that is a symbolic link is rejected;
16. symbolic links in ancestor directories do not by themselves prevent sharing a regular file.
17. the session lifetime defaults to ten minutes and can be set to a positive duration with `--expire`.
