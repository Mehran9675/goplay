package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// controlsLayout positions the control bar at the bottom of the window.
func (u *UI) controlsLayout(gtx layout.Context, c Controller) layout.Dimensions {
	win := gtx.Constraints.Max
	barH := gtx.Dp(controlH)
	winDp := image.Pt(int(gtx.Metric.PxToDp(win.X)), int(gtx.Metric.PxToDp(win.Y)))
	u.controlRect = image.Rectangle{
		Min: image.Pt(0, winDp.Y-controlH),
		Max: winDp,
	}
	return layout.Stack{Alignment: layout.S}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(win.X, barH))
			return u.drawControls(gtx, c)
		}),
	)
}

// drawControls draws the translucent bar and its rows: the seek slider on
// top, then the transport buttons and the right-aligned volume/fullscreen
// group.
func (u *UI) drawControls(gtx layout.Context, c Controller) layout.Dimensions {
	paint.FillShape(gtx.Ops, rgba(0x0b0e14, 0xcc), clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.seekRow(gtx, c)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.transportRow(gtx, c)
			}),
		)
	})
}

// transportRow is the row of transport buttons, time label, volume and
// fullscreen controls.
func (u *UI) transportRow(gtx layout.Context, c Controller) layout.Dimensions {
	sp := func(w unit.Dp) layout.FlexChild {
		return layout.Rigid(layout.Spacer{Width: w}.Layout)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.iconButton(gtx, &u.openBtn, u.icons.openFile, "Open file (O)", false, c.OpenFile)
		}),
		sp(2),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.iconButton(gtx, &u.folderBtn, u.icons.openFolder, "Open folder (D)", false, c.OpenFolder)
		}),
		sp(4),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			playing := c.HasMedia() && !c.Paused() && !c.Finished()
			icon := u.icons.play
			tip := "Play (Space)"
			if playing {
				icon = u.icons.pause
				tip = "Pause (Space)"
			}
			return u.iconButton(gtx, &u.playBtn, icon, tip, true, c.TogglePause)
		}),
		sp(2),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.iconButton(gtx, &u.stopBtn, u.icons.stop, "Stop (S)", false, c.Stop)
		}),
		sp(2),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.iconButton(gtx, &u.nextBtn, u.icons.next, "Next (N)", false, c.Next)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Label(u.th, u.th.TextSize, formatTime(c.Pos())+" / "+formatTime(c.Duration())).Layout(gtx)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			vol := c.Volume()
			icon := u.icons.volumeOff
			if vol > 0.5 {
				icon = u.icons.volumeUp
			}
			return u.icon(gtx, icon, 20)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.volumeSlider(gtx, c)
		}),
		sp(4),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			icon := u.icons.fullscreen
			tip := "Fullscreen (F)"
			if c.Fullscreen() {
				icon = u.icons.fullscreenOut
				tip = "Exit fullscreen (F)"
			}
			return u.iconButton(gtx, &u.fullscreenBtn, icon, tip, false, c.ToggleFullscreen)
		}),
	)
}
