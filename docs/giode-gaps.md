# What GoPlay needs from Giode and the Giode CLI

GoPlay runs on `giode.App` (github.com/mehran9675/giode) and is built with the
Giode CLI (github.com/Mehran9675/giode-cli). Where either is missing something
GoPlay needs, GoPlay doesn't work around it; this file describes what's
missing. Things only a video player would ever need are in GoPlay itself and
listed at the end.

What this file asked for before has landed, and GoPlay uses it: file drops
everywhere with a drop highlight, window placement and the screen's size,
minimized windows on Windows, `SetAlwaysOnTop`, `SetTitle`, the underlay's
`GetProcAddress` and `UnderlayRelease`, the MCP screenshot with video and
the pointer, key and file-drag tools, menus that close as a whole when an
item is chosen or when anything outside them is clicked, menu rows whose
check marks follow the state (`CheckItem`, `RadioItem`), submenus that open
to the left of their parent when the right has no room, text corrected for
linear blending and set on the pixel grid, thumb-only, inset and auto-hiding
scroll bars, and in the CLI Windows DLL imports, ANGLE from MSYS2 or VS
Code, rebuilds, `{version}` in ldflags, package names with the CPU,
fullscreen windows that cover the Windows taskbar, and macOS windows that
can leave fullscreen.

## Giode

Nothing outstanding.

## Giode CLI

### macOS: the app sometimes keeps running after Quit (unknown cause)

Reported on macOS while playing from a DLNA server: Quit closed the
window, but the video's sound went on, so the process was still
running. On Windows the same steps (a DLNA stream, fullscreen, then
Ctrl+Q) end the process at once. GoPlay's Quit calls `App.Close`, and
its `OnShutdown` only saves state, so the process is waiting either
for the run loop to see the window close or in `UnderlayRelease`
(`mpv_render_context_free`). GoPlay now pauses before closing, so
nothing is heard, but a process that doesn't end keeps the
single-instance lock, and the next launch hands its file to it.

**Needed:** a sample of the stuck process (`sample GoPlay 5 -file
goplay-sample.txt` in Terminal while it runs) to see where it waits.
If it's in the run loop, `App.Close` should end the app on macOS even
when the window is fullscreen.

## In GoPlay itself (player-specific)

These are video-player features, so they're written for GoPlay, not
proposed for Giode:

- the timeline (`internal/app/view_seek.go`): buffered ranges, chapter marks,
  A-B loop, a hover preview of time and chapter, keyframe seeks while dragging
- 3D and 360° projection, casting (Chromecast, DLNA), the mpv binding
- browsing DLNA/UPnP media servers (`internal/cast/mediaserver.go`)
- its release workflow: `giode ci` builds on tag pushes and adds web and
  Android, while GoPlay builds when a release is published, runs its tests
  first, and takes libmpv for Windows from shinchiro's builds

Not Giode or the CLI, but worth knowing: Gio doesn't report media keys
(play/pause on keyboards and headsets), so GoPlay can't react to them yet.
