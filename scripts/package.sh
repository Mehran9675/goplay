#!/usr/bin/env bash
# Packages a release build of goplay for this machine's system and CPU
# into dist/:
#   macOS    goplay-<version>-macos-<arch>.dmg        goplay.app with libmpv and its
#            goplay-<version>-macos-<arch>.zip        libraries inside
#   Windows  goplay-<version>-windows-<arch>-setup.exe  an installer (needs Inno
#            goplay-<version>-windows-<arch>.zip        Setup), and goplay.exe with
#                                                     every DLL it needs; run from
#                                                     an MSYS2 UCRT64 or CLANGARM64
#                                                     shell
#   Linux    goplay_<version>_<arch>.deb              a package depending on the
#            goplay-<version>-linux-<arch>.tar.gz     system's libmpv, and the
#                                                     same files to copy by hand
#
# Each adds "Play with GoPlay" to the file manager's context menu where it
# can: a Finder service on macOS, an Explorer verb from the installer on
# Windows, and Dolphin and Nemo actions on Linux.
#
# Usage: scripts/package.sh <version>
# Run it from the source folder to build (it may be another checkout than
# the script's own, to package an older release with current tooling).
#
# This stands in for `giode build` until the Giode CLI can bundle native
# libraries (docs/giode-gaps.md).
set -euo pipefail

version="${1:?usage: $0 <version>}"
tools="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$PWD"
dist="$root/dist"
rm -rf "$dist"
mkdir -p "$dist"
ldflags="-X github.com/mehran9675/goplay/internal/app.Version=$version"
export CGO_ENABLED=1

case "$(uname -s)" in
Darwin)
	arch="$(uname -m)"
	app="$dist/goplay.app"
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$app/Contents/Frameworks"
	go build -trimpath -ldflags "-s -w $ldflags" -o "$app/Contents/MacOS/goplay" .

	# The icon, in every size macOS asks for.
	iconset="$dist/goplay.iconset"
	mkdir -p "$iconset"
	for s in 16 32 128 256 512; do
		sips -z $s $s assets/icon.png --out "$iconset/icon_${s}x${s}.png" >/dev/null
		sips -z $((s * 2)) $((s * 2)) assets/icon.png --out "$iconset/icon_${s}x${s}@2x.png" >/dev/null
	done
	iconutil -c icns "$iconset" -o "$app/Contents/Resources/goplay.icns"
	rm -rf "$iconset"

	sed "s/@VERSION@/$version/g" "$tools/Info.plist" >"$app/Contents/Info.plist"

	# libmpv and everything it links, with install names pointing inside
	# the bundle.
	# Where to look for libraries it can't find from install names alone
	# (it would otherwise stop and ask).
	search=()
	if command -v brew >/dev/null 2>&1; then
		search=(-s "$(brew --prefix)/lib")
	fi
	dylibbundler -od -b -cd "${search[@]}" \
		-x "$app/Contents/MacOS/goplay" \
		-d "$app/Contents/Frameworks" \
		-p @executable_path/../Frameworks/ >/dev/null
	# Rewritten binaries need a fresh signature to run on Apple silicon.
	codesign --force --deep --sign - "$app"

	ditto -c -k --keepParent "$app" "$dist/goplay-$version-macos-$arch.zip"
	# The disk image: the app beside a link to Applications.
	stage="$dist/dmg"
	mkdir -p "$stage"
	mv "$app" "$stage/"
	ln -s /Applications "$stage/Applications"
	hdiutil create -quiet -volname goplay -srcfolder "$stage" -ov -format UDZO 		"$dist/goplay-$version-macos-$arch.dmg"
	rm -rf "$stage"
	;;

MINGW* | MSYS* | CLANG* | CYGWIN*)
	case "$(go env GOARCH)" in
	arm64) arch=arm64 ;;
	*) arch=x64 ;;
	esac
	name="goplay-$version-windows-$arch"
	out="$dist/$name"
	mkdir -p "$out"
	go build -trimpath -ldflags "-s -w -H=windowsgui $ldflags" -o "$out/goplay.exe" .
	(cd "$tools/.." && go run ./scripts/ico "$root/assets/icon.png" "$out/goplay.ico")
	prefix="$(dirname "$(command -v "${CC:-gcc}")")"
	# ANGLE (libEGL, libGLESv2) is loaded at run time, not linked, so ldd
	# doesn't list it: copy it explicitly, then every DLL any of them
	# needs from the toolchain's folder.
	cp "$prefix/libEGL.dll" "$prefix/libGLESv2.dll" "$out/"
	for f in "$out/goplay.exe" "$out/libEGL.dll" "$out/libGLESv2.dll"; do
		ldd "$f" | awk '{print $3}' | grep -i "^$prefix/" || true
	done | sort -u | while read -r dll; do
		cp -n "$dll" "$out/"
	done
	cp README.md "$out/"
	(cd "$dist" && zip -qr "$name.zip" "$name")
	iscc="${ISCC:-$(command -v iscc || command -v ISCC || true)}"
	for d in "/c/Program Files (x86)/Inno Setup 6" "/c/Program Files/Inno Setup 6" "$LOCALAPPDATA/Programs/Inno Setup 6"; do
		[ -n "$iscc" ] && break
		[ -x "$d/ISCC.exe" ] && iscc="$d/ISCC.exe"
	done
	if [ -n "$iscc" ]; then
		MSYS2_ARG_CONV_EXCL='*' "$iscc" /Q "/DVersion=$version" "/DArch=$arch" 			"/DSource=$(cygpath -w "$out")" "/DOutput=$(cygpath -w "$dist")" 			"$(cygpath -w "$tools/goplay.iss")"
	else
		echo "package.sh: Inno Setup (ISCC.exe) not found; skipping the installer" >&2
	fi
	rm -rf "$out"
	;;

