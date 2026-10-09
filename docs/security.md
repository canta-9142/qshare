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
  The opened handle is checked again, and opening does not wait for a writer
  if a regular file is replaced by a FIFO between validation and open.
- Browser routes use opaque resource IDs, never local paths or filenames.
- Duplicate filenames do not merge authorization.
- ZIP entry names are sanitized and cannot be absolute or contain traversal.
  Each name component replaces `/`, `\`, `:`, and NUL with `_`; empty names,
  `.` and `..` become `_`. Trailing ASCII spaces and periods are removed;
  names that become empty become `_`. Windows device names (including names
  with extensions) receive an `_` prefix. Collisions after sanitization are
  compared using Unicode lowercase and receive ` (n)` suffixes.
  Directory archives apply this to the root and each descendant, with files
  and directories sharing collision tracking within each parent.
  This does not guarantee compatibility with every extraction filesystem's
  character restrictions, Unicode normalization, or path length limits.
- Files and archives are streamed and stop on request cancellation.
- ZIP archives are finalized only after every entry succeeds. Generation or
  finalization failures abort the HTTP transfer so an incomplete archive is not
  reported as a successful download.

## Shared directories

Directory authorization is frozen at startup. Hidden descendants, symlinks,
and non-regular entries are excluded. Each included node stores an opaque ID,
relative hierarchy, and filesystem identity.

The selected root cannot be a symlink, including when its path ends in `/`.
qshare keeps handles to the root and every included file and directory until
session cleanup, preventing deletion and inode reuse from making a replacement
object pass the identity check. These handles pin identity, not file contents;
downloads still reopen and verify the current authorized path.

Before serving a file, qshare reopens it from the authorized root without
following symlinks. Each node, including the root and every intermediate
directory, is checked for its startup-time type and filesystem identity using
the opened handle before opening the next node relative to that handle. Added,
renamed, missing, or replaced entries are not served, even if a replacement
directory contains a hard link to the original file. Individual downloads and
directory archive creation use the same reopening checks.

Checks apply as each node is reopened. They do not form an atomic filesystem
snapshot: a node may be renamed or removed after its handle is verified, and
an already opened download may continue using that same object. In-place file
content changes remain visible; the tree and file contents are not locked
throughout a request or archive transfer. Browser navigation shows the frozen
startup metadata.

Directory limits bound startup work and in-memory metadata: 1,000 regular
files, 2,000 encountered entries, and depth 20. Directory sharing retains at
most 2,001 handles, including the root; reopening resources and running the
server require additional descriptors. If the process cannot open enough
handles during validation, startup fails and closes all acquired handles
without publishing a partial tree or changing the process descriptor limit.

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

The server binds to the selected LAN IPv4 address on a random TCP port from
`50000`–`59999` by default, or the port explicitly selected with `--port`
(`1`–`65535`). On supported systems, qshare adds a temporary firewall rule
limited to the selected interface, source subnet, destination address, and
port. HTTPS, Direct Mode, captive portals, and automatic hotspot cleanup are
not part of the current implementation.

Firewall cancellation must preserve ownership boundaries. Firewalld insertions
allow authentication while startup is active, then receive a five-second grace
period after cancellation to report ownership before cleanup. Preexisting or
concurrently added rules are left untouched. An insertion whose
result cannot be confirmed is reported as a failure and retains its native
expiry as a fallback. NixOS helper commands are canceled on parent EOF, signals,
or expiry, and partial cleanup has an independent five-second limit. Cleanup
failures are reported alongside the original error. If an nftables insertion
does not provide a usable handle, its owned source set is emptied to stop the
rule matching; empty objects may remain until manual cleanup.

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

Report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/canta-9142/qshare/security/advisories/new).
