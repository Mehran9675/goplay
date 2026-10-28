# goplay

Cross-platform video player built on Go + **libmpv** + **GLFW** + **Gio**
(gioui.org).

Video is decoded and presented by libmpv through its render API, which keeps
frames on the GPU end to end (zero-copy) and enables hardware decoding plus
GPU tone mapping on every platform — VideoToolbox on macOS, D3D11VA on Windows,
VAAPI on Linux. The controls are drawn with Giode (github.com/mehran9675/giode),
a component library built on top of Gio, an immediate-mode GUI toolkit for Go.

## Requirements

- Go 1.26+
- a C compiler (`gcc`/`clang`) with CGO enabled
- libmpv development files (headers + import lib + `mpv.pc`, or the system
  `pkg-config` package)
- Linux only: `zenity`, for the native file dialogs

## Building

The build scripts find the toolchain by themselves, check that every
dependency is there, and stop with an error that says what is missing and
how to install it:

| | build | build and run (args go to the player) |
|---|---|---|
| Windows | `build.bat` | `run.bat [mediafile]` |
| macOS / Linux | `./build.sh` | `./run.sh [mediafile]` |

To skip the search, point a variable at a dependency yourself:

| Variable | Platform | Meaning |
|---|---|---|
| `MINGW` | Windows | MinGW-w64 `bin` folder (contains `gcc.exe`) |
| `LIBMPV` | all | libmpv dev package (Windows) or install prefix (macOS/Linux) |
| `ANGLE` | Windows | folder with `libEGL.dll` + `libGLESv2.dll` |
| `CC`, `PKG_CONFIG` | macOS/Linux | C compiler and pkg-config to use |

The search logic lives in `scripts/windows.ps1` and `scripts/unix.sh`.

### Windows

Install Go, MinGW-w64 (e.g. MSYS2: `pacman -S mingw-w64-x86_64-gcc
mingw-w64-x86_64-pkgconf`) and the libmpv dev package (`mpv-dev-x86_64-*.7z`
from shinchiro/mpv-winbuild-cmake, extracted to `%USERPROFILE%\libmpv`).
ANGLE is taken from an installed VS Code, Chrome or Edge.

