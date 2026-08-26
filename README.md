# goplay

Cross-platform video player built on Go + **libmpv** + **GLFW** + **OpenGL**
+ **Dear ImGui**.

Video is decoded and presented by libmpv through its render API, which keeps
frames on the GPU end to end (zero-copy) and enables hardware decoding plus
GPU tone mapping on every platform — VideoToolbox on macOS, D3D11VA on Windows,
VAAPI on Linux. The controls are drawn with Dear ImGui.

## Requirements

- Go 1.26+
- a C compiler (`gcc`/`clang`) with CGO enabled
- libmpv development files (headers + import lib + `mpv.pc`, or the system
  `pkg-config` package)
- Linux only: `zenity`, for the native file dialogs

### macOS

```sh
brew install mpv   # provides mpv headers, lib, and pkg-config file
CGO_ENABLED=1 go build -o goplay .
```

Verify: `./goplay somefile.mp4` opens a window with video and the control bar;
hardware decode is active (`hwdec=auto` selects VideoToolbox). In the console,
mpv will print `Using hardware decoding (videotoolbox)` when it engages.

### Windows

Use a MinGW-w64 toolchain plus libmpv dev files, then point `pkg-config` at
the libmpv directory:

```powershell
$env:PKG_CONFIG_PATH = 'C:\path\to\libmpv'
$env:CGO_ENABLED     = '1'
$env:CC              = 'gcc'
go build -o goplay.exe .
```

The DLLs the exe needs at runtime must be next to `goplay.exe` (the app
folder is searched before `PATH`, which avoids picking up an incompatible
`libwinpthread-1.dll` from another MinGW install such as Git's):

```powershell
Copy-Item C:\path\to\mingw64\bin\libgcc_s_seh-1.dll .
Copy-Item C:\path\to\mingw64\bin\libstdc++-6.dll   .
Copy-Item C:\path\to\mingw64\bin\libwinpthread-1.dll .
Copy-Item C:\path\to\libmpv\libmpv-2.dll .
```

(`libmpv-2.dll` is the mpv runtime; the other three are the MinGW C/C++
runtime. Copy them once after building.)

`build.bat` does all of the above in one step: it sets the toolchain
environment, builds with `-ldflags "-H=windowsgui"` (so no console window
appears alongside the app), and copies the DLLs next to `goplay.exe`.

### Linux

```sh
sudo apt install libmpv-dev zenity   # Debian/Ubuntu
CGO_ENABLED=1 go build -o goplay .
```

## Usage

```
goplay [mediafile]
```

Or start with no argument and use the Open buttons / drag a file onto the
window.

| Input               | Action                                  |
| ------------------- | --------------------------------------- |
| buttons             | open file, open folder, play/pause, stop, next, fullscreen |
| Space               | play / pause (replay at end)            |
| S / N               | stop / next in playlist                 |
| O / D               | open file / open folder                 |
| ← / →               | seek −/+ 5 s                            |
| ↑ / ↓               | volume                                  |
| F                   | fullscreen                              |
| Esc                 | quit                                    |
| click/drag seek bar | scrub                                   |
| right-click         | context menu: open file / open folder   |
| drag empty area     | move the window                        |

Opening a folder queues every media file in it (sorted); playback advances
automatically and the Next button/`N` skips ahead.

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
- The window's framebuffer size is `window size × content scale`, so rendering
  is correct on macOS Retina and Windows HiDPI displays.
- The render loop runs at 60 FPS: poll input → render the mpv frame into the
  framebuffer → draw the ImGui control bar → swap buffers.
- Playback state (`time-pos`, `duration`, `pause`, `eof-reached`) is polled
  once per frame; playlist advance happens on `eof-reached`.

The window is frameless; drag the empty video area to move it, `F` toggles
fullscreen, and `Esc` quits.

## Layout

```
main.go               entry point: parses the optional file argument
internal/libmpv/      minimal cgo binding for the libmpv client + render APIs
  libmpv.go           handle, options, properties, commands
  render.go           OpenGL render context (get_proc_address via GLFW)
internal/glfwutil/    GLFW fullscreen + monitor work-area helpers
internal/app/         app controller: mpv lifecycle, playlist, file dialogs, loop
internal/ui/          Dear ImGui controls: transport, seek bar, volume, context menu
```

## Known limitations

- Rendering requires an OpenGL 3.x+ context (the ImGui GLFW backend's default);
  macOS supports this via the bundled GLFW/OpenGL3 loader.
- Hardware decoding is requested via `hwdec=auto` but not asserted; if a
  codec/driver combination falls back to software decoding, playback still
  works, just with higher CPU usage.
