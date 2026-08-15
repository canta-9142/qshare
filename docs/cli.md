# CLI Contract

## 1. Principles

qshare is a Unix-style CLI.

Its interface should compose cleanly with other command-line tools.

The CLI should remain predictable before it becomes clever.

## 2. Send mode

```sh
qshare FILE
```

shares a file.

Phase 1 also supports an explicit session lifetime:

```sh
qshare --expire DURATION FILE
```

`DURATION` uses Go duration syntax, such as `30s`, `10m`, or `1h30m`, and must
be greater than zero. The default lifetime is `10m`. An invalid duration is a
CLI usage error and produces exit status `2`.

Phase 1 listens on TCP port `55544`. If a host firewall blocks inbound LAN
connections, the user must permit that port separately. qshare does not modify
firewall rules in Phase 1.

Later:

```sh
qshare FILE...
```

shares multiple files.

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

Receiving text from the browser and clipboard integration are deferred. The
intended clipboard behavior is to keep qshare running and invoke a configured
integration once for each text submission. It is not specified as
`qshare | wl-copy`, because a plain pipeline treats the entire session as one
input stream rather than a sequence of independently handled submissions.

## 4. stdin

Later versions may treat non-terminal stdin as input to share.

Example:

```sh
cat notes.txt | qshare
```

Behavior when both positional files and piped stdin are present must be explicitly defined before stdin support is released.

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

## 8. Send options

Phase 1 supports:

```sh
qshare --lan FILE
qshare --expire DURATION FILE
```

`--lan` explicitly requests LAN behavior.

`--expire` sets the Phase 1 session lifetime as described in the send-mode
contract above.

Once Direct Mode exists, the default strategy may become automatic.

Future flags may include:

```text
--direct
--once
--text TEXT
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

Receive:

```sh
qshare
```

Share piped text, planned:

```sh
printf 'hello\n' | qshare
```
