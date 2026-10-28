#!/usr/bin/env bash
# Gathers what `giode build` left in dist/<platform> into out/ under
# release names that say the system and CPU:
#   macOS    GoPlay-<version>-macos-<arch>.dmg
#   Windows  GoPlay-<version>-windows-<arch>-setup.exe, and the program
#            with its DLLs as GoPlay-<version>-windows-<arch>.zip
#   Linux    the .deb and .rpm as built, and the program with its files as
#            GoPlay-<version>-linux-<arch>.tar.gz
# The version is the release tag's ($GIODE_VERSION), or giode.toml's with
# -dev for a build without one.
# Usage: scripts/ci-collect.sh <macos|windows|linux> <arch>
set -euo pipefail
cd "$(dirname "$0")/.."
platform="$1" arch="$2"
if [ -n "${GIODE_VERSION:-}" ]; then
	version="${GIODE_VERSION#v}"
else
	version="$(sed -n 's/^version = "\(.*\)"/\1/p' giode.toml)-dev"
fi
name="GoPlay-$version-$platform-$arch"
rm -rf out
mkdir -p out
case "$platform" in
macos)
	cp dist/macos/*.dmg "out/$name.dmg"
	;;
windows)
	cp dist/windows/*-setup.exe "out/$name-setup.exe"
	mkdir -p "out/$name"
	cp dist/windows/*.exe dist/windows/*.dll "out/$name/"
	rm -f "out/$name/"*-setup.exe
	(cd out && zip -qr "$name.zip" "$name" && rm -rf "$name")
	;;
linux)
	cp dist/linux/*.deb dist/linux/*.rpm out/
	mkdir -p "out/$name"
	find dist/linux -mindepth 1 -maxdepth 1 ! -name '*.deb' ! -name '*.rpm' -exec cp -r {} "out/$name/" \;
	tar -C out -czf "out/$name.tar.gz" "$name"
	rm -rf "out/$name"
	;;
esac
ls -l out
