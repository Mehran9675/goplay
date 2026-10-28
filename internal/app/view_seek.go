package app

import (
	"image"
	"image/color"
	"math"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/mehran9675/giode/components/kit"
	"github.com/mehran9675/giode/elements"

	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/giode/styles/properties"
	"github.com/mehran9675/goplay/internal/player"
)

// seekBar is the timeline: what's played, what's buffered, chapter
// marks and an A-B loop, with a preview of the time (and chapter) under
// the pointer. Dragging seeks to keyframes as it goes and lands exactly
// where it's released.
type seekBar struct {
	u        *ui
	drag     gesture.Drag
	hovered  bool
	hoverX   float32
	dragging bool
	dragFrac float64
	lastSeek time.Time
}

// seekInterval paces seeks while dragging.
const seekInterval = 60 * time.Millisecond

func (s *seekBar) Layout(gtx layout.Context) layout.Dimensions {
	a := s.u.a
	pos, dur := a.timeline()
	w := gtx.Constraints.Max.X
	h := gtx.Dp(22)
	size := image.Pt(w, h)
	pad := float32(gtx.Dp(6))
	span := float32(w) - 2*pad
	fracAt := func(x float32) float64 {
		return math.Max(0, math.Min(1, float64((x-pad)/span)))
	}

	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	s.drag.Add(gtx.Ops)
	if dur > 0 {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	kit.BlockDragHere(gtx)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Move | pointer.Enter | pointer.Leave | pointer.Cancel})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			switch e.Kind {
			case pointer.Leave, pointer.Cancel:
				s.hovered = false
			default:
				s.hovered = true
				s.hoverX = e.Position.X
			}
		}
	}
	for {
		e, ok := s.drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		if dur <= 0 {
			continue
		}
		s.hoverX = e.Position.X
		switch e.Kind {
		case pointer.Press:
			s.dragging = true
			s.dragFrac = fracAt(e.Position.X)
			a.seekFraction(s.dragFrac, false)
			s.lastSeek = time.Now()
		case pointer.Drag:
			s.dragFrac = fracAt(e.Position.X)
			if time.Since(s.lastSeek) >= seekInterval {
				a.seekFraction(s.dragFrac, false)
				s.lastSeek = time.Now()
			}
		case pointer.Release:
			s.dragFrac = fracAt(e.Position.X)
			a.seekFraction(s.dragFrac, true)
			s.dragging = false
		case pointer.Cancel:
			s.dragging = false
		}
	}
	area.Pop()
	if s.dragging {
		kit.MarkLive()
	}

	active := s.hovered || s.dragging
	trackH := float32(gtx.Dp(4))
	if active {
		trackH = float32(gtx.Dp(6))
	}
	y := (float32(h) - trackH) / 2
	x := func(t float64) float32 {
		if dur <= 0 {
			return pad
		}
		return pad + span*float32(math.Max(0, math.Min(1, t/dur)))
	}
	bar := func(x0, x1 float32, c color.NRGBA) {
		if x1 <= x0 {
			return
		}
		r := int(trackH / 2)
		rect := image.Rect(int(x0), int(y), int(math.Ceil(float64(x1))), int(y+trackH))
		paint.FillShape(gtx.Ops, c, clip.UniformRRect(rect, r).Op(gtx.Ops))
	}

	bar(pad, pad+span, rgba(0xffffff, 0x38))
	st := &a.player.State
	if !a.casting() {
		for _, r := range st.CacheRanges {
			bar(x(r[0]), x(r[1]), rgba(0xffffff, 0x5c))
		}
		if !math.IsNaN(st.ABLoopA) {
			end := dur
			if !math.IsNaN(st.ABLoopB) {
				end = st.ABLoopB
			}
			bar(x(st.ABLoopA), x(end), rgba(0xf5a524, 0x90))
		}
	}
	shown := pos
	if s.dragging {
		shown = s.dragFrac * dur
	}
	bar(pad, x(shown), rgba(0xffffff, 0xff))
	// Chapter marks: small gaps in the track.
	if !a.casting() && dur > 0 {
		gap := float32(gtx.Dp(2))
		for _, c := range st.Chapters {
			if c.Time <= 0.5 || c.Time >= dur-0.5 {
				continue
			}
			cx := x(c.Time)
			r := image.Rect(int(cx-gap/2), int(y-1), int(cx+gap/2), int(y+trackH+1))
			paint.FillShape(gtx.Ops, rgba(0x000000, 0xb0), clip.Rect(r).Op())
		}
	}
	if active && dur > 0 {
		r := float32(gtx.Dp(7))
		cx := x(shown)
		if !s.dragging {
			cx = max(pad, min(pad+span, s.hoverX))
		}
		c := image.Rect(int(cx-r), int(float32(h)/2-r), int(cx+r), int(float32(h)/2+r))
		paint.FillShape(gtx.Ops, rgba(0xffffff, 0xff), clip.Ellipse(c).Op(gtx.Ops))
		s.preview(gtx, fracAt(cx)*dur, cx)
	}
	return layout.Dimensions{Size: size}
}

// preview shows the time (and chapter) under the pointer above the bar.
func (s *seekBar) preview(gtx layout.Context, t float64, x float32) {
	label := player.FormatTime(t)
	if !s.u.a.casting() {
		st := &s.u.a.player.State
		if i := st.ChapterAt(t); i >= 0 && st.Chapters[i].Title != "" {
			label = st.Chapters[i].Title + " · " + label
		}
	}
	origin := elements.AbsolutePosition()
	anchor := image.Pt(origin.X+int(x)-gtx.Dp(40), origin.Y-gtx.Dp(38))
	elements.RegisterPortal(gtx, anchor, func() elements.Element {
		return elements.Box(styles.Styles{
			Background:   colSurface2,
			BorderRadius: 6,
			Padding:      properties.SymmetricInset(10, 5),
			BoxShadow:    properties.BoxShadow{Y: 2, Blur: 8, Color: "#00000099"},
		}, elements.Text(label, styles.Styles{Color: colText, FontSize: 12}))
	})
}

// timeline is the position and duration to show: of the cast device
// while casting, of local playback otherwise.
func (a *App) timeline() (pos, dur float64) {
	if a.casting() {
		return a.cast.status.Position, a.cast.status.Duration
	}
	return a.player.State.Position, a.player.State.Duration
}

// seekFraction seeks to a fraction of the duration; exact lands on the
// exact frame.
func (a *App) seekFraction(f float64, exact bool) {
	if a.casting() {
		if exact {
			a.castSeek(f * a.cast.status.Duration)
		}
		return
	}
	a.player.SeekFraction(f, exact)
}

func rgba(rgb uint32, alpha uint8) color.NRGBA {
	return color.NRGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: alpha}
}
