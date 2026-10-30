package app

import (
	"context"
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

func isURL(s string) bool { return library.IsURL(s) }

// mediaTypes and subtitleTypes are what the file pickers offer.
func mediaTypes() []string    { return extList(library.MediaExts) }
func subtitleTypes() []string { return extList(library.SubtitleExts) }

func extList(exts map[string]bool) []string {
	var out []string
	for ext := range exts {
		out = append(out, ext)
	}
	return out
}

// pick shows the file picker off the UI goroutine (it blocks) and hands
// the paths back with Post.
func (a *App) pick(types []string, multiple bool, then func([]string)) {
	a.picked(func() ([]string, error) {
		files, err := a.g.PickFiles(context.Background(), giode.PickOptions{Types: types, Multiple: multiple})
		var paths []string
		for _, f := range files {
			f.Close()
			if f.Path != "" {
				paths = append(paths, f.Path)
			}
		}
		return paths, err
	}, then)
}

// picked runs a picker and hands its paths to then on the UI goroutine.
func (a *App) picked(show func() ([]string, error), then func([]string)) {
	go func() {
		paths, err := show()
		a.Post(func() {
			switch {
			case errors.Is(err, giode.ErrCanceled):
			case err != nil:
				a.toastError(tr("The file dialog failed: {error}", "error", err))
			case len(paths) > 0:
				then(paths)
			}
		})
	}()
}

func (a *App) openFileDialog() {
	a.pick(mediaTypes(), true, func(p []string) { a.openItems(p, false) })
}

func (a *App) openFolderDialog() {
	a.picked(func() ([]string, error) {
		d, err := a.g.PickFolder(context.Background(), giode.FolderOptions{Title: tr("Open Folder")})
		return []string{d}, err
	}, func(p []string) { a.openItems(p, false) })
}

func (a *App) addFilesDialog() {
	a.pick(mediaTypes(), true, func(p []string) { a.openItems(p, true) })
}

func (a *App) loadSubtitleDialog() {
	a.pick(subtitleTypes(), false, func(p []string) { a.addSubtitle(p[0]) })
}

func (a *App) loadAudioDialog() {
	a.pick(mediaTypes(), false, func(p []string) {
		if err := a.player.AddAudio(p[0]); err != nil {
			a.toastError(tr("Couldn't load the audio track: {error}", "error", err))
		}
	})
}

func (a *App) addSubtitle(path string) {
	if err := a.player.AddSubtitle(path); err != nil {
		a.toastError(tr("Couldn't load the subtitles: {error}", "error", err))
		return
	}
	a.osd(tr("Subtitles: {track}", "track", filepath.Base(path)))
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
		a.toastError(tr("There's nothing to play there."))
		return
	}
	if err := a.player.Open(list, appendToPlaylist); err != nil {
		a.toastError(err.Error())
		return
	}
	if appendToPlaylist {
		a.toast(tr("{count} files added to the playlist", "count", len(list)))
		return
	}
	if start > 0 {
		a.player.PlaylistPlay(start)
	}
	if a.casting() {
		a.Post(a.castCurrent)
	}
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
	a.g.ReadClipboard(a.openText)
}

// openText plays the links or paths in text, one per line.
func (a *App) openText(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		a.toast(tr("The clipboard is empty"))
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
			a.toast(tr("The clipboard doesn't hold a link or a file"))
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
	a.g.SetClipboard(a.player.State.Path)
	a.toast(tr("Copied"))
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
	a.toast(tr("{file} is read when GoPlay starts", "file", name))
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

// finished reports whether everything opened has played: the last file
// ended and nothing loops.
func (a *App) finished() bool {
	st := &a.player.State
	return !st.Idle && !st.Loading && st.EOF && st.Paused && !a.casting() &&
		!st.LoopFile && !st.LoopPlaylist && st.PlaylistPos == len(st.Playlist)-1
}

// playAgain plays everything opened from the start.
func (a *App) playAgain() {
	if len(a.player.State.Playlist) > 1 {
		a.player.PlaylistPlay(0)
		a.player.SetPause(false)
		return
	}
	a.player.TogglePause()
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
	case settings.KeyLanguage:
		giode.SetLocale(uiLocale(v))
		a.rebuildUI()
		return
	case settings.KeyAlwaysOnTop:
		a.setOnTop(v == "yes")
	case settings.KeyAudioDevice:
		// The player follows the device as it comes and goes.
		a.player.SetAudioDevice(v)
		return
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
		a.toast(tr("{setting}: takes effect when GoPlay restarts", "setting", tr(d.Label)))
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
			a.toastError(tr("{setting}: mpv didn't accept {option}", "setting", tr(d.Label), "option", o.Name+"="+o.Value))
		}
	}
}

