package app

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/ncruces/zenity"

	"github.com/mehran9675/goplay-ebiten/internal/ui"
)

var subtitlePatterns = []string{"*.srt", "*.ass", "*.ssa", "*.vtt", "*.sub", "*.idx", "*.sup", "*.smi"}

// trackProp maps a track kind to the mpv property that selects it.
var trackProp = map[ui.TrackKind]string{
	ui.TrackVideo:    "vid",
	ui.TrackAudio:    "aid",
	ui.TrackSubtitle: "sid",
}

// Tracks lists the current file's tracks of kind from mpv's track-list.
func (a *App) Tracks(kind ui.TrackKind) []ui.Track {
	if a.mpv == nil || !a.hasMedia {
		return nil
	}
	n, _ := a.mpv.GetInt("track-list/count")
	var tracks []ui.Track
	for i := range int(n) {
		p := fmt.Sprintf("track-list/%d/", i)
		if t, _ := a.mpv.GetString(p + "type"); t != string(kind) {
			continue
		}
		id, ok := a.mpv.GetInt(p + "id")
		if !ok {
			continue
		}
		sel, _ := a.mpv.GetBool(p + "selected")
		tracks = append(tracks, ui.Track{ID: int(id), Title: a.trackTitle(p, int(id)), Selected: sel})
	}
	return tracks
}

// trackTitle builds a display name for the track at property prefix p:
// its title (or external file name, or codec and size for video), then
// its language.
func (a *App) trackTitle(p string, id int) string {
	name, _ := a.mpv.GetString(p + "title")
	if name == "" {
		if ext, _ := a.mpv.GetString(p + "external-filename"); ext != "" {
			name = filepath.Base(ext)
		}
	}
	if name == "" {
		if t, _ := a.mpv.GetString(p + "type"); t == string(ui.TrackVideo) {
			codec, _ := a.mpv.GetString(p + "codec")
			w, okW := a.mpv.GetInt(p + "demux-w")
			h, okH := a.mpv.GetInt(p + "demux-h")
			if codec != "" && okW && okH {
				name = fmt.Sprintf("%s %dx%d", codec, w, h)
			}
		}
	}
	if name == "" {
		name = fmt.Sprintf("Track %d", id)
	}
	if lang, _ := a.mpv.GetString(p + "lang"); lang != "" {
		name += " (" + lang + ")"
	}
	return name
}

// SelectTrack switches the kind's track to id, or off for id 0.
func (a *App) SelectTrack(kind ui.TrackKind, id int) {
	prop, ok := trackProp[kind]
	if a.mpv == nil || !ok {
		return
	}
	v := "no"
	if id > 0 {
		v = strconv.Itoa(id)
	}
	if err := a.mpv.SetString(prop, v); err != nil {
		a.msg = err.Error()
	}
}

// LoadSubtitle asks for a subtitle file; drainDialogs adds it with
// sub-add once chosen.
func (a *App) LoadSubtitle() {
	if !a.hasMedia {
		a.msg = "Open a video before loading subtitles."
		return
	}
	a.openDialog(func() dialogResult {
		f, err := zenity.SelectFile(
			zenity.Title("Load subtitle file"),
			zenity.FileFilters{{Name: "Subtitle files", Patterns: subtitlePatterns, CaseFold: true}},
		)
		if err != nil {
			return dialogResult{err: err}
		}
		return dialogResult{files: []string{f}, subtitle: true}
	})
}

// addSubtitle loads path as an external subtitle track and selects it.
func (a *App) addSubtitle(path string) {
	if a.mpv == nil {
		return
	}
	if err := a.mpv.Command("sub-add", path, "select"); err != nil {
		a.msg = "Could not load subtitle: " + err.Error()
	}
}

func (a *App) AspectRatio() string { return a.aspect }

// SetAspectRatio forces the display aspect ratio; "" restores the
// file's own.
func (a *App) SetAspectRatio(ratio string) {
	if a.mpv == nil {
		return
	}
	v := ratio
	if v == "" {
		v = "no"
	}
	if err := a.mpv.SetString("video-aspect-override", v); err != nil {
		a.msg = err.Error()
		return
	}
	a.aspect = ratio
}

func (a *App) Speed() float64 {
	if a.mpv == nil {
		return 1
	}
	if s, ok := a.mpv.GetDouble("speed"); ok {
		return s
	}
	return 1
}

func (a *App) SetSpeed(speed float64) {
	if a.mpv != nil {
		a.mpv.SetDouble("speed", speed)
	}
}

// Loop reports whether the current file repeats.
func (a *App) Loop() bool {
	if a.mpv == nil {
		return false
	}
	v, _ := a.mpv.GetString("loop-file")
	return v != "" && v != "no"
}

func (a *App) ToggleLoop() {
	if a.mpv == nil {
		return
	}
	v := "inf"
	if a.Loop() {
		v = "no"
	}
	a.mpv.SetString("loop-file", v)
}
