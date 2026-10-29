# What GoPlay needs from Giode and the Giode CLI

GoPlay runs on `giode.App` (github.com/mehran9675/giode) and is built with the
Giode CLI (github.com/Mehran9675/giode-cli). Where either is missing something
GoPlay needs, GoPlay doesn't work around it; this file describes what's
missing. Things only a video player would ever need are in GoPlay itself and
listed at the end.

Everything this file has asked for so far has landed, and GoPlay uses it:
file drops everywhere with a drop highlight, window placement and the
screen's size, minimized windows on Windows, `SetAlwaysOnTop`, `SetTitle`,
the underlay's `GetProcAddress` and `UnderlayRelease`, the MCP screenshot
with video and the pointer, key and file-drag tools, menus that close as a
whole when an item is chosen or when anything outside them is clicked, menu
rows whose check marks follow the state (`CheckItem`, `RadioItem`), text
corrected for linear blending and set on the pixel grid, thumb-only, inset
and auto-hiding scroll bars, and in the CLI Windows DLL imports, ANGLE from
MSYS2 or VS Code, rebuilds, `{version}` in ldflags, package names with the
CPU, and fullscreen windows that cover the Windows taskbar.

## Giode

### Submenus near the window's edge cover their parent (`components/menu`)

A submenu opens at its row's right edge (`menuRow.Layout` calls
`OpenAt` at the row's top right), and is then kept inside the window
the same way a context menu is. When there's no room on the right,
it's moved left until it fits: over the parent menu, hiding the
parent's rows. GoPlay's title bar menu opens at the window's right
edge, so every one of its submenus lands on top of it, and moving the
pointer across them opens the next level, which covers both.

**Fix:** when a submenu doesn't fit to the right of its parent, open
it to the left of the parent (its right edge at the parent's left
edge), as Windows and macOS do; move it only when it fits on neither
side. A deeper level should keep the direction of the one before it,
so a cascade going left keeps going left.

## Giode CLI

### macOS: leaving fullscreen doesn't work (the Gio patches)

Reported on macOS: the window couldn't be taken out of fullscreen.
In Gio's `app/os_macos.go`, `Configure` reads the window's style mask,
calls `toggleFullScreen` (which starts macOS's animated transition),
then writes the style mask it read earlier back with
`setWindowStyleMask`. Leaving fullscreen, that mask still has
`NSWindowStyleMaskFullScreen` set, so the window is set back to
fullscreen in the middle of leaving it; entering, the bit is cleared
in the middle of entering. Apple's documentation says the
`NSWindowStyleMaskFullScreen` bit must only be changed by
`toggleFullScreen:`. The same `Configure` also resizes the window
(`setSize`, `setMinSize`) while the transition runs.

Separately, Gio rereads the mode (`updateWindowMode`) only when it
draws, and the window delegate handles `windowWillEnterFullScreen` and
`windowWillExitFullScreen` but not `windowDidEnterFullScreen` and
`windowDidExitFullScreen`. A paused player doesn't draw, so once a
transition ends `Fullscreen()` can report the old mode. GoPlay's
Escape, F, double-click and fullscreen button then ask for the mode
the window is already in, and nothing happens.

**Fix (patch revision 8):** in `Configure`, never write the
`NSWindowStyleMaskFullScreen` bit: reread the mask after toggling and
keep its fullscreen bit. Don't resize the window while it's
fullscreen or changing to or from it. Handle
`windowDidEnterFullScreen` and `windowDidExitFullScreen` by rereading
the mode and sending a `ConfigEvent`.

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
