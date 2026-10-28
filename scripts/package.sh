#!/usr/bin/env bash
# Packages a release build of goplay for this platform into dist/:
#   macOS    goplay-<version>-macos-<arch>.zip      goplay.app with libmpv and its
#                                                   libraries inside
#   Windows  goplay-<version>-windows-x64.zip       goplay.exe with every DLL it
#                                                   needs (run from an MSYS2 UCRT64
#                                                   shell)
#   Linux    goplay-<version>-linux-<arch>.tar.gz   the binary, a desktop entry and
#                                                   the icon; uses the system's
#                                                   libmpv
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
	rm -rf "$app"
	;;

MINGW* | MSYS* | CYGWIN*)
	out="$dist/goplay-$version-windows-x64"
	mkdir -p "$out"
	go build -trimpath -ldflags "-s -w -H=windowsgui $ldflags" -o "$out/goplay.exe" .
	prefix="$(dirname "$(command -v gcc)")"
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
	(cd "$dist" && zip -qr "goplay-$version-windows-x64.zip" "goplay-$version-windows-x64")
	rm -rf "$out"
	;;

Linux)
	arch="$(uname -m)"
	out="$dist/goplay-$version-linux-$arch"
	mkdir -p "$out"
	go build -trimpath -ldflags "-s -w $ldflags" -o "$out/goplay" .
	cp assets/icon.png "$out/goplay.png"
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

To add it to your applications menu:
  install -Dm755 goplay ~/.local/bin/goplay
  install -Dm644 goplay.png ~/.local/share/icons/hicolor/1024x1024/apps/goplay.png
  install -Dm644 goplay.desktop ~/.local/share/applications/goplay.desktop
EOF
	tar -C "$dist" -czf "$dist/goplay-$version-linux-$arch.tar.gz" "goplay-$version-linux-$arch"
	rm -rf "$out"
	;;

*)
	echo "package.sh: unsupported system $(uname -s)" >&2
	exit 1
	;;
esac
ls -l "$dist"
