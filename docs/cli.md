# CLI Contract

## Commands

### Send files

```sh
qshare FILE...
```

Accepts 1–100 regular files. Order is preserved. Supplying a directory requires
exactly one positional path:

```sh
qshare DIRECTORY
```

### Send text

```sh
qshare --text TEXT
printf 'text\n' | qshare
```

Text must be valid UTF-8 and at most 1 MiB. Non-terminal stdin selects text send
mode. It cannot be combined with positional paths, `--text`, `--clipboard`, or
`--receive-dir`.

### Receive

```sh
qshare
qshare --receive-dir DIR
qshare --clipboard BACKEND
```

No positional paths with terminal stdin selects receive mode. The default
directory is `~/Downloads/qshare`. `BACKEND` is `auto`, `wl-copy`, `xclip`, or
`xsel`; receive mode defaults to `auto`.

If `auto` finds no supported program, received text is written unchanged to
stdout. An explicitly selected missing or unknown backend is an error.

## Options

| Option | Meaning |
| --- | --- |
| `-p`, `--port PORT` | Set TCP port (`1`–`65535`); default: random `50000`–`59999` |
| `-e`, `--expire DURATION` | Set session lifetime; default `10m` |
| `-r`, `--receive-dir DIR` | Set the upload destination in receive mode |
| `-t`, `--text TEXT` | Share explicit UTF-8 text |
| `-c`, `--clipboard BACKEND` | Select receive-mode clipboard handling |
| `-v`, `--version` | Print the qshare version and exit |
| `--help` | Print help and exit |

Durations use Go syntax such as `30s`, `10m`, and `1h30m`, and must be greater
than zero. `--receive-dir` is receive-only. `--text` and `--clipboard` are
mutually exclusive and neither can be combined with file paths.

The reserved future options `--direct` and `--once` are not implemented.

## Runtime behavior

qshare listens on the selected LAN IPv4 address at a random TCP port from
`50000`–`59999`. If a candidate is already in use, it selects another one.
`--port` (or `-p`) selects a fixed port in every mode, including piped text.
Zero, out-of-range values, and non-integers are usage errors (exit code `2`).
If the specified port cannot be bound, qshare exits with code `1` without
trying another port.
Supported firewalls receive a temporary rule for the selected port. Every mode
prints a QR code and authenticated URL, then runs until the session expires,
receives a termination signal, the user presses `q`, or it encounters a fatal
server error. A completed transfer does not end the session.

When stdin is a terminal, qshare switches it to non-canonical, no-echo input
only after the server and firewall are ready and the QR code has been rendered.
It then prints `Press q to quit.` to stderr. Pressing `q` does not require Enter.
It stops new HTTP requests, gives active requests up to 30 seconds to finish,
drains accepted text submissions, restores the terminal, and exits successfully.
Terminal signal generation remains enabled, so Ctrl+C retains normal SIGINT
behavior instead of being treated as an input byte. Non-terminal stdin continues
to select text send mode and does not enable the quit key.

## Output streams

```text
stdout = composable data
stderr = interactive UI and diagnostics
```

QR codes, URLs, status, notices, and errors go to stderr. Received text goes to
stdout only when automatic clipboard selection finds no supported backend.
qshare adds no separator between submissions.

Help and version information go to stdout. Invalid command syntax and usage
messages go to stderr. Unstamped local builds report `qshare devel`; release
and package builds report their tagged version, such as `qshare v0.6.2`.

## Exit codes

| Code | Meaning |
| ---: | --- |
| `0` | Success, help, version output, normal expiration, or `q` shutdown |
| `1` | Runtime or internal failure |
| `2` | Invalid CLI usage or input |
| `130` | Terminated by SIGINT |
| `143` | Terminated by SIGTERM |

SIGINT and SIGTERM initiate cleanup. If cleanup also fails, the command exits
with `1`.

## Examples

```sh
# Share one or more files
qshare photo.jpg notes.txt

# Use a fixed port
qshare --port 8080 photo.jpg

# Share a directory
qshare ./photos

# Share text
qshare --text "hello"
printf 'hello\n' | qshare

# Receive into the default or a selected directory
qshare
qshare --receive-dir ./received

# Receive text as a stdout stream when no clipboard backend is available
qshare | COMMAND

# Display the installed version
qshare -v
```
