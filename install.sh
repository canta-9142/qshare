#!/bin/sh

set -eu

repository="canta-9142/qshare"
requested_version="latest"
bin_dir=""

usage() {
    cat <<'EOF'
Install qshare from GitHub Releases.

Usage: install.sh [--version VERSION] [--bin-dir DIR]

Options:
  --version VERSION  Install a stable version such as v0.7.0 (default: latest)
  --bin-dir DIR      Install into an absolute directory (default: ~/.local/bin)
  -h, --help         Print this help and exit
EOF
}

die() {
    printf 'qshare installer: %s\n' "$*" >&2
    exit 1
}

normalize_version() {
    version_input=$1
    candidate=$version_input
    case "$candidate" in
        v*) ;;
        *) candidate="v$candidate" ;;
    esac

    version_components=${candidate#v}
    major=${version_components%%.*}
    remaining=${version_components#*.}
    [ "$remaining" != "$version_components" ] || die "invalid version: $version_input (expected a stable version such as v0.7.0)"
    minor=${remaining%%.*}
    patch=${remaining#*.}
    [ "$patch" != "$remaining" ] || die "invalid version: $version_input (expected a stable version such as v0.7.0)"
    case "$patch" in
        *.*) die "invalid version: $version_input (expected a stable version such as v0.7.0)" ;;
    esac

    for component in "$major" "$minor" "$patch"; do
        case "$component" in
            '' | *[!0-9]*) die "invalid version: $version_input (expected a stable version such as v0.7.0)" ;;
            0 | [1-9] | [1-9][0-9]*) ;;
            *) die "invalid version: $version_input (expected a stable version such as v0.7.0)" ;;
        esac
    done
    printf '%s\n' "$candidate"
}

normalize_requested_version() {
    case "$1" in
        latest) printf 'latest\n' ;;
        *) normalize_version "$1" ;;
    esac
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --version)
            [ "$#" -ge 2 ] || die "--version requires a value"
            requested_version=$(normalize_requested_version "$2")
            shift 2
            ;;
        --version=*)
            requested_version=$(normalize_requested_version "${1#*=}")
            shift
            ;;
        --bin-dir)
            [ "$#" -ge 2 ] || die "--bin-dir requires a value"
            [ -n "$2" ] || die "--bin-dir requires a value"
            bin_dir=$2
            shift 2
            ;;
        --bin-dir=*)
            bin_dir=${1#*=}
            [ -n "$bin_dir" ] || die "--bin-dir requires a value"
            shift
            ;;
        -h | --help)
            usage
            exit 0
            ;;
        *)
            die "unknown argument: $1"
            ;;
    esac
done

for command_name in awk chmod id install mkdir mktemp mv rm sha256sum uname; do
    command -v "$command_name" >/dev/null 2>&1 || die "$command_name is required"
done

if [ "$(uname -s)" != "Linux" ]; then
    die "only Linux is supported"
fi

case "$(uname -m)" in
    x86_64 | amd64)
        arch="amd64"
        ;;
    aarch64 | arm64)
        arch="arm64"
        ;;
    *)
        die "unsupported architecture: $(uname -m)"
        ;;
esac

if [ -z "$bin_dir" ]; then
    [ -n "${HOME:-}" ] || die "HOME is not set; use --bin-dir DIR"
    if [ "$(id -u)" -eq 0 ]; then
        die "refusing a default installation as root; specify --bin-dir explicitly"
    fi
    bin_dir="$HOME/.local/bin"
fi

case "$bin_dir" in
    /*) ;;
    *) die "--bin-dir must be an absolute path" ;;
esac

if command -v curl >/dev/null 2>&1; then
    download() {
        download_url=$1
        download_destination=$2
        if ! curl -fsSL --proto '=https' --tlsv1.2 -o "$download_destination" "$download_url"; then
            die "download failed: $download_url"
        fi
    }
elif command -v wget >/dev/null 2>&1; then
    download() {
        download_url=$1
        download_destination=$2
        if ! wget -q -O "$download_destination" "$download_url"; then
            die "download failed: $download_url"
        fi
    }
else
    die "curl or wget is required"
fi

asset="qshare-linux-$arch"
if [ "$requested_version" = "latest" ]; then
    release_base="https://github.com/$repository/releases/latest/download"
else
    release_base="https://github.com/$repository/releases/download/$requested_version"
fi

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/qshare-install.XXXXXX") || die "cannot create a temporary directory"
destination_temp=""

cleanup() {
    if [ -n "$destination_temp" ]; then
        rm -f "$destination_temp"
    fi
    rm -rf "$temp_dir"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

checksum_path="$temp_dir/checksums.txt"
binary_path="$temp_dir/$asset"

download "$release_base/checksums.txt" "$checksum_path"
download "$release_base/$asset" "$binary_path"

checksum_count=$(awk -v name="$asset" 'NF == 2 && $2 == name { count++ } END { print count + 0 }' "$checksum_path")
[ "$checksum_count" -eq 1 ] || die "checksums.txt must contain exactly one entry for $asset"

expected_checksum=$(awk -v name="$asset" 'NF == 2 && $2 == name { print $1 }' "$checksum_path")
if [ "${#expected_checksum}" -ne 64 ]; then
    die "invalid SHA-256 checksum for $asset"
fi
case "$expected_checksum" in
    *[!0-9a-f]*) die "invalid SHA-256 checksum for $asset" ;;
esac

actual_checksum=$(sha256sum "$binary_path" | awk '{ print $1 }')
[ "$actual_checksum" = "$expected_checksum" ] || die "checksum verification failed for $asset"

chmod 0755 "$binary_path" || die "cannot make downloaded binary executable"
reported_version=$("$binary_path" --version) || die "downloaded binary could not report its version"
case "$reported_version" in
    "qshare "*) installed_version=${reported_version#qshare } ;;
    *) die "downloaded binary returned invalid version output" ;;
esac
installed_version=$(normalize_version "$installed_version")

if [ "$requested_version" != "latest" ] && [ "$installed_version" != "$requested_version" ]; then
    die "downloaded binary reports $installed_version, expected $requested_version"
fi

mkdir -p "$bin_dir" || die "cannot create install directory: $bin_dir"
destination="$bin_dir/qshare"
[ ! -d "$destination" ] || die "install destination is a directory: $destination"

destination_temp=$(mktemp "$bin_dir/.qshare.XXXXXX") || die "cannot create a temporary file in $bin_dir"
install -m 0755 "$binary_path" "$destination_temp" || die "cannot prepare installed binary"
mv -f "$destination_temp" "$destination" || die "cannot install qshare to $destination"
destination_temp=""

printf 'Installed qshare %s to %s\n' "$installed_version" "$destination"

case ":${PATH:-}:" in
    *":$bin_dir:"*) ;;
    *) printf 'Add %s to PATH to run qshare.\n' "$bin_dir" >&2 ;;
esac
