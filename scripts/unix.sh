#!/usr/bin/env bash
# goplay macOS/Linux toolchain detection, build and run.
#
# Called by build.sh and run.sh; not meant to be run directly. It finds Go,
# a C compiler, pkg-config and libmpv (plus the X11/OpenGL headers GLFW
# needs on Linux), stops with an error naming whatever is missing, then
# builds goplay.
#
# Detection order: an environment variable override (CC, PKG_CONFIG,
# LIBMPV - the install prefix of a custom libmpv build), then PATH and the
# system pkg-config search path, then a search of the usual install
# locations (Homebrew, MacPorts, /usr/local, ~/.local, ./deps).
#
# Written for bash 3.2, the version macOS ships.

set -u

mode="${1:-}"
shift || true
case "$mode" in
build | run) ;;
*)
	echo "usage: $0 build|run [player args...]" >&2
	exit 2
	;;
esac

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
os="$(uname -s)"
problems=()

if [ -t 1 ]; then
	c_step=$'\033[36m' c_err=$'\033[31m' c_warn=$'\033[33m' c_ok=$'\033[32m' c_off=$'\033[0m'
else
	c_step='' c_err='' c_warn='' c_ok='' c_off=''
fi

step() { printf '%s==> %s%s\n' "$c_step" "$1" "$c_off"; }
found() { printf '    %-11s %s\n' "$1" "$2"; }
warn() { printf '%s    warning: %s%s\n' "$c_warn" "$1" "$c_off"; }
fail() { problems+=("  - $1"$'\n'"      $2"); }

# install_hint prints the package-manager command that provides the named
# dependency on this system.
distro=""
if [ "$os" = Linux ] && [ -r /etc/os-release ]; then
	distro="$(. /etc/os-release && echo "${ID:-} ${ID_LIKE:-}")"
fi
install_hint() {
	local what="$1"
	if [ "$os" = Darwin ]; then
		case "$what" in
		go) echo "brew install go   (or download it from https://go.dev/dl/)" ;;
		cc) echo "xcode-select --install" ;;
		pkg-config) echo "brew install pkg-config" ;;
		mpv) echo "brew install mpv   (or set LIBMPV to the install prefix of your libmpv build)" ;;
		esac
		return
	fi
	case "$distro" in
	*debian* | *ubuntu*)
		case "$what" in
		go) echo "sudo apt install golang-go   (Go 1.26+; or https://go.dev/dl/)" ;;
		cc) echo "sudo apt install build-essential" ;;
		pkg-config) echo "sudo apt install pkg-config" ;;
		mpv) echo "sudo apt install libmpv-dev" ;;
		glfw) echo "sudo apt install libx11-dev libxrandr-dev libxxf86vm-dev libxi-dev libxcursor-dev libxinerama-dev libgl1-mesa-dev" ;;
		zenity) echo "sudo apt install zenity" ;;
		esac
		;;
	*fedora* | *rhel* | *centos*)
		case "$what" in
		go) echo "sudo dnf install golang   (Go 1.26+; or https://go.dev/dl/)" ;;
		cc) echo "sudo dnf install gcc" ;;
		pkg-config) echo "sudo dnf install pkgconf-pkg-config" ;;
		mpv) echo "sudo dnf install mpv-devel   (from RPM Fusion)" ;;
		glfw) echo "sudo dnf install libX11-devel libXrandr-devel libXxf86vm-devel libXi-devel libXcursor-devel libXinerama-devel mesa-libGL-devel" ;;
		zenity) echo "sudo dnf install zenity" ;;
		esac
		;;
	*arch*)
		case "$what" in
		go) echo "sudo pacman -S go" ;;
		cc) echo "sudo pacman -S base-devel" ;;
		pkg-config) echo "sudo pacman -S pkgconf" ;;
		mpv) echo "sudo pacman -S mpv" ;;
		glfw) echo "sudo pacman -S libx11 libxrandr libxxf86vm libxi libxcursor libxinerama mesa" ;;
		zenity) echo "sudo pacman -S zenity" ;;
		esac
		;;
	*)
		case "$what" in
		go) echo "install Go 1.26+ from https://go.dev/dl/" ;;
		cc) echo "install gcc or clang from your distribution" ;;
		pkg-config) echo "install pkg-config (or pkgconf) from your distribution" ;;
		mpv) echo "install the libmpv development package, or set LIBMPV to its install prefix" ;;
		glfw) echo "install the X11 (Xrandr, Xxf86vm, Xi, Xcursor, Xinerama) and OpenGL development packages" ;;
		zenity) echo "install zenity from your distribution" ;;
		esac
		;;
	esac
}

