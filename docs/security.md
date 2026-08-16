# Security Design

## 1. Scope

qshare exposes local resources to another network participant.

Even though the intended use is temporary and local, every remote request must be treated as hostile.

## 2. Threat model

Relevant attackers include:

* another device on the same LAN;
* another participant that joins the temporary Direct Mode network;
* a browser sending malformed requests;
* an attacker that discovers the server port but not the session token;
* a malicious uploaded filename;
* malicious or oversized submitted text;
* a malicious symlink or path structure.

The initial model does not attempt to protect against a fully compromised host operating system.

## 3. Trust boundaries

```text
Local CLI input
      │
      ▼
validated shared resources
      │
──── trust boundary ────
      │
remote HTTP client
```

Everything received from HTTP is untrusted.

## 4. Session tokens

Each sharing session must use a fresh cryptographically secure 256-bit random secret.

Use Go's cryptographic random source.

Tokens must not contain:

* timestamps as their primary entropy;
* counters;
* filenames;
* predictable PRNG output.

Token comparison should avoid transformations that reduce entropy.

## 5. Authorization

Possession of the session secret authorizes access only to resources belonging to that session.

A valid token for one session must not authorize another session.

Do not expose resource enumeration before authentication if avoidable.

## 6. Filesystem boundary

The HTTP client must never provide an arbitrary filesystem path.

Shared filesystem objects must be resolved and validated before serving begins.

The server should operate on an explicit resource map.

Conceptually:

```text
resource ID → validated local resource
```

Client requests resolve resource IDs, not filesystem paths.

For multiple-file send sessions, each resource ID must be opaque and generated
independently of the resource's local path and filename. Duplicate base names
must not cause resources to share an ID or overwrite one another in the
resource map.

ZIP entry names are derived only from validated resource base names, never
from a client-controlled path. They must not contain directory components.
Duplicate entry names are made unique without dropping or overwriting files.

Send mode accepts at most 100 files. The count must be checked before opening
any selected file. qshare must then validate the complete ordered selection
before creating a session, starting the HTTP server, or displaying access
information. If any selected file fails validation, no partial session may be
made available and every resource acquired during validation must be closed.

## 7. Directory traversal

Requests such as:

```text
../../etc/passwd
```

must never escape the session resource boundary.

URL decoding, path normalization, and platform path syntax must not create alternate traversal routes.

Tests should include encoded traversal forms.

## 8. Symlinks

Symlink behavior must be explicit.

Phase 1 shares regular files only.

The final path component supplied by the user must not be a symbolic link. If it
is a symbolic link, qshare must reject it even when its target is a regular file.

Symbolic links in ancestor directories are permitted. After resolving the
selected path through those directories, the selected object must still be
validated as a regular file before the session begins.

This rule deliberately concerns the selected final path component. Phase 1 does
not attempt to prohibit every symbolic link traversed while resolving ancestor
directories.

For directory sharing, the explicitly selected root itself must not be a
symbolic link. Symbolic links encountered below the root are excluded and are
never dereferenced or represented as downloadable entries. Symbolic links in
ancestors of the explicitly selected root remain permitted.

The authorized directory tree is frozen at session startup. Remote requests
resolve only opaque IDs from that tree, never client-supplied filesystem paths.
Before serving an authorized entry, qshare must reopen it safely from the shared
root without following symbolic links and verify that its filesystem identity
matches the object authorized at startup. Missing, renamed, and replaced
objects must not be served. A same-object content change is permitted because
directory sharing freezes authorization and identity, not file bytes.

The Phase 1 decision is recorded in
[`docs/adr/0005-phase-1-file-selection.md`](adr/0005-phase-1-file-selection.md).

## 9. Uploads

Receive mode must:

* use a configured receive directory, defaulting to `~/Downloads/qshare`;
* reject path separators in remote filenames;
* prevent `..` traversal;
* preserve existing files and resolve collisions by adding ` (n)` before the
  filename extension;
* select collision-free names and create files atomically so concurrent uploads
  cannot overwrite one another;
* avoid silently overwriting existing files;
* limit each file-upload request to 1 GiB (1,073,741,824 bytes);
* remove partial files when an upload fails, is interrupted, or exceeds the
  request limit.

A remote filename is display metadata, not a trusted path.

## 10. HTTP

The MVP may use HTTP on the local network.

The security justification is:

* no external relay;
* temporary service;
* unguessable session token;
* short session lifetime;
* zero-install receiving-device requirement.

This does not provide confidentiality against an attacker capable of sniffing local traffic.

