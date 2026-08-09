# ADR-0005: Phase 1 File Selection and Symbolic Links

Status: Accepted

## Context

Phase 1 shares a single local file. Symbolic links create two separate questions:
whether the path selected by the user may itself be a link, and whether any
ancestor directory used to reach that path may be a link.

Rejecting all paths that traverse a symbolic-link directory would make ordinary
filesystem layouts unnecessarily difficult to use. Accepting a symbolic link as
the selected file would make the CLI argument less explicit about the object
being shared.

## Decision

Phase 1 shares regular files only.

If the selected final path component is a symbolic link, qshare rejects it even
when the link resolves to a regular file.

Symbolic links in ancestor directories are allowed. After traversing those
directories, qshare validates that the selected object is a regular file before
creating the sharing session.

The validated resource is fixed when the session is created. Remote HTTP input
never selects a local filesystem path.

## Consequences

### Positive

* the user explicitly selects the file object exposed by Phase 1;
* common layouts containing linked directories remain usable;
* directories and non-regular file types remain outside the Phase 1 surface;
* HTTP authorization continues to operate on a prevalidated resource.

### Negative

* a convenient symlink directly naming a file cannot be shared without selecting
  its target path;
* path validation must distinguish the final path component from its ancestors;
* filesystem changes after validation still require careful handling by the
  implementation.

## Deferred decisions

Directory-sharing symlink behavior is not defined here. It must be decided before
directory sharing is released.
