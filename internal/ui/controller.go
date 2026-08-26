package ui

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
}
