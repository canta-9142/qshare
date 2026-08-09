# Security Policy

## Supported versions

qshare is currently pre-1.0 software.

Security fixes are generally applied to the latest development version and the most recent published release when practical.

## Reporting a vulnerability

Please do not disclose suspected vulnerabilities in a public issue, discussion, or pull request before maintainers have had an opportunity to investigate them.

Report vulnerabilities privately through the repository hosting platform's private security reporting mechanism when available.

If no private reporting mechanism is available, use the maintainer contact method published with the project.

Please include, when possible:

* affected version or commit;
* operating system;
* reproduction steps;
* expected behavior;
* observed behavior;
* potential impact;
* proof-of-concept details that are necessary to reproduce the issue.

Avoid including unrelated private data.

## Scope

Security-sensitive areas include:

* unauthorized file access;
* directory or symlink traversal;
* session-token bypass;
* unsafe uploads;
* unintended file overwrite;
* remote code execution;
* denial of service;
* temporary hotspot security;
* exposure of resources outside the sharing session.

The project's internal security design is documented in [`docs/security.md`](docs/security.md).
