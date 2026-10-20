#!/usr/bin/env bash

set -euo pipefail

TOOLS_DIR="${GOG_SECURITY_TOOLS_DIR:-.tools/security}"
STAMP_FILE="$TOOLS_DIR/.versions"
MODE="${1:-all}"

case "$MODE" in
  fast|all) ;;
  *)
    echo "usage: $0 [fast|all]" >&2
    exit 2
    ;;
esac

mkdir -p "$TOOLS_DIR"
touch "$STAMP_FILE"

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

installed() {
  local name="$1" version="$2"
  [ -x "$TOOLS_DIR/$name" ] && grep -qx "$name=$version" "$STAMP_FILE"
}

download_binary() {
  local name="$1" version="$2" url="$3" checksum="$4"
  if installed "$name" "$version"; then
    return
  fi

  local archive
  archive="$DOWNLOAD_DIR/$name.archive"
  curl --fail --silent --show-error --location --output "$archive" "$url"
  local actual
  actual="$(sha256 "$archive")"
  if [ "$actual" != "$checksum" ]; then
    echo "checksum mismatch for $name: expected $checksum, got $actual" >&2
    exit 1
  fi
  install -m 0755 "$archive" "$TOOLS_DIR/$name"
  grep -vx "$name=$version" "$STAMP_FILE" > "$STAMP_FILE.tmp" || true
  printf '%s=%s\n' "$name" "$version" >> "$STAMP_FILE.tmp"
  mv "$STAMP_FILE.tmp" "$STAMP_FILE"
}

download_archive() {
  local name="$1" version="$2" url="$3" checksum="$4" member="$5"
  if installed "$name" "$version"; then
    return
  fi

  local archive extract_dir
  archive="$DOWNLOAD_DIR/$name.archive"
  extract_dir="$DOWNLOAD_DIR/$name-extract"
  mkdir -p "$extract_dir"
  curl --fail --silent --show-error --location --output "$archive" "$url"
  local actual
  actual="$(sha256 "$archive")"
  if [ "$actual" != "$checksum" ]; then
    echo "checksum mismatch for $name: expected $checksum, got $actual" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$extract_dir"
  install -m 0755 "$extract_dir/$member" "$TOOLS_DIR/$name"
  grep -vx "$name=$version" "$STAMP_FILE" > "$STAMP_FILE.tmp" || true
  printf '%s=%s\n' "$name" "$version" >> "$STAMP_FILE.tmp"
  mv "$STAMP_FILE.tmp" "$STAMP_FILE"
  rm -rf "$extract_dir"
}

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$ARCH" in
  arm64|aarch64) ARCH=arm64 ;;
  *)
    echo "security tools are currently supported on arm64 only" >&2
    exit 2
    ;;
esac

case "$OS" in
  Darwin)
    OSV_URL='https://github.com/google/osv-scanner/releases/download/v2.7.0/osv-scanner_darwin_arm64'
    OSV_SHA='d4926ef8da134332543be276dff157e4c170f99ad91fc48e89448a8816bdf70b'
    GITLEAKS_URL='https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_darwin_arm64.tar.gz'
    GITLEAKS_SHA='b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5'
    TRIVY_URL='https://github.com/aquasecurity/trivy/releases/download/v0.76.0/trivy_0.76.0_macOS-ARM64.tar.gz'
    TRIVY_SHA='e7ec0d21fe842553f59af99aba2efbcc822369fbaed27019c575408b6d135fce'
    ZIZMOR_URL='https://github.com/woodruffw/zizmor/releases/download/v1.31.1/zizmor-aarch64-apple-darwin.tar.gz'
    ZIZMOR_SHA='ad5aa633cd56215b11e6c51d51f90c7e6b22904a75047d1215abbd422f61d698'
    ;;
  Linux)
    OSV_URL='https://github.com/google/osv-scanner/releases/download/v2.7.0/osv-scanner_linux_arm64'
    OSV_SHA='4061ef4c021ed3c47f203b818f337e351336cc5cd67c0274fc36bd1dfc728dd7'
    GITLEAKS_URL='https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_arm64.tar.gz'
    GITLEAKS_SHA='e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080'
    TRIVY_URL='https://github.com/aquasecurity/trivy/releases/download/v0.76.0/trivy_0.76.0_Linux-ARM64.tar.gz'
    TRIVY_SHA='aab21e677a7b7071fc79f1711480840754c18a1b9d9cefa7ccd06af63a0e8f4c'
    ZIZMOR_URL='https://github.com/woodruffw/zizmor/releases/download/v1.31.1/zizmor-aarch64-unknown-linux-gnu.tar.gz'
    ZIZMOR_SHA='37eebbd5762a44f262d94875431b14a6d2402991e90a685113ea97b8b41be833'
    ;;
  *)
    echo "unsupported platform: $OS/$ARCH" >&2
    exit 2
    ;;
esac

# Each invocation owns one directory, cleaned on success or failure. CI points
# TMPDIR at RUNNER_TEMP, so interrupted jobs also get Actions lifecycle cleanup.
DOWNLOAD_DIR="$(mktemp -d "${TMPDIR:-/tmp}/gog-security-install.XXXXXX")"
trap 'rm -rf -- "$DOWNLOAD_DIR"' EXIT

download_archive gitleaks 8.30.1 "$GITLEAKS_URL" "$GITLEAKS_SHA" gitleaks
download_archive zizmor 1.31.1 "$ZIZMOR_URL" "$ZIZMOR_SHA" zizmor

if [ "$MODE" = "all" ]; then
  download_binary osv-scanner 2.7.0 "$OSV_URL" "$OSV_SHA"
  download_archive trivy 0.76.0 "$TRIVY_URL" "$TRIVY_SHA" trivy
fi

