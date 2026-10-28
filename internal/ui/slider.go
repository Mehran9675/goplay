package ui

import "time"

// syncSliders keeps the sliders tracking the player state, except while
// the user is dragging one or has just clicked the seek bar: then the
// user's value wins and the app is seeked/set through the sliders'
// OnChange callbacks.
func (u *UI) syncSliders(c Controller) {
	if !u.seek.Dragging() && time.Now().After(u.seekHold) {
		v := 0.0
		if c.Duration() > 0 {
			v = clamp01(c.Pos() / c.Duration())
		}
		u.seek.SetValue(float32(v))
	}
	if !u.vol.Dragging() {
		u.vol.SetValue(float32(clamp01(c.Volume())))
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