`build.bat` builds `goplay.exe` with `-H=windowsgui` (no console window);
`run.bat` builds `goplay-debug.exe` with a console so mpv's logs are visible.
Both copy the runtime DLLs next to the exe, since the app folder is searched
before `PATH` (which avoids picking up an incompatible `libwinpthread-1.dll`
from another MinGW install such as Git's):

- `libmpv-2.dll`: the mpv runtime
- `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, `libwinpthread-1.dll`: the MinGW
  C/C++ runtime
- `libEGL.dll`, `libGLESv2.dll`: ANGLE. The app creates an OpenGL ES 3.0
  context through ANGLE; Gio loads `libGLESv2.dll` unconditionally and libmpv
  shares that context.

If `mpv.pc` is missing from the libmpv folder, `build.bat` generates one.

### macOS

```sh
xcode-select --install
brew install go mpv pkg-config
./build.sh
```

Verify: `./run.sh somefile.mp4` opens a window with video and the control bar;
hardware decode is active (`hwdec=auto` selects VideoToolbox). In the console,
mpv will print `Using hardware decoding (videotoolbox)` when it engages.

### Linux

```sh
# Debian/Ubuntu
sudo apt install build-essential pkg-config libmpv-dev zenity \
  libx11-dev libxrandr-dev libxxf86vm-dev libxi-dev libxcursor-dev libxinerama-dev libgl1-mesa-dev
./build.sh
```

The X11/OpenGL headers are for GLFW, which is compiled from source.
`build.sh` prints the matching `dnf` or `pacman` command on Fedora and Arch.

## Usage

```
goplay [mediafile]
```

Or start with no argument and open a file or folder from the right-click
menu (or with `O` / `D`), or drag a file onto the window.

Outside fullscreen the window has its own title bar (drag it to move the
window, double-click it to maximize) and the control bar sits in a strip
below the video. In fullscreen both are hidden; the control bar shows
again while the pointer is over the bottom of the screen.

| Input               | Action                                  |
| ------------------- | --------------------------------------- |
| buttons             | play/pause, stop, next, mute, settings, fullscreen |
| Space               | play / pause (replay at end)            |
| S / N               | stop / next in playlist                 |
| O / D               | open file / open folder                 |
| ← / →               | seek back / forward (5 s by default)    |
| ↑ / ↓               | volume (5% by default)                  |
| F                   | fullscreen                              |
| Esc                 | close the menu or settings, leave fullscreen, else quit |
| click/drag seek bar | scrub                                   |
| speaker button      | mute / restore volume                   |
| right-click         | context menu: open, subtitles, video and audio tracks, aspect ratio, speed, loop, settings, fullscreen, quit |
| drag title bar or video | move the window                     |

Opening a folder queues every media file in it (sorted); playback advances
automatically and the Next button/`N` skips ahead.

## Settings

The settings dialog (gear button, or right-click → Settings…) covers
playback (resume, auto-advance, seek and volume steps, hardware decoding),
video (scaling, debanding, deinterlacing, motion interpolation, HDR tone
mapping, picture adjustments), audio (output device, language, channels,
loudness normalization, pitch correction, delay), subtitles (language,
auto-loading, size, position, color, outline, delay, ASS styling), the
interface, and caching. Changes apply immediately.

Settings are saved to `settings.json` in goplay's config folder
(`%AppData%\goplay` on Windows, `~/Library/Application Support/goplay` on
macOS, `~/.config/goplay` on Linux). Any other mpv option can go in
`mpv.conf` in the same folder (Settings → Advanced → Edit mpv.conf…); it is
read at startup and overrides goplay's own settings.

## How it works

```
 file ──► mpv ──► decoded GPU frames ──► libmpv render API ──► OpenGL framebuffer
              │                              (zero-copy)
              └─► audio (native rate/channels) ──► OS audio device
```

- `vo=libmpv` renders into the app's OpenGL context; the app supplies
  `glfwGetProcAddress` as libmpv's `get_proc_address`, so mpv resolves GL
  entry points against the same GLFW/OpenGL the UI uses.
- `hwdec=auto` enables hardware decode (VideoToolbox / D3D11VA / VAAPI).
  HDR tone mapping and color management run on the GPU via libmpv's
  `gpu-next`/libplacebo path, with no CPU copies.
- On Windows the context is OpenGL ES 3.0 through ANGLE (EGL), which is what
  Gio's backend expects; on other platforms Gio renders through the desktop
  GL context that GLFW creates.
- The window's framebuffer size is `window size × content scale`, so rendering
  is correct on macOS Retina and Windows HiDPI displays.
- The render loop is vsync-limited: poll input → update mpv state → render the
  mpv frame into the framebuffer → lay out the Gio UI on top → swap buffers.
- Playback state (`time-pos`, `duration`, `pause`, `eof-reached`) is polled
  once per frame; playlist advance happens on `eof-reached`.

The window is frameless; drag the empty video area to move it, `F` toggles
fullscreen, and `Esc` closes the context menu (or quits when it is closed).

## Layout

```
main.go               entry point: parses the optional file argument
internal/libmpv/      minimal cgo binding for the libmpv client + render APIs
  libmpv.go           handle, options, properties, commands
  render.go           OpenGL render context (get_proc_address via GLFW)
internal/glfwutil/    GLFW fullscreen + monitor work-area helpers
internal/app/         app controller: mpv lifecycle, playlist, file dialogs, loop
  prefs.go            applying settings, window controls
internal/settings/    settings table (Defs), mpv mapping, JSON store
internal/ui/          Giode chrome: title bar, controls, context menu, settings
  ui.go               UI struct, Layout entry, keyboard handling, message
  controller.go       Controller interface (what the UI drives)
  controls.go         bottom control bar
  icons.go, icons/    embedded SVG icons (Material Design, Apache 2.0)
  slider.go           seek + volume slider sync
  menu.go             right-click context menu
  prefs.go            settings dialog
  stack.go            layout helpers that keep dropdowns anchored
```

## Known limitations

- Rendering requires an OpenGL context (ES 3.0 via ANGLE on Windows); macOS
  and Linux use the desktop GL context GLFW creates.
- Hardware decoding is requested via `hwdec=auto` but not asserted; if a
  codec/driver combination falls back to software decoding, playback still
  works, just with higher CPU usage.
