#!/bin/sh
# install.sh — download and install the `revision` binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bapatchirag/revision/main/install.sh | sh
#
# Environment:
#   REVISION_VERSION      release tag to install (default: latest)
#   REVISION_INSTALL_DIR  install directory (default: /usr/local/bin if writable, else ~/.local/bin)
set -eu

REPO="bapatchirag/revision"
BINARY="revision"

err() {
	echo "install.sh: $*" >&2
	exit 1
}

main() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)

	case "$os" in
	linux | darwin) ;;
	*) err "unsupported OS: $os" ;;
	esac

	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) err "unsupported architecture: $arch" ;;
	esac

	target="${os}-${arch}"
	case "$target" in
	darwin-arm64 | linux-amd64) ;;
	*)
		err "no prebuilt binary for ${target}. Build from source instead:
    go install github.com/${REPO}/cmd/${BINARY}@latest"
		;;
	esac

	asset="${BINARY}-${target}"
	version="${REVISION_VERSION:-}"
	if [ -n "$version" ]; then
		url="https://github.com/${REPO}/releases/download/${version}/${asset}"
	else
		url="https://github.com/${REPO}/releases/latest/download/${asset}"
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading ${asset} ..."
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$tmp/${BINARY}" || err "download failed: $url"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$tmp/${BINARY}" "$url" || err "download failed: $url"
	else
		err "need curl or wget to download"
	fi
	chmod +x "$tmp/${BINARY}"

	# Choose a writable install directory.
	if [ -n "${REVISION_INSTALL_DIR:-}" ]; then
		dir="$REVISION_INSTALL_DIR"
	elif [ -w /usr/local/bin ] 2>/dev/null; then
		dir="/usr/local/bin"
	else
		dir="$HOME/.local/bin"
	fi
	mkdir -p "$dir" || err "cannot create install directory: $dir"

	dest="${dir}/${BINARY}"
	mv "$tmp/${BINARY}" "$dest" ||
		err "cannot write to ${dir}; set REVISION_INSTALL_DIR to a writable directory"

	echo "Installed ${BINARY} to ${dest}"
	case ":${PATH}:" in
	*":${dir}:"*) ;;
	*) echo "Note: ${dir} is not on your PATH; add it to run '${BINARY}'." ;;
	esac
}

main "$@"
