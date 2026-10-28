#!/usr/bin/env bash
# Sets goplay's version in giode.toml from $TAG (a release tag like
# v1.2.3), for CI. The CLI reads app.version from $GIODE_VERSION too, but
# the version goplay shows comes from app.ldflags, which it doesn't
# rewrite (docs/giode-gaps.md). Without a tag it leaves giode.toml alone.
set -euo pipefail
cd "$(dirname "$0")/.."
[ -n "${TAG:-}" ] || exit 0
version="${TAG#v}"
sed -i.bak \
	-e "s/^version = \".*\"/version = \"$version\"/" \
	-e "s/internal\/app\.Version=[^\"]*\"/internal\/app.Version=$version\"/" \
	giode.toml
rm -f giode.toml.bak
grep -E '^(version|ldflags) =' giode.toml
