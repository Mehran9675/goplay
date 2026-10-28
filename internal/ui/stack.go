package ui

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/elements"
)

// Layout helpers for content holding Selects, whose option lists open as
// portals anchored at elements.AbsolutePosition. Giode's flex Boxes (Row,
// column Box) don't report a child's own position there, only the
// container's, so a Select inside one opens its list in the wrong place.
// These helpers place every child themselves and keep AbsolutePosition
// in step.

// place lays el out at off.
func place(gtx layout.Context, off image.Point, el giode.Element) layout.Dimensions {
	t := op.Offset(off).Push(gtx.Ops)
	pop := elements.PushOffset(off)
	dims := el.Layout(gtx)
	pop()
	t.Pop()
	return dims
}

// measured lays el out off-screen into a recording, so it can be placed
// once its size is known. Only for elements without portals: their
// AbsolutePosition is not yet known while recording.
func measured(gtx layout.Context, el giode.Element) (op.CallOp, layout.Dimensions) {
	m := op.Record(gtx.Ops)
	dims := el.Layout(gtx)
	return m.Stop(), dims
}

// vstack lays children out top to bottom, gapDp apart, as wide as the
// constraints allow.
func vstack(gapDp int, children ...giode.Element) giode.Element {
	return elements.Raw(func(gtx layout.Context) layout.Dimensions {
		gap := gtx.Dp(unit.Dp(gapDp))
		gtx.Constraints.Min.Y = 0
		var size image.Point
		for i, ch := range children {
			if i > 0 {
				size.Y += gap
			}
			cgtx := gtx
			cgtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-size.Y)
			dims := place(cgtx, image.Pt(0, size.Y), ch)
			size.X = max(size.X, dims.Size.X)
			size.Y += dims.Size.Y
		}
		size.X = max(size.X, gtx.Constraints.Min.X)
		return layout.Dimensions{Size: size}
	})
}

// formRow lays out a label on the left and control on the right, the
// label centered on the control. A control with fixed width controlDp
// (a Select, which opens a portal) is placed directly; with controlDp 0
// the control is measured first and right-aligned.
func formRow(label giode.Element, control giode.Element, controlDp int) giode.Element {
	return elements.Raw(func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		gap := gtx.Dp(16)

		var cdims layout.Dimensions
		cgtx := gtx
		cgtx.Constraints.Min = image.Point{}
		if controlDp > 0 {
			cw := min(w, gtx.Dp(unit.Dp(controlDp)))
			cgtx.Constraints = layout.Constraints{Min: image.Pt(cw, 0), Max: image.Pt(cw, gtx.Constraints.Max.Y)}
			cdims = place(cgtx, image.Pt(w-cw, 0), control)
		} else {
			call, dims := measured(cgtx, control)
			cdims = dims
			t := op.Offset(image.Pt(w-dims.Size.X, 0)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			t.Pop()
		}

		lgtx := gtx
		lgtx.Constraints.Min = image.Point{}
		lgtx.Constraints.Max.X = max(0, w-cdims.Size.X-gap)
		call, ldims := measured(lgtx, label)
		h := max(cdims.Size.Y, ldims.Size.Y)
		t := op.Offset(image.Pt(0, (h-ldims.Size.Y)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		t.Pop()

		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}
