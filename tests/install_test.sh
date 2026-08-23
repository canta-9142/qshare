#!/bin/sh

set -eu

CDPATH=
export CDPATH
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
installer="$repo_dir/install.sh"
test_root=$(mktemp -d "${TMPDIR:-/tmp}/qshare-install-test.XXXXXX")
fake_bin="$test_root/bin"
fixtures="$test_root/fixtures"
download_log="$test_root/download.log"

cleanup() {
    rm -rf "$test_root"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

fail() {
    printf 'install test: %s\n' "$*" >&2
    exit 1
}

assert_contains() {
    file=$1
    value=$2
    grep -F -- "$value" "$file" >/dev/null || fail "$file does not contain: $value"
}

mkdir -p "$fake_bin" "$fixtures"

cat >"$fixtures/qshare-linux-amd64" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
    printf 'qshare v1.2.3\n'
    exit 0
fi
exit 1
EOF
chmod 0755 "$fixtures/qshare-linux-amd64"
cp "$fixtures/qshare-linux-amd64" "$fixtures/qshare-linux-arm64"

write_checksums() {
    sha256sum "$fixtures/qshare-linux-amd64" "$fixtures/qshare-linux-arm64" |
        awk '{ name = $2; sub(/^.*\//, "", name); print $1 "  " name }' >"$fixtures/checksums.txt"
}
write_checksums

cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
set -eu

output=""
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o)
            output=$2
            shift 2
            ;;
        --proto)
            shift 2
            ;;
        -fsSL | --tlsv1.2)
            shift
            ;;
        https://*)
            url=$1
            shift
            ;;
        *)
            printf 'unexpected curl argument: %s\n' "$1" >&2
            exit 1
            ;;
    esac
done

[ -n "$output" ] && [ -n "$url" ]
printf '%s\n' "$url" >>"$INSTALL_TEST_DOWNLOAD_LOG"

case "$url" in
    */checksums.txt) cp "$INSTALL_TEST_FIXTURES/checksums.txt" "$output" ;;
    */qshare-linux-amd64) cp "$INSTALL_TEST_FIXTURES/qshare-linux-amd64" "$output" ;;
    */qshare-linux-arm64) cp "$INSTALL_TEST_FIXTURES/qshare-linux-arm64" "$output" ;;
    *) exit 1 ;;
esac
chmod 0644 "$output"
EOF
chmod 0755 "$fake_bin/curl"

cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
    -s) printf '%s\n' "${INSTALL_TEST_OS:-Linux}" ;;
    -m) printf '%s\n' "${INSTALL_TEST_ARCH:-x86_64}" ;;
    *) exit 1 ;;
esac
EOF
chmod 0755 "$fake_bin/uname"

cat >"$fake_bin/id" <<'EOF'
#!/bin/sh
[ "${1:-}" = "-u" ] || exit 1
printf '%s\n' "${INSTALL_TEST_UID:-1000}"
EOF
chmod 0755 "$fake_bin/id"

run_installer() {
    env \
        HOME="$test_root/home" \
        INSTALL_TEST_DOWNLOAD_LOG="$download_log" \
        INSTALL_TEST_FIXTURES="$fixtures" \
        PATH="$fake_bin:$PATH" \
        sh "$installer" "$@"
}

test_latest_default_install() {
    output="$test_root/latest-output"
    : >"$download_log"

    run_installer >"$output" 2>&1

    installed="$test_root/home/.local/bin/qshare"
    [ -x "$installed" ] || fail "latest binary was not installed"
    [ "$("$installed" --version)" = "qshare v1.2.3" ] || fail "installed version is incorrect"
    assert_contains "$download_log" "/releases/latest/download/checksums.txt"
    assert_contains "$download_log" "/releases/latest/download/qshare-linux-amd64"
    assert_contains "$output" "Installed qshare v1.2.3"
}

test_pinned_install() {
    output="$test_root/pinned-output"
    target="$test_root/pinned-bin"
    : >"$download_log"

    run_installer --version 1.2.3 --bin-dir "$target" >"$output" 2>&1

    [ "$("$target/qshare" --version)" = "qshare v1.2.3" ] || fail "pinned version is incorrect"
    assert_contains "$download_log" "/releases/download/v1.2.3/checksums.txt"
    assert_contains "$download_log" "/releases/download/v1.2.3/qshare-linux-amd64"
}

test_explicit_latest_install() {
    output="$test_root/explicit-latest-output"
    target="$test_root/explicit-latest-bin"
    : >"$download_log"

    run_installer --version latest --bin-dir "$target" >"$output" 2>&1

    [ "$("$target/qshare" --version)" = "qshare v1.2.3" ] || fail "explicit latest version is incorrect"
    assert_contains "$download_log" "/releases/latest/download/qshare-linux-amd64"
}

