package ui

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// iconButton draws an icon-only button and fires onClick when clicked.
// accent buttons use the theme contrast background; the rest are flat.
func (u *UI) iconButton(gtx layout.Context, btn *widget.Clickable, icon *widget.Icon, desc string, accent bool, onClick func()) layout.Dimensions {
	for btn.Clicked(gtx) {
		onClick()
	}
	style := material.IconButton(u.th, btn, icon, desc)
	style.Inset = layout.UniformInset(unit.Dp(8))
	if accent {
		style.Background = u.th.Palette.ContrastBg
		style.Color = u.th.Palette.ContrastFg
	} else {
		style.Background = color.NRGBA{}
		style.Color = color.NRGBA{A: 0xff, R: 0xd0, G: 0xd5, B: 0xdd}
	}
	return style.Layout(gtx)
}

// icon draws a static icon at the given dp size.
func (u *UI) icon(gtx layout.Context, icon *widget.Icon, size unit.Dp) layout.Dimensions {
	if icon == nil {
		return layout.Dimensions{}
	}
	sz := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(sz, sz))
	return icon.Layout(gtx, u.th.Palette.Fg)
}
