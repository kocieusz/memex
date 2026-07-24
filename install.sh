#!/bin/sh
# Install memex from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/kocieusz/memex/main/install.sh | sh
#
# Environment:
#   MEMEX_VERSION          version to install (default: latest release)
#   MEMEX_INSTALL_DIR      where the binary goes (default: ~/.memex/bin)
#   MEMEX_NO_MODIFY_PATH   set to 1 to skip adding the install dir to your shell rc

set -eu

REPO="kocieusz/memex"
BIN="memex"

say() { printf '%s\n' "$*"; }
warn() { printf '\033[33m!\033[0m %s\n' "$*" >&2; }
die() { printf '\033[31merror\033[0m %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

detect_os() {
	case "$(uname -s)" in
	Darwin) echo darwin ;;
	Linux) echo linux ;;
	*) die "unsupported OS $(uname -s) — build from source with: go install github.com/$REPO@latest" ;;
	esac
}

detect_arch() {
	case "$(uname -m)" in
	arm64 | aarch64) echo arm64 ;;
	x86_64 | amd64) echo amd64 ;;
	*) die "unsupported architecture $(uname -m) — build from source with: go install github.com/$REPO@latest" ;;
	esac
}

# latest_version follows the /releases/latest redirect rather than calling the
# API, which keeps the install off GitHub's unauthenticated rate limit.
latest_version() {
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
		die "could not reach GitHub to resolve the latest release"
	tag=${url##*/}
	[ -n "$tag" ] && [ "$tag" != "releases" ] || die "no published release found for $REPO"
	echo "$tag"
}

sha256() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	elif command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		die "need shasum or sha256sum to verify the download"
	fi
}

# rc_file picks the startup file for the user's login shell. zsh reads .zshrc
# for interactive shells; bash on macOS reads .bash_profile, elsewhere .bashrc.
rc_file() {
	case "${SHELL##*/}" in
	zsh) echo "${ZDOTDIR:-$HOME}/.zshrc" ;;
	bash) if [ "$(uname -s)" = Darwin ]; then echo "$HOME/.bash_profile"; else echo "$HOME/.bashrc"; fi ;;
	*) echo "" ;;
	esac
}

ensure_path() {
	dir=$1
	case ":$PATH:" in
	*":$dir:"*) return 0 ;;
	esac

	line="export PATH=\"$dir:\$PATH\""
	rc=$(rc_file)
	if [ "${MEMEX_NO_MODIFY_PATH:-}" = 1 ] || [ -z "$rc" ]; then
		warn "$dir is not on your PATH — add this to your shell startup file:"
		say "    $line"
		return 0
	fi
	if [ -f "$rc" ] && grep -qF "$dir" "$rc"; then
		warn "$dir is already in $rc but not in this shell's PATH — open a new terminal"
		return 0
	fi
	printf '\n# added by memex install.sh\n%s\n' "$line" >>"$rc"
	say "added $dir to PATH in $rc"
	PATH_HINT="run 'source $rc' (or open a new terminal) to pick up memex"
}

# warn_duplicates flags other memex binaries on PATH, so an old copy (typically
# a leftover from `go install` in ~/go/bin) can't silently shadow this one.
warn_duplicates() {
	installed=$1
	oldifs=$IFS
	IFS=:
	for p in $PATH; do
		IFS=$oldifs
		if [ -n "$p" ] && [ -x "$p/$BIN" ] && [ "$p/$BIN" != "$installed" ]; then
			warn "another memex is on your PATH at $p/$BIN — remove it with: rm $p/$BIN"
		fi
		IFS=:
	done
	IFS=$oldifs
}

main() {
	need curl
	need tar
	need uname

	os=$(detect_os)
	arch=$(detect_arch)
	tag=${MEMEX_VERSION:-$(latest_version)}
	case "$tag" in v*) ;; *) tag="v$tag" ;; esac
	version=${tag#v}

	dir=${MEMEX_INSTALL_DIR:-$HOME/.memex/bin}
	archive="${BIN}_${version}_${os}_${arch}.tar.gz"
	base="https://github.com/$REPO/releases/download/$tag"

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	say "downloading $BIN $tag ($os/$arch)"
	curl -fsSL "$base/$archive" -o "$tmp/$archive" ||
		die "no release asset $archive — check https://github.com/$REPO/releases"
	curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" ||
		die "could not download checksums.txt for $tag"

	want=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)
	[ -n "$want" ] || die "$archive is missing from checksums.txt"
	got=$(sha256 "$tmp/$archive")
	[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"

	tar -xzf "$tmp/$archive" -C "$tmp" "$BIN" || die "could not extract $BIN from $archive"

	mkdir -p "$dir" || die "could not create $dir"
	[ -w "$dir" ] || die "$dir is not writable — set MEMEX_INSTALL_DIR to somewhere you own"

	# Stage in the destination directory so the final move is atomic and
	# replaces any existing binary in place, leaving no second copy behind.
	cp "$tmp/$BIN" "$dir/.$BIN.new"
	chmod 0755 "$dir/.$BIN.new"
	mv -f "$dir/.$BIN.new" "$dir/$BIN"

	say "installed $BIN $tag to $dir/$BIN"

	# Always ensure the default skill library exists so `memex` has somewhere to
	# link from on a fresh machine, no matter where the binary was installed.
	library="$HOME/.memex/skills"
	if mkdir -p "$library" 2>/dev/null; then
		say "skill library ready at $library"
	fi

	PATH_HINT=""
	ensure_path "$dir"
	warn_duplicates "$dir/$BIN"
	[ -z "$PATH_HINT" ] || say "$PATH_HINT"
	say "run 'memex' to get started, 'memex upgrade' to update later"
}

main "$@"
