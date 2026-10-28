package app

import (
	"math"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// stereoState is how the current file's 3D or 360° picture is shown.
type stereoState struct {
	layout     player.StereoLayout
	view       player.StereoView
	projection player.Projection
	vrStereo   player.VRStereo
	vr         player.VRView
	// outW and outH are the 360° view's size in pixels.
	outW, outH int
	// pending marks a view change not yet sent to mpv (drags are sent at
	// most every vrInterval).
	pending  bool
	lastSent time.Time
	// live is whether the 360° filter takes angle changes as commands;
	// otherwise it's rebuilt for each change.
	live bool
	// dragging renders the 360° view at reduced size while the user
	// drags, so rebuilding it keeps up; full size returns on release.
	dragging bool
}

// vrInterval paces 360° view updates while dragging.
const vrInterval = 40 * time.Millisecond

func (a *App) vrActive() bool { return a.stereo.projection != player.ProjectionFlat }

func (a *App) stereoActive() bool { return a.stereo.layout != player.Layout2D }

// stereoFileLoaded resets the picture for a new file and, if enabled,
// recognizes 3D and 360° files by name.
func (a *App) stereoFileLoaded() {
	s := &a.stereo
	*s = stereoState{vr: player.DefaultVRView}
	s.vr.FOV = a.settings.Float(settings.KeyVRFOV)
	if a.settings.Bool(settings.KeyAuto3D) {
		st := &a.player.State
		d := player.DetectStereo(filepath.Base(st.Path)+" "+st.Title, st.VideoW, st.VideoH)
		switch {
		case d.Projection != player.ProjectionFlat:
			s.projection, s.vrStereo = d.Projection, d.VRStereo
			a.toast("360° video — drag to look around, scroll to zoom")
		case d.Layout != player.Layout2D:
			s.layout = d.Layout
			s.view = player.StereoView(a.settings.Get(settings.KeyStereoView))
			a.toast("3D video — change how it's shown in the 3D menu")
		}
	}
	a.player.SetFilter("goplay-3d", "")
	a.player.SetFilter("goplay-360", "")
	a.applyStereo()
}

// setStereoLayout says how the file packs its two views; Layout2D turns
// 3D handling off.
func (a *App) setStereoLayout(l player.StereoLayout) {
	a.stereo.layout = l
	a.stereo.projection = player.ProjectionFlat
	if a.stereo.view == "" {
		a.stereo.view = player.StereoView(a.settings.Get(settings.KeyStereoView))
	}
	a.applyStereo()
	if l == player.Layout2D {
		a.osd("3D off")
	} else {
		a.osd("3D on")
	}
}

// setStereoView chooses how 3D is shown (anaglyph, one eye, ...).
func (a *App) setStereoView(v player.StereoView) {
	a.stereo.view = v
	if a.stereo.layout == player.Layout2D {
		a.stereo.layout = player.LayoutSBSHalf
	}
	a.applyStereo()
}

// setProjection treats the file as 360° or 180° footage (or flat).
func (a *App) setProjection(p player.Projection, s player.VRStereo) {
	a.stereo.projection, a.stereo.vrStereo = p, s
	a.stereo.layout = player.Layout2D
	if a.stereo.vr.FOV == 0 {
		a.stereo.vr = player.DefaultVRView
		a.stereo.vr.FOV = a.settings.Float(settings.KeyVRFOV)
	}
	a.applyStereo()
	if p == player.ProjectionFlat {
		a.osd("Flat video")
	} else {
		a.osd("Drag to look around · scroll to zoom")
	}
}

func (a *App) resetVRView() {
	a.stereo.vr = player.DefaultVRView
	a.stereo.vr.FOV = a.settings.Float(settings.KeyVRFOV)
	a.applyStereo()
}

// applyStereo installs the 3D or 360° filter for the current state.
func (a *App) applyStereo() {
	s := &a.stereo
	if err := a.player.SetFilter("goplay-3d", player.StereoFilter(s.layout, s.view)); err != nil {
		a.toastError("3D: " + err.Error())
	}
	if s.projection == player.ProjectionFlat {
		a.player.SetFilter("goplay-360", "")
		return
	}
	w, h := a.vrOutputSize()
	s.outW, s.outH = w, h
	if err := a.player.SetFilter("goplay-360", player.VRFilter(s.projection, s.vrStereo, s.vr, w, h)); err != nil {
		a.toastError("360°: " + err.Error())
		return
	}
	s.live = true
	s.pending = false
	s.lastSent = time.Now()
}

// vrOutputSize is the 360° view's pixel size: the video area, capped so
// the projection stays cheap.
func (a *App) vrOutputSize() (int, int) {
	r := a.ui.videoRect
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		w, h = 1920, 1080
	}
	if w > 1920 {
		h = h * 1920 / w
		w = 1920
	}
	if a.stereo.dragging {
		w, h = w/2, h/2
	}
	return w, h
}

