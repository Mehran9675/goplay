# What goplay needs from Giode and the Giode CLI

goplay is built on Giode (github.com/mehran9675/giode) and is set up for the
Giode CLI (github.com/Mehran9675/giode-cli). Where either was missing something
goplay needed, goplay doesn't work around it; this file describes what's
missing instead. Things only a video player would ever need are in goplay
itself and listed at the end.

## Giode

### 1. Drawing GPU content under the UI in `giode.App` (the main one)

A video player needs each video frame drawn into the window, with the UI on
top, without copying frames off the GPU. libmpv (like most video, 3D and map
engines) renders with OpenGL into a framebuffer the host provides.

`giode.App` owns the window and its GPU context, and there's no way to draw
anything into the frame before Giode's UI. On macOS Gio renders with Metal, so
an OpenGL engine can't even share the context.

**What goplay does instead:** it uses Giode the documented "GLFW and embedded
windows" way. It creates a GLFW window with an OpenGL (ES) context, lets libmpv
draw the frame, then has Gio's `gpu` package draw the Giode UI over it.

**What's needed:** an `App` option for a caller-drawn layer under the UI, for
example:

```go
giode.Config{
	// Ask for an OpenGL (ES) context on every platform (Metal is the
	// default on macOS) and call this before each UI frame, with the
	// context current.
	Underlay: func(fb giode.GLFramebuffer) { mpvRender(fb.ID, fb.Width, fb.Height) },
}
app.Invalidate() // from any goroutine: a new video frame is ready
```

Metal and Direct3D variants (a texture handle to draw into) would let engines
keep their native APIs. With this, goplay could drop its GLFW window and use
`App` directly.

**What a custom host loses today.** These are `App` methods, so a GLFW-hosted
app can't use them and has to provide its own:

| `App` feature | goplay's replacement |
| --- | --- |
| `Post`, `RunOnUI` | its own queue, woken with `glfw.PostEmptyEvent` |
| `SetClipboard`, `ReadClipboard` | GLFW's clipboard, wired to Gio's router |
| `Config.OnFileDrop`, `NativeHandle` | GLFW's drop callback and native accessors |
| `KeepScreenOn` | its own per-OS code (Windows, macOS, Linux) |
| `TitleBar` window state, `Frameless`, resize borders | its own maximize, fullscreen, drag and edge resizing |
| single instance, context actions, tray, system theme, notifications, MCP | not available |

Package-level versions of `KeepScreenOn`, the clipboard and single-instance
(taking a native window handle where needed) would help any custom host.

### 2. One function for a custom host's frame

`docs/low-level.md` shows a custom render loop, but the example leaves out
steps components depend on: `elements.BeginFrame`, `SetMetric`,
`DrainPortals`, `DispatchBack`, `RunAfterLayout`, `EndFrame`, and
`state.SetInvalidator` so a `State` or toast set from another goroutine can
wake the loop. Leaving any out makes some component quietly misbehave
(portals, keyed state, toasts). A single exported helper would keep custom
hosts in step with `App`:

```go
// Frame lays out one Giode frame into gtx the way App does.
func Frame(gtx layout.Context, view Element) (needsAnotherFrame bool)
```

### 3. The root package pulls in Gio's window layer

The convenient names (`giode.Box`, `giode.Button`, `giode.Styles`, …) live in
the root package, which also holds `App` and so imports `gioui.org/app`. A
GLFW-hosted app that uses those names links Gio's windowing code it never
uses. That means extra Linux build dependencies, a bigger binary, and on macOS
Gio's app package starting up alongside GLFW's `NSApplication`. goplay imports
the component packages directly instead.

**Suggested:** move `App` to a subpackage (`giode/app`), or add a re-export
package without it (`giode/ui`), so custom hosts can use the short names.

### 4. Smaller component gaps

- **TitleBar has no `Styles(st)` setter.** Button, IconButton and Drawer have
  one. goplay's bar is solid in a window and a transparent fade over
  fullscreen video. goplay creates the bar transparent and puts its own
  background behind it.