# --- Go -----------------------------------------------------------------
step "Looking for Go"
if ! command -v go >/dev/null 2>&1; then
	for d in /usr/local/go/bin /opt/homebrew/bin /usr/local/bin "$HOME/go/bin" "$HOME"/sdk/go*/bin /usr/lib/go/bin; do
		if [ -x "$d/go" ]; then
			PATH="$d:$PATH"
			break
		fi
	done
fi
if command -v go >/dev/null 2>&1; then
	found go "$(command -v go)"
else
	fail "Go toolchain not found" "$(install_hint go)"
fi

# --- C compiler ---------------------------------------------------------
step "Looking for a C compiler"
cc=""
if [ -n "${CC:-}" ]; then
	if command -v "$CC" >/dev/null 2>&1; then
		cc="$(command -v "$CC")"
	else
		fail "CC is set to '$CC' but that compiler was not found" "Unset CC to search automatically, or point it at gcc/clang."
	fi
else
	for c in cc clang gcc; do
		if command -v "$c" >/dev/null 2>&1; then
			cc="$(command -v "$c")"
			break
		fi
	done
	[ -n "$cc" ] || fail "no C compiler (cc, clang or gcc) found; cgo needs one" "$(install_hint cc)"
fi
if [ -n "$cc" ] && [ "$os" = Darwin ] && ! xcode-select -p >/dev/null 2>&1; then
	fail "Xcode command line tools are not installed (needed for the macOS SDK headers)" "$(install_hint cc)"
	cc=""
fi
[ -n "$cc" ] && found cc "$cc"

# --- pkg-config ---------------------------------------------------------
step "Looking for pkg-config"
pkgconfig=""
for p in "${PKG_CONFIG:-}" pkg-config pkgconf /opt/homebrew/bin/pkg-config /usr/local/bin/pkg-config /opt/local/bin/pkg-config; do
	[ -n "$p" ] || continue
	if command -v "$p" >/dev/null 2>&1; then
		pkgconfig="$(command -v "$p")"
		break
	fi
done
if [ -n "$pkgconfig" ]; then
	found pkg-config "$pkgconfig"
else
	fail "pkg-config not found" "$(install_hint pkg-config)"
fi