// vrDrag turns the view by a pointer drag of dx, dy pixels.
func (a *App) vrDrag(dx, dy float32) {
	s := &a.stereo
	w := float32(max(a.ui.videoRect.Dx(), 1))
	// One video-width of drag turns by the field of view.
	speed := float64(s.vr.FOV) / float64(w) * a.settings.Float(settings.KeyVRSensitivity)
	sign := -1.0
	if a.settings.Bool(settings.KeyVRInvert) {
		sign = 1
	}
	s.vr.Yaw += sign * float64(dx) * speed
	s.vr.Pitch -= sign * float64(dy) * speed
	s.vr = s.vr.Clamp()
	a.sendVRView()
}

// vrZoom narrows (positive) or widens the field of view.
func (a *App) vrZoom(steps float32) {
	s := &a.stereo
	s.vr.FOV *= math.Pow(0.92, float64(steps))
	s.vr = s.vr.Clamp()
	a.sendVRView()
	a.osd("Field of view " + strconv.Itoa(int(math.Round(s.vr.FOV))) + "°")
}

// sendVRView updates the 360° filter's angles, no more often than
// vrInterval; the last change is sent when the timer fires.
func (a *App) sendVRView() {
	s := &a.stereo
	if time.Since(s.lastSent) < vrInterval {
		s.pending = true
		return
	}
	s.pending = false
	s.lastSent = time.Now()
	if s.live && !s.dragging {
		vfov := s.vr.VerticalFOV(s.outW, s.outH)
		ok := true
		for _, c := range [][2]string{
			{"yaw", ftoa(s.vr.Yaw)}, {"pitch", ftoa(s.vr.Pitch)},
			{"h_fov", ftoa(s.vr.FOV)}, {"v_fov", ftoa(vfov)},
		} {
			if a.player.FilterCommand("goplay-360", c[0], c[1]) != nil {
				ok = false
				break
			}
		}
		if ok {
			return
		}
		// This FFmpeg can't change the angles live: rebuild instead.
		s.live = false
	}
	w, h := a.vrOutputSize()
	a.player.SetFilter("goplay-360", player.VRFilter(s.projection, s.vrStereo, s.vr, w, h))
}

// vrDragEnd restores full resolution after a drag.
func (a *App) vrDragEnd() {
	if !a.stereo.dragging {
		return
	}
	a.stereo.dragging = false
	a.stereo.pending = false
	a.applyStereo()
}

// vrTimer is when a paced view change is due.
func (a *App) vrTimer() time.Time {
	if !a.stereo.pending {
		return time.Time{}
	}
	return a.stereo.lastSent.Add(vrInterval)
}

// vrTick sends a pending view change once due.
func (a *App) vrTick() {
	if a.stereo.pending && time.Since(a.stereo.lastSent) >= vrInterval {
		a.sendVRView()
	}
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }
