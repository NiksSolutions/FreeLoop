#!/bin/sh
# The source of what the AppImage carries (OQ-059). linuxdeploy and its GTK plugin
# copy GTK 3, WebKitGTK 4.1 and what they use, libraries and data files, from the
# build image's Ubuntu packages into the AppImage, many under the LGPL, which asks
# for the source to come with them. This finds the Debian package of every file the
# AppImage carries and gathers each one's source as Ubuntu publishes it, at the
# version installed. It runs in the image from build/linux/Dockerfile, which keeps
# the source indexes it fetched with the packages, so they name those versions.
#
#   sources.sh copyrights <AppDir>
#       copies into the AppDir the Debian copyright file of each package it has files
#       of and no copyright file for: linuxdeploy copies those of libraries only
#       (build/linux/Taskfile.yml's appimage task, before it packs the AppDir again)
#   sources.sh archive <AppImage> <Freeloop-...-sources.tar>
#       fetches the source packages into the archive, with SOURCES.txt saying which
#       package and version each file of the AppImage came from, then checks it
#       (build/linux/package.sh)
#   sources.sh check <AppImage> <Freeloop-...-sources.tar>
#       fails unless every file in the AppImage is Freeloop's or an installed
#       package's, the AppImage holds each such package's copyright file, and the
#       archive holds each one's source, the .dsc and every file it lists with their
#       checksums, and nothing else
set -eu
tab=$(printf '\t')
triplet=$(dpkg-architecture -qDEB_HOST_MULTIARCH)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

die() {
	echo "sources.sh: $*" >&2
	exit 1
}

# Files no package has: Freeloop's own (AppRun is Wails'), the hook linuxdeploy's GTK
# plugin writes, and the caches it makes from the packages' files.
ours() {
	case "$1" in
	AppRun | .DirIcon | freeloop.png | freeloop.desktop | usr/bin/freeloop | usr/share/doc/freeloop/* | \
		apprun-hooks/linuxdeploy-plugin-gtk.sh | \
		usr/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache | usr/lib/gtk-3.0/3.0.0/immodules.cache | \
		usr/share/glib-2.0/schemas/gschemas.compiled) return 0 ;;
	esac
	return 1
}

# carried <root> <out> writes <out>.files, "package<TAB>file" for each file under
# <root> (an AppDir, or an AppImage taken apart) that is not ours, and
# <out>.packages, "package<TAB>version<TAB>source<TAB>source version" for each
# package, and fails on a file no installed package has. linuxdeploy puts every
# library in usr/lib, whichever folder it came from, so a library is looked for in
# each, by the real name of what the system has there first: the name it was
# copied under may be a link a -dev package owns, or one no package does.
carried() {
	root=$1 out=$2
	(cd "$root" && find . -type f | sed 's|^\./||' | LC_ALL=C sort) | while IFS= read -r f; do
		if ours "$f"; then continue; fi
		case "$f" in
		usr/share/doc/*/*)
			p=${f#usr/share/doc/}
			printf '%s\t=%s\n' "$f" "${p%%/*}"
			;;
		usr/lib/*)
			r=${f#usr/lib/}
			r=${r#"$triplet"/}
			for c in "/usr/lib/$triplet/$r" "/usr/lib/$r"; do
				if [ -e "$c" ]; then
					real=$(readlink -f "$c")
					printf '%s\t%s\n%s\t%s\n' "$f" "$real" "$f" "${real#/usr}"
				fi
				printf '%s\t%s\n%s\t%s\n' "$f" "$c" "$f" "${c#/usr}"
			done
			;;
		*) printf '%s\t/%s\n' "$f" "$f" ;;
		esac
	done >"$out.candidates"
	# dpkg's list of each package's files, then the candidates: the first one a
	# package has, or the package a usr/share/doc folder is named after.
	awk -F'\t' -v candidates="$out.candidates" '
		FILENAME != candidates {
			if (FNR == 1) { p = FILENAME; sub(/.*\//, "", p); sub(/\.list$/, "", p); sub(/:.*/, "", p) }
			if (!($0 in owner)) owner[$0] = p
			next
		}
		($1 in found) && found[$1] != "" { next }
		{ found[$1] = "" }
		substr($2, 1, 1) == "=" { found[$1] = substr($2, 2); next }
		$2 in owner { found[$1] = owner[$2] }
		END { for (f in found) print found[f] "\t" f }
	' /var/lib/dpkg/info/*.list "$out.candidates" | LC_ALL=C sort >"$out.files"
	unowned=$(sed -n "s/^$tab//p" "$out.files")
	[ -z "$unowned" ] || die "no installed package has these files of the AppImage (if the build makes one, add it to ours()):
