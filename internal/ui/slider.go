package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/widget/material"
)

// seekRow draws the progress slider; clicking or dragging seeks. While the
// user drags, the slider keeps the dragged value; otherwise it tracks
// c.Pos()/c.Duration().
func (u *UI) seekRow(gtx layout.Context, c Controller) layout.Dimensions {
	height := gtx.Dp(20)
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	if c.Duration() <= 0 {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(material.Label(u.th, u.th.TextSize, "--:--").Layout),
		)
	}
	if !u.seekFloat.Dragging() {
		u.seekFloat.Value = float32(clamp01(c.Pos() / c.Duration()))
	}
	prev := u.seekFloat.Value
	sl := material.Slider(u.th, &u.seekFloat)
	sl.Axis = layout.Horizontal
	sl.FingerSize = 20
	sl.Layout(gtx)
	if u.seekFloat.Value != prev {
		c.Seek(float64(u.seekFloat.Value))
	}
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// volumeSlider draws the volume slider next to the volume icon.
func (u *UI) volumeSlider(gtx layout.Context, c Controller) layout.Dimensions {
	gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(90), gtx.Dp(24)))
	if !u.volFloat.Dragging() {
		u.volFloat.Value = float32(clamp01(c.Volume()))
	}
	prev := u.volFloat.Value
	sl := material.Slider(u.th, &u.volFloat)
	sl.Axis = layout.Horizontal
	sl.FingerSize = 20
	sl.Layout(gtx)
	if u.volFloat.Value != prev {
		c.SetVolume(float64(u.volFloat.Value))
	}
	return layout.Dimensions{Size: gtx.Constraints.Min}
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