// Window modes.

func (a *App) setFullscreen(on bool) {
	if a.ui.pip && on {
		a.togglePiP()
	}
	a.g.SetFullscreen(on)
	a.dirty = true
}

// setOnTop keeps the window above others, or not.
func (a *App) setOnTop(on bool) {
	a.g.SetAlwaysOnTop(on)
}

// pipWidth is the picture-in-picture window's width in dp, and pipMargin
// its distance from the screen's edges.
const pipWidth, pipMargin = 420, 16

// togglePiP shrinks the window to a small always-on-top player in the
// screen's bottom right corner, or brings it back.
func (a *App) togglePiP() {
	u := a.ui
	if u.pip {
		u.pip = false
		a.setOnTop(u.pipWasOnTop)
		a.g.SetSize(int(u.pipRestore.Width+0.5), int(u.pipRestore.Height+0.5))
		if u.pipHadFrame {
			a.g.SetPosition(u.pipRestore.X, u.pipRestore.Y)
		}
		return
	}
	if a.g.Fullscreen() {
		a.g.SetFullscreen(false)
	}
	if a.g.Maximized() {
		a.g.Unmaximize()
	}
	u.pip = true
	sz := a.windowSizeDp()
	u.pipRestore = giode.Rect{Width: float32(sz.X), Height: float32(sz.Y)}
	if f, err := a.g.Frame(); err == nil {
		u.pipRestore.X, u.pipRestore.Y = f.X, f.Y
		u.pipHadFrame = true
	} else {
		u.pipHadFrame = false
	}
	u.pipWasOnTop = a.g.AlwaysOnTop()
	w, h := pipWidth, pipWidth*9/16
	if st := &a.player.State; st.VideoW > 0 && st.VideoH > 0 {
		h = w * st.VideoH / st.VideoW
	}
	a.setOnTop(true)
	a.g.SetSize(w, h)
	// Where windows can't be placed (Wayland), it stays where it is.
	a.g.MoveTo(giode.CornerBottomRight, pipMargin)
}

// windowSizeDp is the window's size in dp.
func (a *App) windowSizeDp() image.Point {
	s := a.ui.scale
	if s <= 0 {
		s = 1
	}
	return image.Pt(int(float32(a.winSize.X)/s+0.5), int(float32(a.winSize.Y)/s+0.5))
}

// maxFit is the share of the screen's work area sizeToVideo fills at
// most, and maxFitW, maxFitH the limit in dp where the screen's size
// isn't known (Wayland).
const (
	maxFit           = 0.9
	maxFitW, maxFitH = 1280, 800
)

// sizeToVideo fits the window to the video when a new one starts,
// leaving room for the bars around it.
func (a *App) sizeToVideo() {
	st := &a.player.State
	if !a.settings.Bool(settings.KeyResizeToVideo) || st.VideoW <= 0 || st.VideoH <= 0 ||
		a.g.Maximized() || a.g.Fullscreen() || a.ui.pip {
		return
	}
	// One video pixel per screen pixel, in dp.
	s := a.ui.scale
	if s <= 0 {
		s = 1
	}
	chrome := a.ui.titleHeightDp() + controlBarH
	maxW, maxH := float32(maxFitW), float32(maxFitH)
	if sc, err := a.g.Screen(); err == nil && sc.WorkArea.Width > 0 {
		maxW, maxH = sc.WorkArea.Width*maxFit, sc.WorkArea.Height*maxFit
	}
	w, h := float32(st.VideoW)/s, float32(st.VideoH)/s
	if f := min(maxW/w, (maxH-float32(chrome))/h, 1); f < 1 {
		w, h = w*f, h*f
	}
	a.g.SetSize(max(int(w), 480), max(int(h), 270)+chrome)
}
