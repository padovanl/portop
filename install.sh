#!/bin/sh
# Installs the latest (or a pinned) portop release for your OS/arch.
#
#   curl -fsSL https://raw.githubusercontent.com/padovanl/portop/main/install.sh | sh
#
# Env overrides:
#   PORTOP_VERSION=v0.3.0   install a specific tag instead of latest
#   PORTOP_INSTALL_DIR=...  install location (default: /usr/local/bin if
#                           writable/root, otherwise ~/.local/bin)

set -eu

REPO="padovanl/portop"

info()  { printf '\033[1;34m==>\033[0m %s\n' "$1"; }
warn()  { printf '\033[1;33m!!\033[0m %s\n' "$1" >&2; }
die()   { printf '\033[1;31merror:\033[0m %s\n' "$1" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "'$1' is required but not found in PATH"
}

need curl
need tar
need uname

os="$(uname -s)"
case "$os" in
  Linux) os=linux ;;
  Darwin) die "portop is Linux-only: it reads /proc/net directly, which doesn't exist on macOS" ;;
  *) die "unsupported OS: $os (portop is Linux-only)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

version="${PORTOP_VERSION:-}"
if [ -z "$version" ]; then
  info "looking up the latest release..."
  version="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  [ -n "$version" ] || die "could not determine the latest release; set PORTOP_VERSION=vX.Y.Z and retry"
fi

version_num="${version#v}"
archive="portop_${version_num}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${version}"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

info "downloading portop ${version} for ${os}/${arch}..."
curl -fsSL "${base_url}/${archive}" -o "${workdir}/${archive}" \
  || die "download failed: ${base_url}/${archive} (does that release/asset exist?)"

if curl -fsSL "${base_url}/checksums.txt" -o "${workdir}/checksums.txt" 2>/dev/null; then
  info "verifying checksum..."
  expected="$(grep " ${archive}\$" "${workdir}/checksums.txt" | awk '{print $1}')"
  if [ -n "$expected" ]; then
    actual="$(cd "$workdir" && (sha256sum "$archive" 2>/dev/null || shasum -a 256 "$archive") | awk '{print $1}')"
    [ "$expected" = "$actual" ] || die "checksum mismatch for ${archive} (expected ${expected}, got ${actual})"
  else
    warn "archive not listed in checksums.txt, skipping verification"
  fi
else
  warn "could not fetch checksums.txt, skipping verification"
fi

info "extracting..."
tar -xzf "${workdir}/${archive}" -C "$workdir" portop
chmod +x "${workdir}/portop"

install_dir="${PORTOP_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ "$(id -u)" = "0" ] || [ -w "/usr/local/bin" ]; then
    install_dir="/usr/local/bin"
  else
    install_dir="${HOME}/.local/bin"
  fi
fi
mkdir -p "$install_dir"

mv "${workdir}/portop" "${install_dir}/portop"
info "installed to ${install_dir}/portop"

case ":$PATH:" in
  *":${install_dir}:"*) ;;
  *) warn "${install_dir} is not on your PATH — add it, e.g.: export PATH=\"${install_dir}:\$PATH\"" ;;
esac

if command -v portop >/dev/null 2>&1; then
  info "done — $(portop --version)"
else
  info "done — run ${install_dir}/portop"
fi
