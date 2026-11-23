#!/bin/sh
# NFR-SEC-004: builds the app twice from one commit, in two fresh clones at
# different paths with separate Go build caches, and compares every file of the
# bundle and the bill of materials byte for byte. Exits 1 if any differ, if
# either build fails, or if a file it compares is missing from either.
#
#   scripts/verify-repro.sh [commit or tag]      (default HEAD)
#
# Same result needs the same Go (go.mod's toolchain), Node and Xcode tools on
# both sides: their linker and SDK shape the binary. A DMG is not compared: hdiutil
# stamps it with the time. A release signed with a Developer ID has a dated
# signature; to compare one, remove it first (codesign --remove-signature) and
# sign the same way (ad hoc) here.
#
# Per platform (NFR-XPLAT-003): on a Mac it compares the app bundle and its bill
# (`make release RELEASE_PLATFORMS=mac`), and on every host the two Windows exes
# (`make windows-exe`: pure Go, cross-compiled, so any host makes the same bytes).
# The Windows installer and zip are not compared: makensis and zip stamp times.
# Linux builds in Docker (`make linux-package`) and is not compared here; its
# .deb and AppImage carry times too.
set -eu
ref="${1:-HEAD}"
repo="$(git rev-parse --show-toplevel)"
commit="$(git -C "$repo" rev-parse --verify "$ref^{commit}")"
work="$(mktemp -d "${TMPDIR:-/tmp}/freeloop-repro.XXXXXX")"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

run_make() { # folder under $work, then make's arguments
	dir="$1"
	shift
	# Each make is checked on its own. The builds once ran in one `( … ) || { … }`,
	# where set -e is off, so a failed Mac build went on to the Windows exes and
	# was reported reproducible over those two files.
	if ! (cd "$work/$dir" && GOCACHE="$work/cache-$dir" GOFLAGS=-modcacherw make "$@") >>"$work/$dir.log" 2>&1; then
		tail -20 "$work/$dir.log"
		echo "the build in $dir failed: make $*" >&2
		exit 1
	fi
}

build() { # folder under $work
	git clone -q --no-hardlinks "$repo" "$work/$1"
	git -C "$work/$1" checkout -q "$commit"
	: >"$work/$1.log"
	if [ "$mac" = 1 ]; then
		run_make "$1" release RELEASE_PLATFORMS=mac
	fi
	run_make "$1" windows-exe
	files="windows-amd64/Freeloop.exe windows-arm64/Freeloop.exe"
	if [ "$mac" = 1 ]; then
		files="Freeloop.app release/Freeloop.sbom.cdx.json $files"
	fi
	# Every file compared must be there: find skips a missing one with a message
	# and the pipe's status is xargs', so two builds missing the same file matched.
	for f in $files; do
		n=$(find "$work/$1/bin/$f" -type f 2>/dev/null | wc -l | tr -d ' ')
		[ "$n" -gt 0 ] || {
			echo "the build in $1 made no bin/$f" >&2
			exit 1
		}
	done
	# shellcheck disable=SC2086 # the list is split on purpose
	(cd "$work/$1/bin" && find $files -type f | LC_ALL=C sort | xargs $sha) >"$work/$1.sums"
}
mac=0
[ "$(uname -s)" = Darwin ] && mac=1
sha="shasum -a 256"
command -v shasum >/dev/null 2>&1 || sha=sha256sum
echo "building $commit twice; this takes some minutes"
build first
build a-second-folder-with-a-longer-name
if diff "$work/first.sums" "$work/a-second-folder-with-a-longer-name.sums"; then
	echo "reproducible: $(wc -l <"$work/first.sums" | tr -d ' ') files match"
	if [ -f "$work/first/bin/release/BUILDINFO.txt" ]; then cat "$work/first/bin/release/BUILDINFO.txt"; fi
else
	echo "NOT reproducible: the files above differ" >&2
	exit 1
fi
