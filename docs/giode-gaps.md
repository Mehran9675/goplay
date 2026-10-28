# What goplay needs from Giode and the Giode CLI

goplay runs on `giode.App` (github.com/mehran9675/giode) and is built with the
Giode CLI (github.com/Mehran9675/giode-cli). Where either is missing something
goplay needs, goplay doesn't work around it; this file describes what's
missing. Things only a video player would ever need are in goplay itself and
listed at the end.

Everything the previous version of this file asked for has landed, and goplay
uses it: file drops everywhere with a drop highlight, window placement and the
screen's size, minimized windows on Windows, `SetAlwaysOnTop`, `SetTitle`, the
underlay's `GetProcAddress` and `UnderlayRelease`, the MCP screenshot with
video and the pointer, key and file-drag tools, and in the CLI Windows DLL
imports, ANGLE from MSYS2 or VS Code, rebuilds, `{version}` in ldflags and
package names with the CPU.

## Giode

### 1. Menus with submenus (`components/menu`)

All four were reported by a user of goplay's context menu.

- **A click in a submenu leaves the menu open.** `menuRow.Layout` closes
  `r.menu`, which for a submenu item is the submenu, not the menu it came
  from. Choosing "Open → Open File…" or "Audio → Audio Settings…" runs the
  action and leaves the context menu on screen. **Fix:** give a submenu its
  parent and close the root menu (`menuItemActor.Click` needs the same).
- **Submenus stay open.** `openSubmenu` changes only when another row with a
  submenu is hovered, so after "Audio" opens, hovering "Playlist" or "Stop"
  in the parent leaves the Audio submenu up. **Fix:** hovering any other
  row of the parent closes the open submenu (after a short delay, about
  300 ms, so a diagonal move into the submenu doesn't close it), as on
  Windows and macOS.
- **A right-click outside doesn't close the menu.** The scrim takes only
  primary clicks; a secondary click goes through to what's under it, so
  right-clicking elsewhere on goplay's video opens a second context menu at
  the new place instead of just closing the first. **Fix:** a press of any
  button outside the menu closes it and is consumed.
- **A submenu's shadow falls on its parent.** The submenu is drawn above
  the parent panel, so the side of its shadow next to the parent darkens
  the parent's rows. **Fix:** draw a parent panel above its submenus (its
  shadow then falls on the submenu instead, and the submenu's is hidden
  under the parent), or leave the shadow out on the parent's side.

goplay's menu rows now use `CheckItem` and `RadioItem`, so a check mark
moves as soon as a track or setting changes.

### 2. Fullscreen on Windows leaves the taskbar on top

Gio's Windows backend makes a window fullscreen by removing its frame and
showing it maximized (`SW_SHOWMAXIMIZED` in `os_windows.go`). goplay's
fullscreen window has style `0x17000000`: `WS_MAXIMIZE`, no `WS_POPUP`. It
covers the whole monitor (0,0 2560×1440 on a 2560×1440 screen) and is the
foreground window, yet Windows treats it as maximized and draws the taskbar
over the bottom of the video.

**Fix (the CLI's Gio patches):** for `Fullscreen`, set `WS_POPUP` (without
`WS_MAXIMIZE`), `SetWindowPos` to the monitor's full rectangle with
`HWND_TOP`, and `SW_SHOWNORMAL`; restore the saved style and placement when
leaving it.

### 3. Text looks rough at UI sizes

UI text (11–15 dp, Segoe UI on Windows) is drawn with grayscale coverage
only, without hinting, so at 100% scaling it looks soft and uneven next to
the system's ClearType text. A user asked for "edge smoothing". Possible
improvements, best first: gamma-correct coverage for text (light text on
dark backgrounds looks thin and ragged without it), hinting or snapping
stems to pixels at small sizes, and subpixel (LCD) antialiasing on Windows.

### 4. Scroll bars

goplay's lists and settings pages scroll with Giode's scroll bar, styled
through `ScrollBar`. Users found it too visible and too close to the content:

- `TrackColor` with zero alpha falls back to the default track, so a bar
  can't be thumb-only; goplay uses an almost invisible track.
- There's no inset: the bar sits on the viewport's right edge. goplay pads
  its rows to keep them clear of it, but the bar itself can't be moved in
  from the edge. A `ScrollBar.Inset` (dp from the edge, and from top and
  bottom) would do it.
- An overlay style that shows the bar while scrolling or hovered and fades
  it out after (as macOS, Windows 11 and browsers do) would suit a media
  player best.

## Giode CLI

Nothing outstanding.

## In goplay itself (player-specific)

These are video-player features, so they're written for goplay, not
proposed for Giode:

- the timeline (`internal/app/view_seek.go`): buffered ranges, chapter marks,
  A-B loop, a hover preview of time and chapter, keyframe seeks while dragging
- 3D and 360° projection, casting (Chromecast, DLNA), the mpv binding
- browsing DLNA/UPnP media servers (`internal/cast/mediaserver.go`)
- its release workflow: `giode ci` builds on tag pushes and adds web and
  Android, while goplay builds when a release is published, runs its tests
  first, and takes libmpv for Windows from shinchiro's builds

Not Giode or the CLI, but worth knowing: Gio doesn't report media keys
(play/pause on keyboards and headsets), so goplay can't react to them yet.
