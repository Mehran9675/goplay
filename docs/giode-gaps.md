# What goplay needs from Giode and the Giode CLI

goplay runs on `giode.App` (github.com/mehran9675/giode) and is built with the
Giode CLI (github.com/Mehran9675/giode-cli). Where either is missing something
goplay needs, goplay doesn't work around it; this file describes what's
missing. Things only a video player would ever need are in goplay itself and
listed at the end.

Everything the previous version of this file asked for has landed: the
underlay, `ui.Frame` and the `host` package, the menu and tooltip fixes, menu
shortcuts and check items, native library bundling, file types, context
actions, cgo on Windows and CI setup hooks. goplay now uses them, and its own
GLFW host and packaging scripts are gone.

## Giode

### 1. File drop on macOS and Linux

`Config.OnFileDrop` only works on Windows. Dropping videos, folders and
subtitle files onto the player is how many people open them, especially on
macOS, goplay's main platform. With the GLFW host it worked everywhere; now it
works only on Windows.

- **macOS**: register the window's view for `NSPasteboardTypeFileURL`
  (`registerForDraggedTypes:`) and deliver `performDragOperation:`'s URLs.
- **Linux**: Gio's event loop consumes the X11 (XDND) and Wayland
  (`wl_data_device`) drag-and-drop messages, so this needs Gio changes.

Drag enter/leave events, for highlighting a drop target, would help too.

### 2. Window position and the screen's size

Picture in picture shrinks the window and keeps it on top, but can't move it
to a corner of the screen: giode.App can't place a window or report the
screen's work area. "Resize the window to fit each video" can't cap the size
at the screen's either, so goplay uses a fixed 1280×800 dp limit.

**Suggested:** `App.SetPosition(x, y)` (or `MoveTo(corner, margin)`), and
`App.Screen()` returning the current monitor's bounds and work area in dp.

### 3. Minimized windows on Windows

Gio's Windows backend never reports `app.Minimized` in a `ConfigEvent`, so
`OnWindowStateChange` never sees `WindowMinimized` there. `OnBackground`,
documented as running "on desktops when the window is minimized", doesn't run
on desktop at all (only Android and iOS call it). goplay's "Pause when
minimized" uses `OnWindowStateChange` and works only where Gio reports the
state.

**Suggested:** report `Minimized` from `WM_SIZE`/`SIZE_MINIMIZED` (and restore
on `SIZE_RESTORED`) in the patched Gio, and call `OnBackground`/`OnForeground`
from window state on desktops as the doc says.

### 4. Always on top at run time

`Config.AlwaysOnTop` only applies at startup. goplay toggles it with
`App.Window().Option(app.TopMost(on))`, which works but goes around giode.App.
`App.SetAlwaysOnTop(bool)` and `App.AlwaysOnTop()` would match `SetFullscreen`
and keep giode's own state right.

### 5. The underlay's GL context

- **Function lookup.** OpenGL engines in `Config.Underlay` need the context's
  `GetProcAddress` (libmpv's render API asks for one). goplay resolves it
  itself (`internal/mpv/glproc.go`: ANGLE's `eglGetProcAddress` on Windows,
  EGL on Linux, `dlsym` on macOS), which every OpenGL engine would repeat.
  `GPUTarget` could carry a `GetProcAddress func(name string) unsafe.Pointer`.
- **Teardown.** An engine's GL resources must be freed on the render thread
  with the context current, before the window's context goes away
  (`mpv_render_context_free` must also run before the mpv core is
  destroyed). There's no callback for that, so goplay lets the process exit
  with mpv alive. **Suggested:** `Config.UnderlayRelease func()`, called on
  the render thread before the context is destroyed.

### 6. MCP

goplay is tested with giode's MCP server, which found several of the problems
above, but:

- **`screenshot` is blank** (white) with `Underlay` and `RendererOpenGL`. It
  should read the window's framebuffer after the frame, video included.
- **No pointer or keyboard tools** besides clicks: hover (tooltips, a seek
  bar's preview, a control bar that shows on hover), drag (sliders, seek
  bars), right-click (context menus) and key presses (shortcuts) can't be
  tested. Windows messages posted to the window don't reach Gio either,
  since it reads pointer input. A `ui_pointer` tool (move, press and release
  with a button, at a node or at coordinates) and `ui_key` (a key with
  modifiers) would cover them.

## Giode CLI

### 1. Windows DLL imports are never found (bug)

`native.Collect` reads a Windows binary's imports with Go's
`(*pe.File).ImportedLibraries`, which Go's standard library has never
implemented: it always returns nil. So no imported DLL is bundled, and no
missing one is warned about. goplay's exe imports `libmpv-2.dll`, and nothing
bundled it.

**Fix:** collect the DLL names from `(*pe.File).ImportedSymbols()`, whose
entries are `symbol:dll`, or read the import directory. Until then goplay
lists its DLLs in `[native.windows] libraries`, which only works because the
libmpv it uses (shinchiro's build) is a single self-contained DLL. MSYS2's
libmpv, with dozens of dependencies, can't be bundled.

### 2. Paths in giode.toml

`[native.windows] libraries` and `search` take fixed paths. ANGLE lives in a
different place on every machine (an Edge, Chrome or VS Code install, MSYS2's
`angleproject`), so goplay's build scripts copy it to `build/native` first.
Environment variables (`$MSYSTEM_PREFIX/bin/libEGL.dll`) or globs in these
paths would remove that step. With `renderer = "opengl"` the CLI could also
find and bundle ANGLE itself, since `RendererOpenGL` requires it on Windows.

### 3. The version in ldflags

`$GIODE_VERSION` sets `app.version`, but `app.ldflags` can't refer to it, so
an app's own version string (goplay's About box) doesn't follow the release
tag. goplay's CI rewrites both (`scripts/ci-version.sh`). A `{version}`
placeholder in `app.ldflags`, or the CLI setting a variable the app can read
(`-X github.com/mehran9675/giode.AppVersion=...`), would fix it.

### 4. CI for ARM and file names per CPU

`giode ci` builds x64 only, in MSYS2's MINGW64. goplay's releases also have
Linux and Windows ARM64 builds (GitHub's `ubuntu-24.04-arm` and
`windows-11-arm` runners, MSYS2's CLANGARM64), so goplay keeps its own
workflow. Also, `dmg` and `windows-installer` name their output without the
CPU (`goplay-1.1.0.dmg`, `goplay-1.1.0-setup.exe`), so two builds of one
release collide; goplay's CI renames them (`scripts/ci-collect.sh`).
**Suggested:** an `arch` list in `[ci]`, and the CPU in every package's name
(the `.deb`, `.rpm` and AppImage already have it).

## In goplay itself (player-specific)

These are video-player features, so they're written for goplay, not
proposed for Giode:

- the timeline (`internal/app/view_seek.go`): buffered ranges, chapter marks,
  A-B loop, a hover preview of time and chapter, keyframe seeks while dragging
- 3D and 360° projection, casting (Chromecast, DLNA), the mpv binding

Not Giode or the CLI, but worth knowing: Gio doesn't report media keys
(play/pause on keyboards and headsets), so goplay can't react to them yet.