$unowned"
	cut -f1 "$out.files" | LC_ALL=C sort -u | while IFS= read -r p; do
		dpkg-query -W -f '${db:Status-Abbrev}\t${Package}\t${Version}\t${source:Package}\t${source:Version}\n' "$p" 2>/dev/null |
			sed -n 's/^ii *\t//p' | grep . || die "no package $p is installed, which usr/share/doc/$p is named after"
	done >"$out.packages"
}

# The AppImage taken apart, in the work folder.
unpack() {
	appimage=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
	mkdir "$work/appimage"
	(cd "$work/appimage" && "$appimage" --appimage-extract >/dev/null)
	root="$work/appimage/squashfs-root"
}

copyrights() {
	carried "$1" "$work/carried"
	cut -f1 "$work/carried.packages" | while IFS= read -r p; do
		[ ! -e "$1/usr/share/doc/$p/copyright" ] || continue
		[ -f "/usr/share/doc/$p/copyright" ] || die "$p has no /usr/share/doc/$p/copyright to copy"
		mkdir -p "$1/usr/share/doc/$p"
		cp -L "/usr/share/doc/$p/copyright" "$1/usr/share/doc/$p/copyright"
		echo "sources.sh: added usr/share/doc/$p/copyright"
	done
}

# fetch <src> <version> <folder>: the source package's files, each checked against the
# checksum the image's index has for it. From the archive the image installed from,
# or, once a newer version has replaced it there, from Launchpad, which keeps every
# version Ubuntu published.
fetch() {
	uris=$(cd "$3" && apt-get source --only-source --print-uris "$1=$2" 2>/dev/null |
		sed -n "s/^'\([^']*\)' \([^ ]*\) [0-9]* \([A-Z0-9]*\):\([0-9a-f]*\)\$/\1 \2 \3 \4/p")
	[ -n "$uris" ] || die "the image's source indexes have no $1 $2 (build/linux/Dockerfile fetches them with the packages)"
	echo "$uris" | while read -r url name algo hash; do
		case "$algo" in
		SHA256) sum=sha256sum ;;
		SHA512) sum=sha512sum ;;
		*) die "$name: no SHA-256 or SHA-512 for it in the index, only $algo" ;;
		esac
		if ! curl -fsSL --retry 3 -o "$3/$name" "$url" 2>/dev/null; then
			echo "sources.sh: $name is no longer at $url; taking it from Launchpad"
			curl -fsSL --retry 3 -o "$3/$name" "https://launchpad.net/ubuntu/+archive/primary/+sourcefiles/$1/$2/$name" ||
				die "$name: not at $url, nor on Launchpad"
		fi
		echo "$hash  $3/$name" | "$sum" --check --strict --quiet - || die "$name: not the file the index lists"
	done
}

