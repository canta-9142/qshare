# Requirements

Version: 0.6.2

## Purpose

qshare transfers files and UTF-8 text between a Linux PC and a smartphone or
other browser-capable device on the same LAN. The remote device needs only a
camera, Wi-Fi, and a browser; qshare requires no cloud service, account, or
dedicated phone application.

qshare is a temporary transfer tool, not a persistent file server, sync tool,
cloud service, or device-management system.

## Supported environment

- Host: Linux on amd64 or arm64
- Remote device: a modern browser on the same reachable LAN
- Transport: local HTTP on a randomly selected TCP port from `50000`–`59999`
- Internet access: not required

qshare temporarily configures active firewalld installations and standard
NixOS firewalls. Other firewall configurations may still require manual setup.
Direct Mode (a temporary Wi-Fi hotspot) is not implemented. Windows and macOS
host support are future work.

## Session behavior

Every operation creates a temporary session with a cryptographically secure
256-bit token. The QR code and displayed URL contain that token.

- The default lifetime is 10 minutes and `--expire` may change it.
- Downloads and submissions do not end a session.
- Expiration stops new requests and gives active requests up to 30 seconds to
  finish before they are closed.
- With terminal stdin, pressing `q` stops new requests, gives active requests
  up to 30 seconds to finish, drains accepted text submissions, and exits
  successfully. No Enter key is required.
- SIGINT and SIGTERM trigger graceful shutdown.
- Interactive output, including the QR code and URL, goes to stderr.

## Send files

```sh
qshare FILE...
qshare DIRECTORY
```

Send mode accepts either 1–100 regular files or exactly one directory. A
directory cannot be combined with another path. All inputs are validated
before the server starts; an invalid input fails the complete operation.

For multiple files, the browser preserves CLI order and offers individual
downloads and an on-demand ZIP. Duplicate base names remain distinct and are
renamed inside the ZIP with ` (n)` suffixes. Archives are streamed without a
temporary archive file.

Directory sharing freezes an authorized tree at startup and provides browser
navigation, individual downloads, and a streamed ZIP. It applies these limits:

- 1,000 regular files;
- 2,000 encountered entries below the root;
- depth 20, with the root at depth 0.

Hidden descendants, symlinks, and non-regular files are excluded. Objects
added later are not shared; missing, renamed, or replaced authorized objects
are not served. Changes to the contents of the same regular file are visible
when it is downloaded.

## Send text

```sh
qshare --text "hello"
printf 'hello\n' | qshare
```

Text must be valid UTF-8 and no larger than 1 MiB. Text input cannot be combined
with file paths or receive-only options.

## Receive files and text

```sh
qshare
qshare --receive-dir DIR
```

The default destination is `~/Downloads/qshare`, resolved from the current
user's home directory. qshare creates the destination when needed.

- Each uploaded file is limited to 1 GiB.
- Client-supplied paths are rejected; only a single filename is accepted.
- Existing files are never overwritten. Collisions become `name (1).ext`,
  `name (2).ext`, and so on.
- Failed or oversized uploads leave no partial destination file.
- Browser-submitted text must be valid UTF-8 and no larger than 1 MiB.

Receive mode defaults to `--clipboard auto`. It selects `wl-copy`, `xclip`, or
`xsel`, in that order. If none is installed, qshare reports this on stderr and
writes submitted text unchanged to stdout. Text submissions are processed
serially; qshare does not insert separators into the stdout stream.

## Security requirements

Remote input must never select a local filesystem path or act as a credential.
Only resources authorized when the session starts may be served. Tokens must
be unpredictable, compared safely, expire with the session, and never be
included in routine diagnostics.

Detailed invariants and threat boundaries are in [security.md](security.md).
CLI combinations and exit behavior are in [cli.md](cli.md).
