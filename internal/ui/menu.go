package ui

import (
	"fmt"
	"image"
	"os"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// menuLayout draws the right-click context menu at the cursor plus a
// full-window dismissal area underneath it: any click closes the menu, and
// menu item clicks reach the items because both handlers receive the events.
func (u *UI) menuLayout(gtx layout.Context, c Controller) layout.Dimensions {
	if !u.menuOpen {
		return layout.Dimensions{}
	}
	fmt.Fprintln(os.Stderr, "DBG menuLayout: menu open")
	for u.menuDismiss.Clicked(gtx) {
		u.menuOpen = false
		fmt.Fprintln(os.Stderr, "DBG menuDismiss CLICKED -> close")
	}
	u.menuDismiss.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})

	// The panel is positioned at the cursor (converted from dp to px).
	off := op.Offset(image.Pt(int(u.menuPos.X*gtx.Metric.PxPerDp), int(u.menuPos.Y*gtx.Metric.PxPerDp))).Push(gtx.Ops)
	macro := op.Record(gtx.Ops)
	dims := u.menuItems(gtx, c)
	call := macro.Stop()
u.menuRect = image.Rectangle{
		Min: image.Pt(int(u.menuPos.X), int(u.menuPos.Y)),
		Max: image.Pt(int(u.menuPos.X)+int(gtx.Metric.PxToDp(dims.Size.X)), int(u.menuPos.Y)+int(gtx.Metric.PxToDp(dims.Size.Y))),
	}
	fmt.Fprintf(os.Stderr, "DBG menuRect %v dims %v pos %v scale %v\n", u.menuRect, dims.Size, u.menuPos, gtx.Metric.PxPerDp)
	paint.FillShape(gtx.Ops, rgba(0x2e2e2e, 0xf2), clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(8)).Op(gtx.Ops))
	call.Add(gtx.Ops)
	off.Pop()
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func (u *UI) menuItems(gtx layout.Context, c Controller) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.menuItem(gtx, &u.menuOpenFile, u.icons.openFile, "Open File", func() {
				c.OpenFile()
				u.menuOpen = false
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.menuItem(gtx, &u.menuOpenFolder, u.icons.openFolder, "Open Folder", func() {
				c.OpenFolder()
				u.menuOpen = false
			})
		}),
	)
}

func (u *UI) menuItem(gtx layout.Context, btn *widget.Clickable, icon *widget.Icon, label string, onClick func()) layout.Dimensions {
	for btn.Clicked(gtx) {
		fmt.Fprintf(os.Stderr, "DBG menu item %q CLICKED\n", label)
		onClick()
	}
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return u.icon(gtx, icon, 18)
				}),
				layout.Rigid(layout.Spacer{Width: 10}.Layout),
				layout.Rigid(material.Label(u.th, u.th.TextSize, label).Layout),
			)
		})
	})
}