That limitation must remain documented.

## 11. HTTPS

Self-signed certificates normally produce browser warnings or require manual trust installation, which conflicts with the receiving-device zero-install goal.

HTTPS may be reconsidered if a mechanism can provide:

* trusted browser TLS;
* no prior client configuration;
* no external dependency;
* acceptable UX.

Until then, pretending that a self-signed certificate meaningfully improves the default experience is not useful.

## 12. Direct Mode

Direct Mode should prefer WPA2 or WPA3 where supported.

SSID and credentials should be temporary.

Network teardown is part of the security boundary.

After the session ends, qshare must not unintentionally leave behind:

* an active hotspot;
* permissive firewall rules;
* DHCP services;
* DNS interception;
* stale credentials.

## 13. Resource exhaustion

The server should apply reasonable limits to attacker-controlled data.

Potential limits include:

* upload size;
* number of files;
* header size;
* request duration;
* concurrent uploads.

Downloads of explicitly shared large files must remain supported.

A send session is limited to 100 explicitly selected files. This keeps browser
output usable and bounds validation work, open file descriptors, and
per-session resource metadata without imposing a size limit on an explicitly
shared file. A request above the limit must be rejected before files are opened
or session state is allocated.

A directory-sharing walk is limited to 1,000 regular files, 2,000 encountered
entries below the root, and depth 20 with the root at depth 0. Encountered
entries count even when excluded, while excluded directories are not traversed.
This bounds filesystem work and authorized-tree metadata. Directory sharing
does not impose a per-file or aggregate byte-size limit because authorized file
contents remain streamed.

## 14. Browser content

Filenames and user-provided text displayed in HTML must be escaped.

Never concatenate untrusted strings directly into HTML markup.

Browser-submitted text must be valid UTF-8 and limited to 1 MiB (1,048,576
bytes) per submission. The limit must be enforced before unbounded buffering or
clipboard-process execution. Concurrent submissions must be serialized with a
bounded queue or equivalent backpressure.

Clipboard backend names are trusted local CLI configuration, not values chosen
by the remote client. qshare must invoke only supported backends, pass submitted
text through standard input, and never interpolate text or backend selection
into a shell command. Environment-based executable lookup must not permit the
remote client to influence the executable path or arguments.

## 15. Security tests

At minimum, send mode must have automated tests for:

* invalid token rejection;
* token separation between sessions;
* unknown resource rejection;
* traversal attempts;
* encoded traversal attempts;
* expiration.

Multiple-file send-mode tests must additionally cover the exact 100-file
boundary, rejection above it before files are opened, preservation of CLI
order, duplicate base names as distinct resources, opaque IDs that do not
expose paths or filenames, and all-or-nothing validation with cleanup after an
intermediate failure.

Before directory sharing is considered complete, automated tests must cover:

* rejection of a symlink as the selected root and non-traversal of descendant
  symlinks, including links whose targets remain inside the root;
* exclusion of hidden and special entries without descending into them;
* all-or-nothing startup on walk, metadata, and permission errors;
* exact file-count, encountered-entry, and depth boundaries;
* opaque node IDs, cross-session IDs, invalid and expired tokens, and raw and
  encoded traversal attempts;
* additions after startup remaining unavailable and missing, renamed, or
  replaced objects not being served;
* HTML escaping and non-disclosure of absolute local paths;
* safe individual GET, HEAD, Range, retry, and concurrent downloads;
* ZIP entry traversal prevention, hierarchy and empty-directory preservation,
  incremental streaming, filesystem changes, and request cancellation.

Before receive-mode security testing is considered complete, automated tests
must cover:

* invalid, cross-session, and expired-token rejection without invoking storage;
* malicious upload filenames, including POSIX and Windows path separators,
  traversal components, empty names, and NUL bytes;
* request and file-size limits at their boundaries, including multipart
  overhead;
* malformed multipart bodies, missing file parts, and invalid
  `Content-Disposition` values;
* interrupted and cancelled uploads, with no partial file left behind;
* collision handling that preserves existing files, including concurrent
  uploads of the same filename;
* rejection of unsupported methods, routes, and encoded traversal paths.

Before text-sharing security testing is considered complete, automated tests
must cover invalid UTF-8, exact and oversized limits, HTML escaping, unsupported
methods, invalid or expired tokens, concurrent submission ordering, and
clipboard-backend failures without shell interpretation or session termination.

The roadmap item for receive-mode security tests may be marked complete when
these cases are represented by automated tests and the full test suite and
static analysis pass.
