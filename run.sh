#!/usr/bin/env bash
# Builds ./goplay and runs it; arguments go to the player, e.g. ./run.sh movie.mkv
# The toolchain is detected automatically; see scripts/unix.sh for the
# search order and the CC / PKG_CONFIG / LIBMPV overrides.
exec bash "$(dirname "$0")/scripts/unix.sh" run "$@"
