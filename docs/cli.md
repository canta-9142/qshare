# CLI Contract

## 1. Principles

qshare is a Unix-style CLI.

Its interface should compose cleanly with other command-line tools.

The CLI should remain predictable before it becomes clever.

## 2. Send mode

```sh
qshare FILE...
```

shares one or more files. Send mode accepts at most 100 positional file
arguments. The limit keeps the browser list practical and bounds validation,
open-file, and session-metadata costs. Supplying more than 100 files is a usage
error, produces exit status `2`, and is rejected before any file is opened or
a session is created.

An explicit session lifetime may be selected with:

```sh
qshare --expire DURATION FILE...
```

`DURATION` uses Go duration syntax, such as `30s`, `10m`, or `1h30m`, and must
be greater than zero. The default lifetime is `10m`. An invalid duration is a
CLI usage error and produces exit status `2`.

Phase 1 listens on TCP port `55544`. If a host firewall blocks inbound LAN
connections, the user must permit that port separately. qshare does not modify
firewall rules in Phase 1.

Files are displayed in the same order as their positional arguments. Files
with the same base name are accepted and remain separate downloadable items.
Remote URLs identify each file with an opaque resource ID that is independent
of, and does not expose, its local path or filename.

The authenticated browser page also offers an on-demand ZIP containing all
files in CLI order. Duplicate names use `name (1).ext`, `name (2).ext`, and so
on, selecting the first unused name. ZIP output is streamed and is not staged
in a temporary file.

qshare validates every positional file before it creates a session, starts the
server, or prints the QR code and access information. If any file is invalid,
the command fails without sharing the valid subset. Resources opened while
validating the selection are closed on failure.

Version 0.5 also accepts exactly one directory:

```sh
qshare DIRECTORY
```

A directory cannot be combined with positional files or another directory.
The selected root must not itself be a symbolic link. qshare validates and
freezes the complete authorized directory tree before creating the session,
starting the server, or printing access information. Directory mode is limited
to 1,000 regular files, 2,000 encountered entries below the root, and depth 20
with the root at depth 0. A limit or traversal error fails the complete
operation; qshare does not start a partial directory session.

The browser provides hierarchical navigation, individual file downloads, and
an on-demand streamed ZIP named from the shared root. Hidden entries,
descendant symbolic links, and non-regular special files are excluded. Files or
directories added after startup are not shared, and missing, renamed, or
replaced authorized objects are not served.

## 3. Receive mode

```sh
qshare
```

with no positional arguments enters receive mode.

The receive destination may be selected with:

```sh
qshare --receive-dir DIR
```

The default is `~/Downloads/qshare`, with `~` resolved by qshare to the current
user's home directory rather than by the shell.

When an uploaded filename collides with an existing file, qshare does not
overwrite it. It adds ` (n)` before the extension, starting at ` (1)`; for
example, `photo.jpg` becomes `photo (1).jpg`.

Each file-upload request is limited to 1 GiB (1,073,741,824 bytes).

Receiving a file does not end the session. Receive mode continues to accept
files until the session expires or is otherwise terminated.

In v0.3, the browser may submit UTF-8 text in receive mode. Each submission is
limited to 1 MiB (1,048,576 bytes). Concurrent submissions are handled one at a
time in arrival order.

Clipboard integration may be enabled with:

```sh
qshare --clipboard BACKEND
```

`BACKEND` is one of `auto`, `wl-copy`, `xclip`, or `xsel`. `auto` selects the
first executable found in `PATH`, in the order `wl-copy`, `xclip`, then `xsel`.
If `auto` finds none of them, startup fails with exit status `1`. An explicitly
selected backend must also be found in `PATH` when qshare starts or startup
fails with exit status `1`.

qshare uses fixed arguments for backends that need them: `xclip -selection
clipboard` and `xsel --clipboard --input`. `wl-copy` needs no fixed arguments.
Users cannot provide extra backend arguments in v0.3.

