package platform

import (
	"image"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// monitorOf returns the monitor showing most of r (screen coordinates),
// or the primary monitor when r is off-screen.
func monitorOf(r image.Rectangle) *glfw.Monitor {
	best, bestArea := glfw.GetPrimaryMonitor(), 0
	for _, m := range glfw.GetMonitors() {
		ov := r.Intersect(monitorBounds(m))
		if a := ov.Dx() * ov.Dy(); a > bestArea {
			best, bestArea = m, a
		}
	}
	return best
}

// monitorBounds is a monitor's whole area in screen coordinates.
func monitorBounds(m *glfw.Monitor) image.Rectangle {
	if m == nil {
		return image.Rectangle{}
	}
	x, y := m.GetPos()
	mode := m.GetVideoMode()
	if mode == nil {
		return image.Rectangle{}
	}
	return image.Rect(x, y, x+mode.Width, y+mode.Height)
}

// workArea is a monitor's area minus taskbars, docks and menu bars.
func workArea(m *glfw.Monitor) image.Rectangle {
	if m == nil {
		return image.Rectangle{}
	}
	x, y, w, h := m.GetWorkarea()
	if w <= 0 || h <= 0 {
		return monitorBounds(m)
	}
	return image.Rect(x, y, x+w, y+h)
}

// WorkArea is the work area of the monitor the window is on, in screen
// coordinates.
func (w *Window) WorkArea() image.Rectangle { return workArea(monitorOf(w.Bounds())) }

// centered returns a w×h rectangle with the same center as r.
func centered(r image.Rectangle, w, h int) image.Rectangle {
	c := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	min := image.Pt(c.X-w/2, c.Y-h/2)
	return image.Rectangle{Min: min, Max: min.Add(image.Pt(w, h))}
}

// fitInto shrinks r to fit in area and moves it inside.
func fitInto(r, area image.Rectangle) image.Rectangle {
	if area.Empty() {
		return r
	}
	w, h := min(r.Dx(), area.Dx()), min(r.Dy(), area.Dy())
	x := max(area.Min.X, min(r.Min.X, area.Max.X-w))
	y := max(area.Min.Y, min(r.Min.Y, area.Max.Y-h))
	return image.Rect(x, y, x+w, y+h)
}
