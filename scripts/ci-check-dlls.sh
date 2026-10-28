#!/usr/bin/env bash
# Fails when a Windows build in dist/windows imports a DLL that's neither
# beside it nor part of Windows, so CI never ships a package that can't
# start. The giode CLI bundles imported DLLs itself; this guards releases
# against one it couldn't find (it only warns).
set -euo pipefail
cd "$(dirname "$0")/../dist/windows"
missing=0
for f in *.exe *.dll; do
	for dll in $(objdump -p "$f" | sed -n 's/^\s*DLL Name: //p' | tr -d '\r'); do
		lower="$(echo "$dll" | tr 'A-Z' 'a-z')"
		case "$lower" in api-ms-win-* | ext-ms-*) continue ;; esac
		if [ -e "$dll" ] || [ -e "/c/Windows/System32/$dll" ] || ls | grep -qix "$dll"; then
			continue
		fi
		echo "$f needs $dll, which isn't in the package" >&2
		missing=1
	done
done
if [ "$missing" = 1 ]; then
	echo "ERROR: the Windows package is missing DLLs and would not start." >&2
	exit 1
fi
echo "every DLL the package imports is in it or in Windows"
