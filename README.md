# goplay

A video player for people who just want to watch something, with depth for
those who want to tune everything. It plays almost any file or stream with
mpv's engine, in an interface built with [Giode](https://github.com/mehran9675/giode).

Made for macOS first, and built for Windows and Linux too.

## What it does

**Watching**
- Plays nearly every video and audio format, local or from a link: YouTube,
  Vimeo, Twitch and the thousands of other sites yt-dlp knows, and HLS/DASH
  streams.
- Hardware decoding on every platform (VideoToolbox, D3D11, VA-API), drawn
  straight from the GPU without copies.
- Resumes where you left off. Opening one episode queues the rest of the
  folder, in natural order (2 before 10).
- Recent files on the start screen, with how far you got.
- Drag and drop files, folders and subtitles; paste a link or path.

**Controls**
- In a window, the title bar and the controls sit above and below the
  picture, never over it. Maximized or fullscreen, they float over the video
  and appear when you move to the top or bottom edge; the pointer hides.
- Timeline with chapters, buffered ranges and an A–B loop, and a preview of
  the time and chapter under the pointer.
- Speed from 0.25× to 4× with pitch kept, frame stepping, A–B repeat, file
  and playlist repeat, shuffle, go to a time.
- Playlist and chapters panel; picture-in-picture; always on top.
- Click to pause, double-click for fullscreen, wheel for volume (or seeking).
- A command palette (Ctrl+K, ⌘K on macOS) for every command, and every
  shortcut can be changed.

**Picture and sound**
- Video, audio and subtitle tracks, a second subtitle line, external
  subtitle and audio files.
- Subtitle size, position, color, outline, background box, delay and text
  encoding; preferred audio and subtitle languages.
- Fit, fill or stretch; aspect ratio, zoom, rotation, mirroring;
  deinterlacing, debanding, smooth motion, HDR tone mapping, color
  adjustments, display color profile.
- Equalizer presets, loudness normalization, night mode (dynamic range
  compression), audio delay, output device, exclusive mode, surround
  passthrough.

**3D and VR**
- Stereoscopic 3D in side-by-side and top-and-bottom layouts (half or full,
  either eye first), shown as red/cyan, green/magenta or yellow/blue
  anaglyph, one eye in 2D, interleaved rows or columns for passive 3D
  monitors, checkerboard for 3D projectors, or side-by-side and
  top-and-bottom for 3D TVs.
- 360° and VR180 video (equirectangular, 180°, fisheye, YouTube's cubemap;
  mono or stereo): drag to look around, scroll to zoom.
- Recognizes 3D and 360° files by name ("3D.HSBS", "TAB", "360", "VR180").

**Casting**
- Chromecast, Google TV and Android TV, and DLNA/UPnP renderers (most smart
  TVs, consoles and receivers), found automatically on the network.
- Local files are served to the TV from goplay; files the TV can't play are
  repackaged (or converted) on the fly with FFmpeg. Subtitles are sent along.
- The controls drive the TV while casting; stopping picks up here where the
  TV left off.

**For power users**
- Every mpv option: in Settings, as "Extra mpv options", or in `mpv.conf`.
  `input.conf` bindings and mpv scripts (Lua or JavaScript) work too; keys
  goplay doesn't use reach them.
- `goplay --run=fullscreen,info movie.mkv` runs commands at startup.
- Media information panel (codec, resolution, color, bitrate, decoder,
  dropped frames) and the mpv log.

Settings, `mpv.conf`, `input.conf`, `scripts/` and resume data live in
goplay's folder: `~/Library/Application Support/goplay` on macOS,
`%AppData%\goplay` on Windows, `~/.config/goplay` on Linux (Settings ›
Advanced › Open Config Folder).

## Building

You need Go 1.26+, a C compiler, pkg-config and libmpv's development files.
`build.sh` and `build.bat` find the toolchain themselves and say exactly
what's missing and how to install it.

| | build | build and run |
|---|---|---|
| macOS, Linux | `./build.sh` | `./run.sh [file]` |
| Windows | `build.bat` | `run.bat [file]` |

### macOS

```sh
xcode-select --install
brew install go mpv pkg-config
./build.sh
```