Linux)
	arch="$(uname -m)"
	out="$dist/goplay-$version-linux-$arch"
	mkdir -p "$out"
	go build -trimpath -ldflags "-s -w $ldflags" -o "$out/goplay" .
	cp assets/icon.png "$out/goplay.png"
	cp "$tools/linux/goplay-dolphin.desktop" "$tools/linux/goplay.nemo_action" "$out/"
	cat >"$out/goplay.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=goplay
Comment=Play videos, streams and music
Exec=goplay %U
Icon=goplay
Terminal=false
Categories=AudioVideo;Video;Player;
MimeType=video/mp4;video/x-matroska;video/webm;video/quicktime;video/x-msvideo;video/mpeg;video/mp2t;video/x-flv;video/x-ms-wmv;video/ogg;audio/mpeg;audio/flac;audio/ogg;audio/mp4;audio/x-wav;x-scheme-handler/http;x-scheme-handler/https;
EOF
	cat >"$out/INSTALL.txt" <<'EOF'
goplay needs libmpv from your distribution:
  Debian, Ubuntu:  sudo apt install libmpv2      (libmpv1 on older releases)
  Fedora:          sudo dnf install mpv-libs     (RPM Fusion)
  Arch:            sudo pacman -S mpv
Optional: zenity (file dialogs), yt-dlp (web videos), ffmpeg (casting).

On Debian and Ubuntu, the .deb does all of this for you. Otherwise, to
add it to your applications menu:
  install -Dm755 goplay ~/.local/bin/goplay
  install -Dm644 goplay.png ~/.local/share/icons/hicolor/1024x1024/apps/goplay.png
  install -Dm644 goplay.desktop ~/.local/share/applications/goplay.desktop
and "Play with GoPlay" to the context menu of Dolphin (KDE) and Nemo
(Cinnamon):
  install -Dm755 goplay-dolphin.desktop ~/.local/share/kio/servicemenus/goplay.desktop
  install -Dm644 goplay.nemo_action ~/.local/share/nemo/actions/goplay.nemo_action
EOF
	tar -C "$dist" -czf "$dist/goplay-$version-linux-$arch.tar.gz" "goplay-$version-linux-$arch"

	# The Debian package, depending on the libraries the binary links.
	debarch="$(dpkg --print-architecture)"
	# Debian versions start with a digit: v1.2.3 is 1.2.3, a dev build 0~dev.
	debver="${version#v}"
	[[ "$debver" =~ ^[0-9] ]] || debver="0~$debver"
	pkg="$dist/deb"
	install -Dm755 "$out/goplay" "$pkg/usr/bin/goplay"
	install -Dm644 "$out/goplay.desktop" "$pkg/usr/share/applications/goplay.desktop"
	install -Dm644 "$out/goplay.png" "$pkg/usr/share/icons/hicolor/1024x1024/apps/goplay.png"
	install -Dm755 "$out/goplay-dolphin.desktop" "$pkg/usr/share/kio/servicemenus/goplay.desktop"
	install -Dm644 "$out/goplay-dolphin.desktop" "$pkg/usr/share/kservices5/ServiceMenus/goplay.desktop"
	install -Dm644 "$out/goplay.nemo_action" "$pkg/usr/share/nemo/actions/goplay.nemo_action"
	mkdir -p "$pkg/debian" "$pkg/DEBIAN"
	echo "Source: goplay" >"$pkg/debian/control"
	depends="$(cd "$pkg" && dpkg-shlibdeps -O usr/bin/goplay | sed 's/^shlibs:Depends=//')"
	rm -rf "$pkg/debian"
	cat >"$pkg/DEBIAN/control" <<EOF
Package: goplay
Version: $debver
Architecture: $debarch
Maintainer: Mehran <61628815+Mehran9675@users.noreply.github.com>
Installed-Size: $(du -sk "$pkg/usr" | cut -f1)
Depends: $depends
Recommends: yt-dlp, ffmpeg, zenity
Section: video
Priority: optional
Homepage: https://github.com/Mehran9675/goplay
Description: video player for files, streams, 3D and casting
 Plays nearly any video or audio file or stream with mpv, including
 stereoscopic 3D and 360-degree video, and casts to Chromecast and DLNA
 devices.
EOF
	dpkg-deb --root-owner-group --build "$pkg" "$dist/goplay_${debver}_$debarch.deb" >/dev/null
	rm -rf "$pkg" "$out"
	;;

*)
	echo "package.sh: unsupported system $(uname -s)" >&2
	exit 1
	;;
esac
ls -l "$dist"
