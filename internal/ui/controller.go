package ui

import "github.com/mehran9675/goplay-ebiten/internal/settings"

// Controller is the subset of the app that the UI needs to drive.
type Controller interface {
	Pos() float64
	Duration() float64
	Volume() float64
	Paused() bool
	Finished() bool
	HasMedia() bool
	Message() string
	TogglePause()
	Stop()
	Next()
	OpenFile()
	OpenFolder()
	Seek(frac float64)
	SeekRelative(seconds float64)
	SetVolume(v float64)
	ToggleFullscreen()
	Fullscreen() bool
	Quit()

	// Tracks lists the current file's tracks of kind.
	Tracks(kind TrackKind) []Track
	// SelectTrack switches to track id of kind; id 0 turns the kind
	// off (no subtitles, say).
	SelectTrack(kind TrackKind, id int)
	// LoadSubtitle asks for a subtitle file and adds it to the current
	// file.
	LoadSubtitle()

	// AspectRatio is the forced display aspect ratio ("16:9", say), or
	// "" for the file's own.
	AspectRatio() string
	SetAspectRatio(ratio string)

	Speed() float64
	SetSpeed(speed float64)
	Loop() bool
	ToggleLoop()

	// Settings is the user's preferences; the settings dialog reads it
	// and changes values through SetSetting, which also applies them.
	Settings() *settings.Store
	SetSetting(key, value string)
	// ResetSettings restores and applies every default.
	ResetSettings()
	// AudioDevices lists the audio outputs for the settings dialog.
	AudioDevices() []settings.Option
	// EditMPVConf opens mpv.conf, creating it if needed, in a text
	// editor.
	EditMPVConf()

	// Title is the window title: the playing file's name.
	Title() string
	// Window controls for the title bar (see titlebar.Window).
	Minimize()
	ToggleMaximize()
	Close()
	Maximized() bool
	Focused() bool
}

// TrackKind is the type of a media track, named as mpv names it.
type TrackKind string

const (
	TrackVideo    TrackKind = "video"
	TrackAudio    TrackKind = "audio"
	TrackSubtitle TrackKind = "sub"
)

// Track is one video, audio or subtitle track of the current file.
type Track struct {
	ID       int
	Title    string // display name, e.g. "English (eng)"
	Selected bool
}
