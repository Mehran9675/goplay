#!/usr/bin/env bash
# Builds goplay for macOS or Linux with the giode CLI, into dist/.
# The toolchain is detected automatically; see scripts/unix.sh for the
# search order and the CC / PKG_CONFIG / LIBMPV overrides.
exec bash "$(dirname "$0")/scripts/unix.sh" build "$@"