For every received text submission, qshare starts the selected backend once and
writes the text to its standard input. qshare invokes the executable directly,
without a shell. A backend failure rejects that submission and is reported to
the browser, but does not end the receive session or prevent later submissions.
Without `--clipboard`, each received value is written unchanged to stdout in
serialized submission order and is not copied to the system clipboard. qshare
does not add separators between submissions; they form one ordinary byte stream
that can be piped into another command. Status and diagnostic output remain on
stderr.

## 4. Text send mode

In v0.3, non-terminal stdin is shared as UTF-8 text:

Example:

```sh
cat notes.txt | qshare
```

Text may instead be supplied explicitly:

```sh
qshare --text "hello"
```

Text input is limited to 1 MiB (1,048,576 bytes). Invalid UTF-8 and input over
the limit are rejected as usage errors with exit status `2`.

`--text` cannot be combined with positional files. Non-terminal stdin cannot be
combined with positional files, `--text`, or `--clipboard`; these combinations
are usage errors with exit status `2`. Terminal stdin with no positional files
continues to select receive mode.

Do not silently invent precedence rules.

## 5. stdout and stderr

General rule:

```text
stdout = useful/composable program output
stderr = interactive UI, QR code, status, progress, diagnostics
```

Examples of stderr output:

* QR rendering;
* server address;
* transfer status;
* progress;
* warnings.

Examples of stdout output:

* text received from the browser when `--clipboard` is not enabled;
* future machine-readable output modes.

This separation preserves shell composition.

## 6. Exit codes

Initial convention:

```text
0    success, including normal session expiration
1    runtime, transfer, or fatal internal error
2    invalid CLI usage
130  terminated by SIGINT
143  terminated by SIGTERM
```

More specific exit codes may be added only when they provide useful scripting semantics.

## 7. Signals

SIGINT and SIGTERM must initiate graceful shutdown.

Graceful shutdown includes:

* stopping the HTTP server;
* closing active resources;
* cleaning temporary networking state where applicable.

After graceful cleanup, qshare exits with status `130` for SIGINT and `143` for
SIGTERM. Normal session expiration exits with status `0`, including when qshare
must close transfers that remain after the 30-second expiration drain period.
A cleanup failure exits with status `1`.

In Phase 1, a completed download does not cause qshare to exit. The session
continues until expiration, a termination signal, or a fatal error. The future
semantics of `--once` are not yet defined.

## 8. Options

Phase 1 supports:

```sh
qshare --lan FILE...
qshare --expire DURATION FILE...
```

`--lan` explicitly requests LAN behavior.

`--expire` sets the Phase 1 session lifetime as described in the send-mode
contract above.

Version 0.3 adds:

```sh
qshare --text TEXT
qshare --clipboard BACKEND
```

`--text` selects text send mode. `--clipboard` selects receive mode and enables
the per-submission clipboard behavior described in section 3.

Future flags may include:

```text
--direct
--once
```

Flags should not be added until their semantics are defined.

In particular, `--once` is only a reserved name at this stage; no behavior should
be inferred from it.

## 9. Diagnostics

Error messages should identify:

* what failed;
* the relevant resource when safe;
* an actionable cause where possible.

Avoid dumping Go internals or stack traces during ordinary failures.

## 10. Examples

Send:

```sh
qshare photo.jpg
```

Send multiple files, preserving the specified display order:

```sh
qshare front/photo.jpg back/photo.jpg notes.txt
```

Share one directory recursively:

```sh
qshare ./photos
```

Receive:

```sh
qshare
```

Share piped text:

```sh
printf 'hello\n' | qshare
```

Share explicit text:

```sh
qshare --text "hello"
```

Receive text and copy each submission to an automatically selected clipboard
backend:

```sh
qshare --clipboard auto
```

Receive text as a composable stdout stream:

```sh
qshare | COMMAND
```
