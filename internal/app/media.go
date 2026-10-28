package app

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ncruces/zenity"

	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

func isURL(s string) bool { return library.IsURL(s) }

// mediaFilter is the file dialogs' filter for video and audio files.
func mediaFilter() zenity.FileFilters {
	var pats []string
	for ext := range library.MediaExts {
		pats = append(pats, "*"+ext)
	}
	return zenity.FileFilters{{Name: "Video and audio", Patterns: pats, CaseFold: true}}
}

func subtitleFilter() zenity.FileFilters {
	var pats []string
	for ext := range library.SubtitleExts {
		pats = append(pats, "*"+ext)
	}
	return zenity.FileFilters{{Name: "Subtitles", Patterns: pats, CaseFold: true}}
}

// dialog runs a native file dialog off the UI goroutine (it blocks) and
// hands the result back with Post.
func (a *App) dialog(show func() ([]string, error), then func([]string)) {
	go func() {
		paths, err := show()
		a.Post(func() {
			switch {
			case err == zenity.ErrCanceled:
			case err != nil:
				a.toastError("The file dialog failed: " + err.Error())
			case len(paths) > 0:
				then(paths)
			}
		})
	}()
}

func (a *App) openFileDialog() {
	a.dialog(func() ([]string, error) {
		return zenity.SelectFileMultiple(zenity.Title("Open"), mediaFilter())
	}, func(p []string) { a.openItems(p, false) })
}

func (a *App) openFolderDialog() {
	a.dialog(func() ([]string, error) {
		d, err := zenity.SelectFile(zenity.Title("Open Folder"), zenity.Directory())
		return []string{d}, err
	}, func(p []string) { a.openItems(p, false) })
}

func (a *App) addFilesDialog() {
	a.dialog(func() ([]string, error) {
		return zenity.SelectFileMultiple(zenity.Title("Add to Playlist"), mediaFilter())
	}, func(p []string) { a.openItems(p, true) })
}

func (a *App) loadSubtitleDialog() {
	a.dialog(func() ([]string, error) {
		f, err := zenity.SelectFile(zenity.Title("Load Subtitles"), subtitleFilter())
		return []string{f}, err
	}, func(p []string) { a.addSubtitle(p[0]) })
}

func (a *App) loadAudioDialog() {
	a.dialog(func() ([]string, error) {
		f, err := zenity.SelectFile(zenity.Title("Load Audio Track"), mediaFilter())
		return []string{f}, err
	}, func(p []string) {
		if err := a.player.AddAudio(p[0]); err != nil {
			a.toastError("Couldn't load the audio track: " + err.Error())
		}
	})
}

func (a *App) addSubtitle(path string) {
	if err := a.player.AddSubtitle(path); err != nil {
		a.toastError("Couldn't load the subtitles: " + err.Error())
		return
	}
	a.osd("Subtitles: " + filepath.Base(path))
}

