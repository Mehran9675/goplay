package platform

import (
	"image"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/io/system"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Frameless windows move and resize themselves: a press on a drag area
// (the title bar's empty space, or wherever CanDrag says) becomes a
// window move once the pointer travels a few pixels, and a press on the
// window's edge resizes it.

// dragSlop is how far, in screen coordinates, the pointer must move
// before a press on a drag area moves the window.
const dragSlop = 4

// resizeBorder is the width, in screen coordinates, of the band along
// the window's edges that resizes it.
const resizeBorder = 6

type dragState struct {
	pending, active bool
	startCursor     image.Point // screen coordinates
	startWindow     image.Point
}

// Resize edges, combinable.
const (
	edgeLeft = 1 << iota
	edgeRight
	edgeTop
	edgeBottom
)

type resizeState struct {
	active      bool
	edges       int
	startCursor image.Point
	startBounds image.Rectangle
}

func (w *Window) screenCursor() image.Point {
	x, y := w.win.GetPos()
	return image.Pt(x+int(w.cursor.X), y+int(w.cursor.Y))
}

// resizeEdge returns the edges under p (window coordinates), or 0.
func (w *Window) resizeEdge(p f32.Point) int {
	if w.maximized || w.fullscreen || !nativeResizeMissing {
		return 0
	}
	ww, wh := w.win.GetSize()
	e := 0
	switch {
	case p.X < resizeBorder:
		e |= edgeLeft
	case p.X >= float32(ww-resizeBorder):
		e |= edgeRight
	}
	switch {
	case p.Y < resizeBorder:
		e |= edgeTop
	case p.Y >= float32(wh-resizeBorder):
		e |= edgeBottom
	}
	return e
}

// beginResizeOrDrag starts an edge resize, which takes the press, or
// arms a window drag, which doesn't (the UI still sees the press, so a
// title bar's double-click works). It reports whether the press was
// taken.
func (w *Window) beginResizeOrDrag() bool {
	if e := w.resizeEdge(w.cursor); e != 0 {
		w.resize = resizeState{active: true, edges: e, startCursor: w.screenCursor(), startBounds: w.Bounds()}
		return true
	}
	if w.fullscreen {
		return false
	}
	p := w.ToPx(w.cursor)
	action, ok := w.ActionAt(p)
	switch {
	case ok && action == system.ActionMove:
	case !ok && w.CanDrag != nil && w.CanDrag(p):
	default:
		return false
	}
	x, y := w.win.GetPos()
	w.drag = dragState{pending: true, startCursor: w.screenCursor(), startWindow: image.Pt(x, y)}
	return false
}

// trackDragResize moves or resizes the window while the button is held.
// Called on every cursor move; it reports whether it took the move.
func (w *Window) trackDragResize() bool {
	if w.win.GetMouseButton(glfw.MouseButtonLeft) != glfw.Press {
		w.drag, w.resize = dragState{}, resizeState{}
		return false
	}
	cur := w.screenCursor()
	switch {
	case w.resize.active:
		d := cur.Sub(w.resize.startCursor)
		r := w.resize.startBounds
		if w.resize.edges&edgeLeft != 0 {
			r.Min.X = min(r.Min.X+d.X, r.Max.X-minWindowSize)
		}
		if w.resize.edges&edgeRight != 0 {
			r.Max.X = max(r.Max.X+d.X, r.Min.X+minWindowSize)
		}
		if w.resize.edges&edgeTop != 0 {
			r.Min.Y = min(r.Min.Y+d.Y, r.Max.Y-minWindowSize)
		}
		if w.resize.edges&edgeBottom != 0 {
			r.Max.Y = max(r.Max.Y+d.Y, r.Min.Y+minWindowSize)
		}
		w.SetBounds(r)
		return true
	case w.drag.pending:
		d := cur.Sub(w.drag.startCursor)
		if abs(d.X) < dragSlop && abs(d.Y) < dragSlop {
			return false
		}
		w.drag.pending, w.drag.active = false, true
		// The UI saw the press; tell it the gesture is over.
		w.buttons = 0
		w.Router.Queue(pointer.Event{Kind: pointer.Cancel, Source: pointer.Mouse, Time: time.Since(w.start)})
		fallthrough
	case w.drag.active:
		if w.maximized {
			// Dragging a maximized window restores it under the pointer.
			w.maximized = false
			r := w.restore
			x := cur.X - r.Dx()/2
			w.SetBounds(image.Rect(x, w.drag.startWindow.Y, x+r.Dx(), w.drag.startWindow.Y+r.Dy()))
			px, py := w.win.GetPos()
			w.drag.startWindow = image.Pt(px, py)
			w.drag.startCursor = cur
			w.dirty = true
		}
		p := w.drag.startWindow.Add(cur.Sub(w.drag.startCursor))
		w.win.SetPos(p.X, p.Y)
		return true
	}
	return false
}

// minWindowSize is the smallest the edge resize makes the window.
const minWindowSize = 240

// updateResizeCursor shows a resize cursor over the window's edges.
func (w *Window) updateResizeCursor() {
	e := w.resizeEdge(w.cursor)
	switch {
	case e == 0:
		return
	case e&(edgeLeft|edgeRight) != 0 && e&(edgeTop|edgeBottom) == 0:
		w.applyShape(glfw.HResizeCursor)
	case e&(edgeTop|edgeBottom) != 0 && e&(edgeLeft|edgeRight) == 0:
		w.applyShape(glfw.VResizeCursor)
	default:
		w.applyShape(glfw.CrosshairCursor)
	}
	w.cursorStale = true // the UI's cursor applies again off the edge
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