### Linux

```sh
# Debian and Ubuntu (build.sh prints the dnf and pacman equivalents)
sudo apt install build-essential pkg-config libmpv-dev zenity \
  libx11-dev libxrandr-dev libxxf86vm-dev libxi-dev libxcursor-dev libxinerama-dev libgl1-mesa-dev
./build.sh
```

### Windows

Install Go, MinGW-w64 (MSYS2: `pacman -S mingw-w64-x86_64-gcc
mingw-w64-x86_64-pkgconf`) and the libmpv development package
(`mpv-dev-x86_64-*.7z` from shinchiro/mpv-winbuild-cmake, extracted to
`%USERPROFILE%\libmpv`). ANGLE comes from an installed VS Code, Chrome or Edge.
`build.bat` makes `goplay.exe` and copies the DLLs it needs next to it;
`run.bat` makes `goplay-debug.exe`, which shows mpv's log in the console.

To skip the search, set `LIBMPV` (all platforms), `MINGW` and `ANGLE`
(Windows), or `CC` and `PKG_CONFIG` (macOS and Linux).

Optional at runtime: **yt-dlp** for web pages, **FFmpeg** for casting files
TVs can't play, and on Linux **zenity** for file dialogs.

Publishing a release builds and tests it on macOS, Linux and Windows and
attaches the packages to it (`.github/workflows/build.yml`,
`scripts/package.sh`); the macOS app carries libmpv inside, the Windows zip
has every DLL next to `goplay.exe`, and Linux uses the system's libmpv. The
project is set up for the Giode CLI (`giode.toml`), which can't yet bundle
libmpv into app packages; see `docs/giode-gaps.md`.

## How it works

```
main.go                 command line, window icon
internal/mpv/           cgo binding: client API (commands, observed properties,
                        events, wakeups) and the OpenGL render API
internal/player/        the mpv core goplay drives: state, commands, filters,
                        3D and 360° projection, track names
internal/platform/      the GLFW window: GL context per OS, input into Gio's
                        router, clipboard, cursor, maximize/fullscreen on the
                        right monitor, dragging and edge resizing, keep-awake,
                        macOS window style and Finder open events
internal/app/           the player itself: event loop and rendering, commands,
                        opening media, 3D/VR, casting, and the views
internal/actions/       the command registry: titles, shortcuts, the palette
internal/settings/      the settings table, its mpv mapping and the JSON store
internal/library/       recent files, folder scanning, natural sorting
internal/cast/          Chromecast (mDNS, Cast v2) and DLNA (SSDP, UPnP
                        AVTransport), and the HTTP server feeding them
```

**Rendering.** Video stays on the GPU: libmpv renders into the window's
OpenGL framebuffer, then Gio draws the Giode interface over it in the same
frame. On Windows the context is OpenGL ES through ANGLE, so hardware-decoded
D3D11 frames are shared without copies; on macOS it's OpenGL, and on Linux
OpenGL ES through EGL.

**Efficiency.** Nothing runs unless something changed. The loop sleeps until
mpv has an event or a new frame (its wakeup and render callbacks), the user
does something, or an animation needs its next frame. Playback state comes
from mpv's observed properties, not polling. A paused or idle player uses no
CPU, and during playback frames are drawn at the video's rate. Playing a
1080p H.264 file on Windows uses about 1.7% of one CPU core, against 8% for
the previous version, which redrew continuously. Memory is about the same
(roughly 120 MB private while playing, 65 MB idle), nearly all of it mpv,
FFmpeg and the graphics driver.

**Giode without `giode.App`.** Giode's `App` can't put video under its UI
yet (see `docs/giode-gaps.md`), so goplay hosts Giode's components in its own
GLFW window, as Giode's docs describe for embedded windows. It imports the
component packages directly, so Gio's own windowing layer isn't linked.

## Testing

```sh
scripts/test-media.sh /tmp/media      # small test videos, made with FFmpeg
GOPLAY_TEST_MEDIA=/tmp/media/chapters.mkv GOPLAY_TEST_MEDIA_DIR=/tmp/media go test ./...
```

Without those variables, the tests that need real media and libmpv are
skipped.