// openItems plays files, folders and URLs, or adds them to the playlist.
// A single file brings the rest of its folder along when that setting is
// on, starting at the file itself.
func (a *App) openItems(items []string, appendToPlaylist bool) {
	if len(items) == 0 {
		return
	}
	if a.settings.Bool("resume") {
		a.player.SaveResumePosition()
	}
	list, start := library.Expand(items, !appendToPlaylist && a.settings.Bool(settings.KeyQueueFolder))
	if len(list) == 0 {
		a.toastError("There's nothing to play there.")
		return
	}
	if err := a.player.Open(list, appendToPlaylist); err != nil {
		a.toastError(err.Error())
		return
	}
	if appendToPlaylist {
		a.toast(plural(len(list), "file", "files") + " added to the playlist")
		return
	}
	if start > 0 {
		a.player.PlaylistPlay(start)
	}
	if a.casting() {
		a.Post(a.castCurrent)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// dropped handles files dropped on the window: subtitle files join the
// playing video; anything else plays.
func (a *App) dropped(paths []string) {
	var media []string
	for _, p := range paths {
		if library.IsSubtitle(p) && !a.player.State.Idle {
			a.addSubtitle(p)
			continue
		}
		media = append(media, p)
	}
	a.openItems(media, false)
}

// openClipboard plays a link or path from the clipboard.
func (a *App) openClipboard() {
	text := strings.TrimSpace(a.win.Clipboard())
	if text == "" {
		a.toast("The clipboard is empty")
		return
	}
	var items []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.Trim(strings.TrimSpace(line), `"`); line != "" {
			items = append(items, line)
		}
	}
	for _, it := range items {
		if _, err := os.Stat(it); err != nil && !isURL(it) {
			a.toast("The clipboard doesn't hold a link or a file")
			return
		}
	}
	a.openItems(items, false)
}

// openURL plays a web link (a direct stream, or a page yt-dlp knows).
func (a *App) openURL(u string) {
	u = strings.TrimSpace(u)
	if u == "" {
		return
	}
	if !isURL(u) && !strings.Contains(u, "://") {
		if _, err := os.Stat(u); err != nil {
			u = "https://" + u
		}
	}
	a.openItems([]string{u}, false)
}

func (a *App) revealCurrent() {
	path := a.player.State.Path
	if path == "" || isURL(path) {
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", path)
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	startDetached(cmd)
}

func (a *App) copyCurrentPath() {
	a.win.SetClipboard(a.player.State.Path)
	a.toast("Copied")
}

// openConfigFolder opens goplay's configuration folder, where settings,
// mpv.conf, input.conf and scripts live.
func (a *App) openConfigFolder() {
	if a.configDir == "" {
		return
	}
	os.MkdirAll(filepath.Join(a.configDir, "scripts"), 0o755)
	openPath(a.configDir)
}

// editConfigFile opens mpv.conf or input.conf in a text editor, creating
// it with a short guide first.
func (a *App) editConfigFile(name string) {
	if a.configDir == "" {
		return
	}
	path := filepath.Join(a.configDir, name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.MkdirAll(a.configDir, 0o755)
		os.WriteFile(path, []byte(configTemplates[name]), 0o644)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad.exe", path)
	case "darwin":
		cmd = exec.Command("open", "-t", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	startDetached(cmd)
	a.toast(name + " is read when goplay starts")
}

var configTemplates = map[string]string{
	"mpv.conf": `# mpv options, one per line: name=value
# The full list: https://mpv.io/manual/stable/#options
# Read when goplay starts; these override goplay's own settings.
#
# Examples:
# sub-font="Segoe UI"
# screenshot-format=png
# deband-iterations=2
`,
	"input.conf": `# Key bindings for mpv commands, one per line: KEY command
# The syntax: https://mpv.io/manual/stable/#input-conf
# goplay's own shortcuts come first (change them in Settings > Shortcuts);
# keys goplay doesn't use reach these bindings.
#
# Examples:
# ctrl+h cycle-values hwdec "auto-safe" "no"
# g cycle-values video-aspect-override "16:9" "4:3" "no"
`,
}

func openPath(p string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", p)
	case "darwin":
		cmd = exec.Command("open", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	startDetached(cmd)
}

func openWebPage(u string) { openPath(u) }

func startDetached(cmd *exec.Cmd) {
	if cmd.Start() == nil {
		go cmd.Wait()
	}
}

// setSetting stores a setting and puts it into effect.
func (a *App) setSetting(key, value string) {
	if a.settings.Get(key) == value {
		return
	}
	a.settings.Set(key, value)
	a.applySetting(key)
}

// applySetting puts a stored setting into effect: its mpv options, or
// the app behavior it controls.
func (a *App) applySetting(key string) {
	d := settings.Lookup(key)
	if d == nil {
		return
	}
	v := a.settings.Get(key)
	switch key {
	case settings.KeyAlwaysOnTop:
		a.win.SetFloating(v == "yes")
	case settings.KeyStereoView:
		if a.stereo.layout != player.Layout2D {
			a.stereo.view = player.StereoView(v)
			a.applyStereo()
		}
	case settings.KeyVRFOV:
		if a.vrActive() {
			a.stereo.vr.FOV = a.settings.Float(settings.KeyVRFOV)
			a.applyStereo()
		}
	}
	if d.Restart {
		a.toast(d.Label + ": takes effect when goplay restarts")
		return
	}
	var opts []settings.MPVOption
	if d.Group != "" {
		opts = []settings.MPVOption{a.settings.GroupOption(d.Group)}
	} else {
		opts = d.MPVOptions(v)
	}
	for _, o := range opts {
		if err := a.player.Set(o.Name, o.Value); err != nil {
			a.toastError(d.Label + ": mpv didn't accept " + o.Name + "=" + o.Value)
		}
	}
}

// Window modes.

func (a *App) setFullscreen(on bool) {
	if a.ui.pip && on {
		a.togglePiP()
	}
	a.win.SetFullscreen(on)
	a.dirty = true
}

// togglePiP shrinks the window to a small always-on-top player in the
// corner of the screen, or brings it back.
func (a *App) togglePiP() {
	u := a.ui
	if u.pip {
		u.pip = false
		a.win.SetFloating(u.pipWasFloating)
		a.win.SetBounds(u.pipRestore)
		return
	}
	if a.win.Fullscreen() {
		a.win.SetFullscreen(false)
	}
	if a.win.Maximized() {
		a.win.SetMaximized(false)
	}
	u.pip = true
	u.pipRestore = a.win.Bounds()
	u.pipWasFloating = a.win.Floating()
	work := a.win.WorkArea()
	w := work.Dx() / 4
	h := w * 9 / 16
	if st := &a.player.State; st.VideoW > 0 && st.VideoH > 0 {
		h = w * st.VideoH / st.VideoW
	}
	margin := work.Dx() / 60
	r := image.Rect(work.Max.X-w-margin, work.Max.Y-h-margin, work.Max.X-margin, work.Max.Y-margin)
	a.win.SetFloating(true)
	a.win.SetBounds(r)
}

// sizeToVideo fits the window to the video when a new one starts,
// leaving room for the bars around it.
func (a *App) sizeToVideo() {
	st := &a.player.State
	if !a.settings.Bool(settings.KeyResizeToVideo) || st.VideoW <= 0 || st.VideoH <= 0 ||
		a.win.Maximized() || a.win.Fullscreen() || a.ui.pip {
		return
	}
	work := a.win.WorkArea()
	if work.Empty() {
		return
	}
	// Window sizes are in screen coordinates, which are points on a
	// Retina Mac: one video pixel per screen pixel either way.
	pps := a.win.ScreenToPx(1)
	chrome := int(float32(a.ui.titleHeightPx()+a.ui.barHeightPx()) / pps)
	maxW, maxH := work.Dx()*85/100, work.Dy()*85/100-chrome
	w, h := float32(st.VideoW)/pps, float32(st.VideoH)/pps
	if f := min(float32(maxW)/w, float32(maxH)/h, 1); f < 1 {
		w, h = w*f, h*f
	}
	a.win.SetSize(max(int(w), 480), max(int(h), 270)+chrome)
}
