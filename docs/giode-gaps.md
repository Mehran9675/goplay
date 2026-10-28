# What goplay needs from Giode and the Giode CLI

goplay runs on `giode.App` (github.com/mehran9675/giode) and is built with the
Giode CLI (github.com/Mehran9675/giode-cli). Where either is missing something
goplay needs, goplay doesn't work around it; this file describes what's
missing. Things only a video player would ever need are in goplay itself and
listed at the end.

Everything the previous version of this file asked for has landed, and goplay
uses it: file drops on every desktop (with a drop highlight from
`App.FileDrag`), `Screen`, `Frame`, `SetPosition` and `MoveTo` (picture in
picture goes to the screen's corner; "resize to the video" stays within the
screen), minimized windows on Windows ("Pause when minimized"),
`SetAlwaysOnTop`, `GPUTarget.GetProcAddress` and `UnderlayRelease` (goplay's
own GL lookup is gone, and mpv's render context is freed on the render
thread), the MCP screenshot with the underlay and `ui_pointer`/`ui_key`, and
in the CLI Windows DLL imports, paths with variables and globs, ANGLE
bundling, `{version}` in ldflags and package names with the CPU.

## Giode CLI

### 1. Rebuilding a Windows app empties its DLLs (bug)

On Windows, `native.Collect` looks for a binary's DLLs in the binary's own
folder first (`find`: `dirs = append([]string{filepath.Dir(from)}, ...)`).
For the app that folder is the output, `dist/windows`, so on a second
`giode build windows` it finds the previous build's `libmpv-2.dll` there
and copies it onto itself: `copyFile` opens the destination with `O_TRUNC`
first, and the DLL ends up 0 bytes. The package is broken, and the build
after that fails with `native libraries: libmpv-2.dll: EOF`. A stale DLL in
`dist` would also win over a newer one in the search folders.

**Fix:** leave the output folder out of the search for the app itself
(looking beside a bundled DLL for its own imports is fine), skip copying a
file onto itself, or empty `dist/<platform>` before building.

**This blocks goplay's Windows packages.** CI builds into a clean folder,
but `giode build windows windows-installer` collects the DLLs into
`dist/windows` once per target, so the installer target finds the `windows`
target's copies and fails the same way.

### 2. VS Code's ANGLE isn't found

`angleDirs` looks in `%LOCALAPPDATA%\Programs\Microsoft VS Code` and
`C:\Program Files\Microsoft VS Code`, but VS Code keeps its DLLs in a folder
named after its build (`Microsoft VS Code\6d62b1d5da\libEGL.dll`), and after
an update there are two of them. So on a machine with VS Code and no MSYS2
angleproject the build stops with "none was found for this CPU".

**Fix:** also look in the install's subfolders (`Microsoft VS Code\*`),
newest first. goplay's README asks Windows builders to install MSYS2's
angleproject, which the CLI does find.

### 3. Warnings about DLLs that are part of Windows

`giode build windows` warns that goplay "needs AVICAP32.dll, WLDAP32.dll,
bcryptprimitives.dll, vulkan-1.dll, not found". The first three are in
System32 on every Windows; `vulkan-1.dll` comes with the graphics driver
(libmpv loads Vulkan only when it's asked to use it). A warning that's
always there hides the ones that matter.

**Fix:** treat a DLL found in `%SystemRoot%\System32` (or in a fuller list
of system DLLs) as part of Windows, and `vulkan-1.dll` as the driver's.

goplay's CI keeps its own check (`scripts/ci-check-dlls.sh`) so a release
never ships a package that can't start, since the CLI only warns.

## Giode

### 1. The title at run time

`Config.Title` is only the first title. goplay shows the playing file's
title and sets it with `App.Window().Option(app.Title(...))`, which works
but goes around giode.App. `App.SetTitle(string)` would match the other
window calls.

### 2. MCP: dragging files over the window

`ui_pointer` and `ui_key` made hover, right-click, dragging and shortcuts
testable. File drags still aren't: a `ui_file_drag` tool (enter, move, leave
and drop with paths, at a node or coordinates) would test drop targets and
their highlight. goplay's drop highlight is untested for that reason.

## In goplay itself (player-specific)

These are video-player features, so they're written for goplay, not
proposed for Giode:

- the timeline (`internal/app/view_seek.go`): buffered ranges, chapter marks,
  A-B loop, a hover preview of time and chapter, keyframe seeks while dragging
- 3D and 360° projection, casting (Chromecast, DLNA), the mpv binding
- its release workflow: `giode ci` builds on tag pushes and adds web and
  Android, while goplay builds when a release is published, runs its tests
  first, and takes libmpv for Windows from shinchiro's builds

Not Giode or the CLI, but worth knowing: Gio doesn't report media keys
(play/pause on keyboards and headsets), so goplay can't react to them yet.