# --- libmpv -------------------------------------------------------------
# pkg-config finds a system libmpv by itself; for anything else, search
# the usual prefixes for mpv.pc and put its folder on PKG_CONFIG_PATH.
step "Looking for libmpv"
mpv_pc_dir=""
if [ -n "$pkgconfig" ]; then
	search=()
	if [ -n "${LIBMPV:-}" ]; then
		search=("$LIBMPV" "$LIBMPV/lib/pkgconfig" "$LIBMPV/lib64/pkgconfig" "$LIBMPV/share/pkgconfig" "$LIBMPV"/lib/*/pkgconfig)
	elif ! PKG_CONFIG_PATH="${PKG_CONFIG_PATH:-}" "$pkgconfig" --exists mpv 2>/dev/null; then
		brew_mpv=""
		if command -v brew >/dev/null 2>&1; then
			brew_mpv="$(brew --prefix mpv 2>/dev/null || true)"
		fi
		search=("$root/deps/libmpv/lib/pkgconfig" "$root"/deps/*/lib/pkgconfig
			"${brew_mpv:+$brew_mpv/lib/pkgconfig}" /opt/homebrew/lib/pkgconfig /usr/local/lib/pkgconfig
			/opt/local/lib/pkgconfig "$HOME/.local/lib/pkgconfig" "$HOME/.local/lib64/pkgconfig"
			/usr/local/lib64/pkgconfig /usr/lib/pkgconfig /usr/lib64/pkgconfig /usr/lib/*/pkgconfig
			/usr/share/pkgconfig /opt/homebrew/Cellar/mpv/*/lib/pkgconfig /usr/local/Cellar/mpv/*/lib/pkgconfig)
	fi
	# ${arr[@]+...}: bash 3.2 treats an empty array as unset under set -u.
	for d in ${search[@]+"${search[@]}"}; do
		if [ -n "$d" ] && [ -f "$d/mpv.pc" ]; then
			mpv_pc_dir="$d"
			break
		fi
	done
	# Last resort: a bounded search of the home folder and /opt.
	if [ -z "$mpv_pc_dir" ] && [ "${#search[@]}" -gt 0 ] && [ -z "${LIBMPV:-}" ]; then
		pc="$(find "$root/deps" "$HOME" /opt -maxdepth 6 -name mpv.pc -path '*pkgconfig*' 2>/dev/null | head -n 1)"
		[ -n "$pc" ] && mpv_pc_dir="$(dirname "$pc")"
	fi
	if [ -n "$mpv_pc_dir" ]; then
		export PKG_CONFIG_PATH="$mpv_pc_dir${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
	fi

	if [ -n "${LIBMPV:-}" ] && [ -z "$mpv_pc_dir" ]; then
		fail "LIBMPV is set to '$LIBMPV' but no mpv.pc was found under it" "Point LIBMPV at the install prefix (the folder with include/ and lib/), or unset it to search automatically."
	elif ! "$pkgconfig" --exists mpv 2>/dev/null; then
		fail "libmpv development files not found (no mpv.pc on the pkg-config path)" "$(install_hint mpv)"
	else
		incdir="$("$pkgconfig" --variable=includedir mpv)"
		libdir="$("$pkgconfig" --variable=libdir mpv)"
		if [ -n "$incdir" ] && [ ! -f "$incdir/mpv/client.h" ]; then
			fail "mpv.pc found but mpv/client.h is missing from $incdir" "$(install_hint mpv)"
		else
			found libmpv "$("$pkgconfig" --modversion mpv) (${libdir:-system})"
		fi
	fi
fi

# --- Linux: GLFW build dependencies and dialogs -------------------------
if [ "$os" = Linux ] && [ -n "$pkgconfig" ]; then
	step "Looking for X11/OpenGL development files (GLFW)"
	missing=""
	for m in x11 xrandr xxf86vm xi xcursor xinerama gl; do
		"$pkgconfig" --exists "$m" 2>/dev/null || missing="$missing $m"
	done
	if [ -n "$missing" ]; then
		fail "missing development packages for:$missing" "$(install_hint glfw)"
	else
		found glfw "x11 xrandr xxf86vm xi xcursor xinerama gl"
	fi
	if ! command -v zenity >/dev/null 2>&1 && ! command -v qarma >/dev/null 2>&1 && ! command -v matedialog >/dev/null 2>&1; then
		warn "zenity not found: the Open File/Folder dialogs will not work ($(install_hint zenity))"
	fi
fi

# --- Report -------------------------------------------------------------
if [ "${#problems[@]}" -gt 0 ]; then
	echo
	printf '%sERROR: missing build dependencies:%s\n' "$c_err" "$c_off" >&2
	for p in "${problems[@]}"; do
		printf '%s%s%s\n' "$c_err" "$p" "$c_off" >&2
	done
	exit 1
fi

# --- Build --------------------------------------------------------------
export CGO_ENABLED=1
export CC="$cc"
export PKG_CONFIG="$pkgconfig"

# A libmpv outside the system library path gets an rpath, so the binary
# finds it at runtime without LD_LIBRARY_PATH/DYLD_LIBRARY_PATH.
case "${libdir:-}" in
"" | /usr/lib | /usr/lib64 | /usr/lib/*-linux-gnu | /lib | /lib64) ;;
*) export CGO_LDFLAGS="${CGO_LDFLAGS:+$CGO_LDFLAGS }-Wl,-rpath,$libdir" ;;
esac

step "Building goplay"
cd "$root" || exit 1
version="$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)"
if ! go build -ldflags "-X github.com/mehran9675/goplay/internal/app.Version=$version" -o goplay .; then
	printf '%sERROR: go build failed.%s\n' "$c_err" "$c_off" >&2
	exit 1
fi

if [ "$mode" = build ]; then
	echo
	printf '%sBuild complete: %s/goplay%s\n' "$c_ok" "$root" "$c_off"
	exit 0
fi

step "Running goplay $*"
exec ./goplay "$@"