- **Switch: no knob color.** The knob is white, so a light "on" track (white,
  in a monochrome theme) hides it. A `BorderColor` or `Color` for the knob
  would fix it; goplay uses green for "on".
- **Menu: no shortcut column.** Desktop menus show each item's shortcut
  right-aligned ("Open File… Ctrl+O"). An `ItemWithShortcut(label,
  shortcut, fn)` or `Shortcut(text)` on items.
- **Menu: no check or radio items.** goplay shows the current track or speed
  with a check icon (`IconItem` with a "check" or blank icon). A native
  `CheckItem(label, checked, fn)` and radio groups would be clearer and
  accessible.
- **Toaster: no inset.** Placement is relative to the window edges, so toasts
  can sit over a custom title bar or control bar. A `Margin`/`Inset` setting
  would let apps keep them clear.

## Giode CLI

### 1. Bundling native libraries

goplay links libmpv with cgo. libmpv is a shared library with dozens of its
own (FFmpeg, libass, …). `giode build` builds the binary, but no target
bundles those libraries:

- **macOS `.app`/`.dmg`**: copy the dylibs into `Contents/Frameworks`, rewrite
  their install names and the binary's rpath (what `dylibbundler` does), and
  sign them with the app.
- **Windows (`windows`, `windows-installer`, `msix`)**: put the DLLs next to the
  executable in the installer and package.
- **Linux (`appimage`, `flatpak`, `deb`, `rpm`)**: bundle them in the AppImage or
  Flatpak, or declare them as package dependencies (`libmpv2`) for deb and
  rpm.

A `[native]` section listing libraries (or finding them with
`otool`/`ldd`/`objdump`) would cover any cgo app with shared libraries.

Until then, `scripts/package.sh` does this for goplay's release builds; it
goes away once `giode build` can.

### 2. cgo on Windows

The README's table says Windows builds need "nothing extra: Gio doesn't need
cgo on Windows". A cgo app needs a C compiler and pkg-config for the target:
a `[windows] cc = "x86_64-w64-mingw32-gcc"` (and `pkg_config_path`) setting,
and `giode doctor` checking them.

### 3. File associations

A video player must appear in "Open with" for video files:

- **macOS**: `CFBundleDocumentTypes` (and `UTImportedTypeDeclarations`) in
  Info.plist. `[ios.plist]` takes arbitrary keys; macOS needs the same
  (`[macos.plist]`).
- **Windows installer and MSIX**: registering file types and the "Open with"
  entry.
- **Linux**: `MimeType=` in the `.desktop` file.

A cross-platform `[[file_types]]` list (extensions, MIME types, a name, a role)
would generate all three.

A context menu entry ("Play with GoPlay") fits next to that, as an
`[[actions]]` list with a label, the file types and the command line. It
would make an `NSServices` entry on macOS (the app answers it from its
services provider), a `shell` verb under `SystemFileAssociations` in the
Windows installer, and Dolphin and Nemo action files on Linux.
`scripts/package.sh`, `scripts/goplay.iss` and `scripts/linux/` do this for
goplay until then.

### 4. `giode ci` with system packages

The generated workflow builds pure-Go apps. cgo apps need packages installed
first (Homebrew, apt, MSYS2). A `[ci] setup = { macos = "brew install mpv",
linux = "...", windows = "..." }` hook would let `giode ci` build goplay.
goplay has its own workflow in `.github/workflows/build.yml` until then.

## In goplay itself (player-specific)

These are video-player features, so they're written for goplay, not
proposed for Giode:

- the timeline (`internal/app/view_seek.go`): buffered ranges, chapter marks,
  A-B loop, a hover preview of time and chapter, keyframe seeks while dragging
- the GLFW host (`internal/platform`): needed only because of Giode gap 1
- 3D and 360° projection, casting (Chromecast, DLNA), the mpv binding

Not Giode or the CLI, but worth knowing: GLFW doesn't report media keys
(play/pause on keyboards and headsets), so goplay can't react to them yet.
