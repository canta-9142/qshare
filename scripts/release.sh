#!/usr/bin/env bash
set -euo pipefail

die() { printf 'release: %s\n' "$*" >&2; exit 1; }

if [[ ${1:-} == --help && $# == 1 ]]; then
    printf 'Usage: %s X.Y.Z\nUpdates packages, runs tests and vet, and creates a local commit and annotated tag. Does not push.\n' "$0"
    exit 0
fi
[[ $# == 1 ]] || die "usage: $0 X.Y.Z"
version=$1
[[ $version =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || die 'expected a stable version such as 0.7.0 (without v)'
tag="v$version"

cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
go version
git --version
[[ $(git rev-parse --show-toplevel) == "$PWD" ]] || die 'script must live in the repository root/scripts directory'
git symbolic-ref -q HEAD >/dev/null || die 'check out a branch before releasing'
[[ -z $(git status --porcelain --untracked-files=all) ]] || die 'working tree and index must be clean'
if git show-ref --verify --quiet "refs/tags/$tag"; then
    die "tag $tag already exists"
fi
author=$(git var GIT_AUTHOR_IDENT)
git var GIT_COMMITTER_IDENT >/dev/null
author=${author%>*}'>'
release_date=$(LC_ALL=C date -u '+%a %b %d %Y')

# Prepare both files before replacing either; reject unexpected layouts.
release_tmp=$(mktemp -d)
trap 'rm -f -- "$release_tmp/flake.nix" "$release_tmp/qshare.spec"; rmdir -- "$release_tmp"' EXIT
awk -v version="$version" '
    /^[[:space:]]*packageVersion = "[^"]*";$/ {
        sub(/"[^"]*"/, "\"" version "\""); count++
    }
    { print }
    END { if (count != 1) exit 1 }
' flake.nix > "$release_tmp/flake.nix" || die 'expected exactly one packageVersion assignment in flake.nix'
RELEASE_AUTHOR="$author" awk -v version="$version" -v date="$release_date" '
    /^Version:/ { $0 = "Version:        " version; versions++ }
    /^Release:/ { $0 = "Release:        1%{?dist}"; releases++ }
    { print }
    /^%changelog$/ {
        print "* " date " " ENVIRON["RELEASE_AUTHOR"] " - " version "-1"
        print "- Release version " version
        print ""
        changelogs++
    }
    END { if (versions != 1 || releases != 1 || changelogs != 1) exit 1 }
' qshare.spec > "$release_tmp/qshare.spec" || die 'expected one Version, Release, and changelog in qshare.spec'
cp -- "$release_tmp/flake.nix" flake.nix
cp -- "$release_tmp/qshare.spec" qshare.spec

printf 'Validating %s; on failure, package edits are left for inspection.\n' "$tag"
go test ./...
go vet ./...
git diff --check
git diff --cached --quiet || die 'index changed during validation'

git add -- flake.nix qshare.spec
git commit -m "chore: release $tag"
git tag -a "$tag" -m "qshare $tag" || die "release commit exists, but tag creation failed; inspect HEAD before creating $tag manually"
printf '\nCreated local release %s. Review with:\n  git show HEAD\n  git show %s\nPush the branch and tag when ready; nothing has been pushed.\n' "$tag" "$tag"