test_arm64_install() {
    output="$test_root/arm64-output"
    target="$test_root/arm64-bin"
    : >"$download_log"

    INSTALL_TEST_ARCH=aarch64 run_installer --bin-dir "$target" >"$output" 2>&1

    [ "$("$target/qshare" --version)" = "qshare v1.2.3" ] || fail "arm64 version is incorrect"
    assert_contains "$download_log" "/releases/latest/download/qshare-linux-arm64"
}

test_checksum_failure_preserves_existing_binary() {
    output="$test_root/checksum-output"
    target="$test_root/checksum-bin"
    mkdir -p "$target"
    cat >"$target/qshare" <<'EOF'
#!/bin/sh
printf 'existing binary\n'
EOF
    chmod 0755 "$target/qshare"
    printf '%064d  qshare-linux-amd64\n' 0 >"$fixtures/checksums.txt"

    if run_installer --bin-dir "$target" >"$output" 2>&1; then
        fail "checksum mismatch was accepted"
    fi

    [ "$("$target/qshare")" = "existing binary" ] || fail "existing binary was replaced"
    assert_contains "$output" "checksum verification failed"
    write_checksums
}

test_duplicate_checksum_is_rejected() {
    output="$test_root/duplicate-output"
    target="$test_root/duplicate-bin"
    duplicate_checksum=$(awk '$2 == "qshare-linux-amd64" { print }' "$fixtures/checksums.txt")
    printf '%s\n' "$duplicate_checksum" >>"$fixtures/checksums.txt"

    if run_installer --bin-dir "$target" >"$output" 2>&1; then
        fail "duplicate checksum was accepted"
    fi

    [ ! -e "$target/qshare" ] || fail "duplicate checksum installed a binary"
    assert_contains "$output" "exactly one entry for qshare-linux-amd64"
    write_checksums
}

test_invalid_version_is_rejected_before_download() {
    output="$test_root/version-output"
    : >"$download_log"

    if run_installer --version v01.2.3 >"$output" 2>&1; then
        fail "invalid version was accepted"
    fi

    [ ! -s "$download_log" ] || fail "invalid version triggered a download"
    assert_contains "$output" "invalid version"
}

test_version_mismatch_preserves_existing_binary() {
    output="$test_root/mismatch-output"
    target="$test_root/mismatch-bin"
    mkdir -p "$target"
    cat >"$target/qshare" <<'EOF'
#!/bin/sh
printf 'existing binary\n'
EOF
    chmod 0755 "$target/qshare"

    if run_installer --version v1.2.4 --bin-dir "$target" >"$output" 2>&1; then
        fail "version mismatch was accepted"
    fi

    [ "$("$target/qshare")" = "existing binary" ] || fail "existing binary was replaced"
    assert_contains "$output" "reports v1.2.3, expected v1.2.4"
}

test_unsupported_architecture_is_rejected() {
    output="$test_root/arch-output"
    : >"$download_log"

    if INSTALL_TEST_ARCH=riscv64 run_installer >"$output" 2>&1; then
        fail "unsupported architecture was accepted"
    fi

    [ ! -s "$download_log" ] || fail "unsupported architecture triggered a download"
    assert_contains "$output" "unsupported architecture: riscv64"
}

test_root_default_install_is_rejected() {
    output="$test_root/root-output"
    : >"$download_log"

    if INSTALL_TEST_UID=0 run_installer >"$output" 2>&1; then
        fail "default root installation was accepted"
    fi

    [ ! -s "$download_log" ] || fail "default root installation triggered a download"
    assert_contains "$output" "refusing a default installation as root"
}

test_relative_bin_dir_is_rejected() {
    output="$test_root/bin-dir-output"
    : >"$download_log"

    if run_installer --bin-dir relative/bin >"$output" 2>&1; then
        fail "relative install directory was accepted"
    fi

    [ ! -s "$download_log" ] || fail "relative install directory triggered a download"
    assert_contains "$output" "--bin-dir must be an absolute path"
}

test_latest_default_install
test_pinned_install
test_explicit_latest_install
test_arm64_install
test_checksum_failure_preserves_existing_binary
test_duplicate_checksum_is_rejected
test_invalid_version_is_rejected_before_download
test_version_mismatch_preserves_existing_binary
test_unsupported_architecture_is_rejected
test_root_default_install_is_rejected
test_relative_bin_dir_is_rejected

printf 'install tests passed\n'
