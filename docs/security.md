# Security Model

## Scope

qshare serves untrusted browsers on a local network. A LAN must not be treated
as trusted merely because it is local. The current transport is HTTP, so qshare
protects authorization and filesystem boundaries but does not provide
confidentiality against a network observer.

Users should share only on a trusted LAN and stop the session when finished.

## Session authorization

Each session uses a 256-bit token generated with `crypto/rand`. The token is
embedded in the QR URL and authorizes that session until it expires.

Required invariants:

- tokens are unpredictable and compared in constant time;
- malformed, wrong, cross-session, and expired tokens are rejected;
- ordinary error responses do not reveal whether a protected resource exists;
- tokens are not written to routine logs or persisted by qshare;
- a successful request does not extend the lifetime or end the session.

The URL is visible in terminal output and may remain in browser history. Anyone
who obtains it during the session can use it, so it must be treated as a
temporary bearer credential.

## Shared files

Only paths explicitly selected by the CLI may enter a send session. Validation
finishes before the server starts.

- The selected final file component must be a regular file, not a symlink.
- Browser routes use opaque resource IDs, never local paths or filenames.
- Duplicate filenames do not merge authorization.
- ZIP entry names are sanitized and cannot be absolute or contain traversal.
- Files and archives are streamed and stop on request cancellation.

## Shared directories

Directory authorization is frozen at startup. Hidden descendants, symlinks,
and non-regular entries are excluded. Each included node stores an opaque ID,
relative hierarchy, and filesystem identity.

Before serving a file, qshare reopens it from the authorized root without
following symlinks and verifies that it is the same filesystem object. Added,
renamed, missing, or replaced entries are not served. The same checks apply
while creating a directory archive.

Directory limits bound startup work and in-memory metadata: 1,000 regular
files, 2,000 encountered entries, and depth 20.

## Uploads

The receive directory is chosen locally and converted to an absolute path.
Remote input supplies only a filename.

- Empty names, `.`/`..`, NUL, `/`, and `\` are rejected.
- Uploads are written to a temporary file within the destination.
- The per-file limit is 1 GiB.
- Publication is atomic and never overwrites an existing file.
- Concurrent collisions select distinct ` (n)` names.
- Failed, cancelled, and oversized uploads remove temporary data.

The HTTP server also bounds headers and multipart request size. It uses read
header and idle timeouts to limit stalled connections.

## Text and clipboard handling

Sent and received text must be valid UTF-8 and no larger than 1 MiB. Templates
escape text before displaying it in HTML.

Clipboard backends are a fixed allowlist: `wl-copy`, `xclip`, and `xsel`.
qshare looks them up in `PATH`, invokes them directly without a shell, supplies
fixed arguments, and writes text through stdin. A backend failure rejects only
that submission and does not terminate the receive session.

When no automatic backend is available, submitted bytes are written unchanged
to stdout. Users who pipe that stream into another program are responsible for
the behavior of that program; qshare does not execute received text.

## HTTP behavior

- Protected routes require the session token.
- Unsupported methods are rejected by the route set.
- Browser-visible values are escaped and responses use appropriate content
  types and download headers.
- File responses support `GET`, `HEAD`, retries, and ranges without broadening
  authorization.
- Error responses should disclose as little resource metadata as practical.

## Network boundary

The server binds to the selected LAN IPv4 address on TCP port `55544`. qshare
does not configure the host firewall. HTTPS, Direct Mode, captive portals, and
automatic hotspot cleanup are not part of the current implementation.

## Verification

Security-sensitive pure logic requires unit tests. HTTP tests should cover
invalid and expired tokens, traversal and filename rejection, upload limits and
cleanup, HTML escaping, method handling, resource replacement, archive
cancellation, text limits, concurrent submissions, and clipboard failures.

Run the complete suite and static analysis before merging:

```sh
go test ./...
go vet ./...
```

Report vulnerabilities according to [`SECURITY.md`](../SECURITY.md).