archive() {
	unpack "$1"
	tar=$(cd "$(dirname "$2")" && pwd)/$(basename "$2")
	base=$(basename "$tar" .tar)
	carried "$root" "$work/carried"
	dir="$work/archive/$base"
	mkdir -p "$dir"
	cut -f3,4 "$work/carried.packages" | LC_ALL=C sort -u | while IFS="$tab" read -r src ver; do
		mkdir "$dir/$src"
		fetch "$src" "$ver" "$dir/$src"
	done
	{
		cat <<-EOF
			The source of the Ubuntu packages $(basename "$1") carries

			The AppImage carries files from the $(. /etc/os-release && echo "$PRETTY_NAME") packages below, copied from
			the system it was built on (build/linux/Dockerfile in Freeloop's repository) by
			linuxdeploy and its GTK plugin. Each package's source is here as Ubuntu publishes
			it, at the version carried: a folder per source package, with its .dsc and the
			files that lists. "dpkg-source -x <name>.dsc" unpacks one; its debian/copyright
			says under which licenses it is, and the AppImage holds the same file under
			usr/share/doc/<package>.

			Each package, its version and its source, then the files of the AppImage it gave:
		EOF
		while IFS="$tab" read -r p ver src srcver; do
			echo
			echo "$p $ver, from $src/${src}_${srcver#*:}.dsc"
			awk -F'\t' -v p="$p" '$1 == p { print "  " $2 }' "$work/carried.files"
		done <"$work/carried.packages"
	} >"$dir/SOURCES.txt"
	(cd "$work/archive" && tar --sort=name --owner=0 --group=0 --numeric-owner --mode=a+rX,u+w,go-w --mtime=@0 -cf "$tar.tmp" "$base")
	mv "$tar.tmp" "$tar"
	verify "$root" "$tar"
	echo "$tar: $(wc -l <"$work/carried.packages") packages from $(cut -f3 "$work/carried.packages" | sort -u | wc -l) source packages, $(($(stat -c %s "$tar") / 1000000)) MB"
}

# verify <root> <tar>: what check says, for an AppImage already taken apart. It works
# out what the AppImage carries again rather than trust what archive wrote.
verify() {
	root=$1 tar=$2
	base=$(basename "$tar" .tar)
	carried "$root" "$work/check"
	cut -f1 "$work/check.packages" | while IFS= read -r p; do
		[ -f "$root/usr/share/doc/$p/copyright" ] || die "the AppImage carries files of $p and not its copyright file, usr/share/doc/$p/copyright"
	done
	rm -rf "$work/x" && mkdir "$work/x"
	tar -xf "$tar" -C "$work/x"
	[ -s "$work/x/$base/SOURCES.txt" ] || die "$tar: no $base/SOURCES.txt"
	echo "$base/SOURCES.txt" >"$work/x.expected"
	cut -f3,4 "$work/check.packages" | LC_ALL=C sort -u | while IFS="$tab" read -r src ver; do
		dsc="$src/${src}_${ver#*:}.dsc"
		[ -f "$work/x/$base/$dsc" ] || die "$tar has no source of $src $ver ($dsc), whose packages the AppImage carries files of: $(awk -F'\t' -v s="$src" '$3 == s { printf " %s", $1 }' "$work/check.packages")"
		echo "$base/$dsc" >>"$work/x.expected"
		awk '/^Checksums-Sha256:/ { s = 1; next } s && /^ / { print $1 "  " $3; next } { s = 0 }' "$work/x/$base/$dsc" >"$work/x.sums"
		[ -s "$work/x.sums" ] || die "$dsc lists no files"
		(cd "$work/x/$base/$src" && sha256sum --check --strict --quiet) <"$work/x.sums" || die "$tar: the files of $src are not the ones $dsc lists"
		sed "s|^[0-9a-f]*  |$base/$src/|" "$work/x.sums" >>"$work/x.expected"
	done
	(cd "$work/x" && find . -type f | sed 's|^\./||' | LC_ALL=C sort) >"$work/x.present"
	LC_ALL=C sort "$work/x.expected" | diff - "$work/x.present" >&2 ||
		die "$tar holds files no carried package's source lists (>) or lacks some (<)"
}

usage() {
	die "usage: sources.sh copyrights <AppDir> | archive <AppImage> <archive.tar> | check <AppImage> <archive.tar>"
}

# Not in an && list, which would turn off set -e in the functions.
case "${1:-}" in
copyrights)
	[ $# -eq 2 ] || usage
	copyrights "$2"
	;;
archive)
	[ $# -eq 3 ] || usage
	archive "$2" "$3"
	;;
check)
	[ $# -eq 3 ] || usage
	unpack "$2"
	verify "$root" "$3"
	;;
*) usage ;;
esac
