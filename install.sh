#!/bin/sh
# install.sh — download and install the `revision` binary.
#
# The download is checked against the SHA-256 the release publishes, and is
# staged inside the install directory so the final move is an atomic rename.
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

# Releases before the four-target matrix carry only darwin-arm64 and linux-amd64, so a
# pinned older tag can resolve a URL that never existed. Always offer the source build.
download_failed() {
	err "download failed: ${url}
    This release may not include ${asset}. Build from source instead:
    go install github.com/${REPO}/cmd/${BINARY}@latest"
}

# download fetches url into the file at path. --proto-redir stops a redirect
# from downgrading the transport, which matters because the release download is
# served by one.
download() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL --proto-redir '=https' --tlsv1.2 "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -qO "$2" "$1"
	else
		err "need curl or wget to download"
	fi
}

# sha256 prints the SHA-256 of a file using whichever of the three usual tools
# the host has.
sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		line=$(sha256sum "$1")
		echo "${line%% *}"
	elif command -v shasum >/dev/null 2>&1; then
		line=$(shasum -a 256 "$1")
		echo "${line%% *}"
	elif command -v openssl >/dev/null 2>&1; then
		line=$(openssl dgst -sha256 "$1")
		echo "${line##*= }"
	else
		err "need sha256sum, shasum or openssl to verify the download"
	fi
}

# verify checks the downloaded file against the SHA-256 the release published
# for it. There is no way to skip this: a binary that cannot be checked is not
# one to install over the copy already on the machine.
verify() {
	sums="${tmp}/checksums.txt"
	download "${release}/checksums.txt" "$sums" ||
		err "cannot verify ${asset}: this release publishes no checksums.txt"

	expected=""
	while read -r sum name; do
		# GNU tools mark a binary-mode digest with a leading asterisk.
		if [ "${name#\*}" = "$asset" ]; then
			expected="$sum"
			break
		fi
	done <"$sums"
	[ -n "$expected" ] || err "cannot verify ${asset}: it is not listed in checksums.txt"

	actual=$(sha256 "$1")
	[ "$actual" = "$expected" ] || err "checksum mismatch for ${asset}
    expected ${expected}
    got      ${actual}
    The download was corrupted or tampered with; nothing was installed."
}

# install_dir prints where the binary belongs.
install_dir() {
	if [ -n "${REVISION_INSTALL_DIR:-}" ]; then
		echo "$REVISION_INSTALL_DIR"
	elif [ -w /usr/local/bin ] 2>/dev/null; then
		echo "/usr/local/bin"
	else
		echo "${HOME}/.local/bin"
	fi
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
	asset="${BINARY}-${target}"
	version="${REVISION_VERSION:-}"
	# REVISION_BASE_URL is not for general use; it exists so the test suite can
	# serve releases from a local server.
	base="${REVISION_BASE_URL:-https://github.com}"
	if [ -n "$version" ]; then
		release="${base}/${REPO}/releases/download/${version}"
	else
		release="${base}/${REPO}/releases/latest/download"
	fi
	url="${release}/${asset}"

	dir=$(install_dir)
	mkdir -p "$dir" || err "cannot create install directory: $dir"
	# Staging inside the destination keeps the final move a rename on one
	# filesystem. Staging in $TMPDIR would make it a copy whenever the two are on
	# different mounts, which can tear or hit ETXTBSY against a running binary.
	tmp=$(mktemp -d "${dir}/.${BINARY}-install.XXXXXX") ||
		err "cannot write to ${dir}; set REVISION_INSTALL_DIR to a writable directory"
	trap 'rm -rf "$tmp"' EXIT

	echo "Downloading ${asset} ..."
	download "$url" "$tmp/${BINARY}" || download_failed
	verify "$tmp/${BINARY}"
	chmod +x "$tmp/${BINARY}"

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
