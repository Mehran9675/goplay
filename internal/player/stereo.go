package player

import (
	"fmt"
	"math"
	"strings"
)

// 3D and VR video. Both are done with FFmpeg filters in mpv's filter
// chain: stereo3d converts between stereoscopic layouts and outputs
// (anaglyph, one eye, interleaved, ...), v360 projects 360° and 180°
// footage onto a flat view that the user steers.

// StereoLayout is how a stereoscopic file packs its two views (stereo3d
// input names).
type StereoLayout string

const (
	Layout2D            StereoLayout = ""
	LayoutSBSHalf       StereoLayout = "sbs2l" // side by side, squeezed, left eye first
	LayoutSBSHalfRight  StereoLayout = "sbs2r" // right eye first (cross-eyed)
	LayoutSBSFull       StereoLayout = "sbsl"
	LayoutSBSFullRight  StereoLayout = "sbsr"
	LayoutTBHalf        StereoLayout = "ab2l" // top and bottom, squeezed, left eye on top
	LayoutTBHalfRight   StereoLayout = "ab2r"
	LayoutTBFull        StereoLayout = "abl"
	LayoutTBFullRight   StereoLayout = "abr"
	LayoutAlternateLeft StereoLayout = "al" // frame sequential
)

// StereoView is how to show stereoscopic video (stereo3d output names).
type StereoView string

const (
	ViewLeft            StereoView = "ml" // 2D, left eye
	ViewRight           StereoView = "mr" // 2D, right eye
	ViewRedCyan         StereoView = "arcd"
	ViewRedCyanColor    StereoView = "arcc"
	ViewGreenMagenta    StereoView = "agmd"
	ViewYellowBlue      StereoView = "aybd"
	ViewSideBySide      StereoView = "sbs2l" // half side by side, for 3D TVs
	ViewTopBottom       StereoView = "ab2l"
	ViewInterleavedRows StereoView = "irl" // passive (polarized) 3D monitors
	ViewInterleavedCols StereoView = "icl"
	ViewCheckerboard    StereoView = "chl" // DLP 3D projectors
)

// StereoFilter returns the mpv video filter that shows a file packed as
// layout the way view asks, or "" when none is needed.
func StereoFilter(layout StereoLayout, view StereoView) string {
	if layout == Layout2D || view == "" || string(view) == string(layout) {
		return ""
	}
	return fmt.Sprintf("lavfi=[stereo3d=in=%s:out=%s]", layout, view)
}

// Projection is how a 360° or 180° file maps the sphere (v360 input
// names).
type Projection string

const (
	ProjectionFlat     Projection = ""
	ProjectionEquirect Projection = "e"         // 360° equirectangular
	ProjectionHalf     Projection = "hequirect" // 180° (VR180)
	ProjectionFisheye  Projection = "fisheye"
	ProjectionEAC      Projection = "eac" // equi-angular cubemap (YouTube)
	ProjectionCubemap  Projection = "c3x2"
)

// VRStereo is how a stereoscopic 360°/180° file packs its eyes (v360
// in_stereo names).
type VRStereo string

const (
	VRMono       VRStereo = "2d"
	VRSideBySide VRStereo = "sbs"
	VRTopBottom  VRStereo = "tb"
)

// VRView is where the viewer looks, in degrees: yaw turns right, pitch
// looks up, FOV is the horizontal field of view.
type VRView struct {
	Yaw, Pitch, Roll, FOV float64
}

// DefaultVRView looks straight ahead with a natural field of view.
var DefaultVRView = VRView{FOV: 100}

// Clamp keeps the view in range: yaw wraps, pitch stops at the poles,
// FOV stays between a narrow zoom and a wide angle.
func (v VRView) Clamp() VRView {
	v.Yaw = math.Mod(v.Yaw+540, 360) - 180
	v.Pitch = math.Max(-90, math.Min(90, v.Pitch))
	v.Roll = math.Mod(v.Roll+540, 360) - 180
	v.FOV = math.Max(30, math.Min(150, v.FOV))
	return v
}

// VerticalFOV is the vertical field of view for an output of w×h pixels.
func (v VRView) VerticalFOV(w, h int) float64 {
	if w <= 0 || h <= 0 {
		return v.FOV
	}
	half := v.FOV * math.Pi / 360
	return 2 * math.Atan(math.Tan(half)*float64(h)/float64(w)) * 180 / math.Pi
}

// VRFilter returns the mpv video filter projecting proj footage onto a
// flat w×h view, or "" for flat video.
func VRFilter(proj Projection, stereo VRStereo, view VRView, w, h int) string {
	if proj == ProjectionFlat {
		return ""
	}
	if stereo == "" {
		stereo = VRMono
	}
	view = view.Clamp()
	return fmt.Sprintf("lavfi=[v360=input=%s:output=flat:in_stereo=%s:out_stereo=2d:"+
		"yaw=%.2f:pitch=%.2f:roll=%.2f:h_fov=%.2f:v_fov=%.2f:w=%d:h=%d:interp=line]",
		proj, stereo, view.Yaw, view.Pitch, view.Roll, view.FOV, view.VerticalFOV(w, h), even(w), even(h))
}

// even rounds down to an even size, which video filters need.
func even(n int) int {
	if n < 2 {
		return 2
	}
	return n &^ 1
}

// Detected is what a file's name (and picture shape) suggests about it.
type Detected struct {
	Layout     StereoLayout
	Projection Projection
	VRStereo   VRStereo
}

// DetectStereo guesses a file's 3D layout or 360° projection from the
// usual naming conventions ("Movie.3D.HSBS.mkv", "clip_360_TB.mp4",
// "VR180") and its size: a side-by-side picture twice as wide as usual
// is full width, otherwise half.
func DetectStereo(name string, w, h int) Detected {
	tokens := map[string]bool{}
	for _, t := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		tokens[t] = true
	}
	has := func(ts ...string) bool {
		for _, t := range ts {
			if tokens[t] {
				return true
			}
		}
		return false
	}
	aspect := 0.0
	if h > 0 {
		aspect = float64(w) / float64(h)
	}

	sbs := has("sbs", "hsbs", "halfsbs", "fsbs", "fullsbs", "lr", "3dsbs")
	tb := has("tb", "tab", "htab", "ou", "hou", "overunder", "topbottom", "ftab", "fou", "3dou", "3dtab")

	switch {
	case has("180", "vr180", "180vr", "180x180"):
		d := Detected{Projection: ProjectionHalf, VRStereo: VRSideBySide}
		if tb {
			d.VRStereo = VRTopBottom
		}
		if has("fisheye") {
			d.Projection = ProjectionFisheye
		}
		return d
	case has("360", "vr360", "360vr", "equirect", "equirectangular", "eac"):
		d := Detected{Projection: ProjectionEquirect, VRStereo: VRMono}
		if has("eac") {
			d.Projection = ProjectionEAC
		}
		switch {
		case tb || (aspect > 0 && aspect < 1.5 && has("3d", "stereo")):
			d.VRStereo = VRTopBottom
		case sbs:
			d.VRStereo = VRSideBySide
		}
		return d
	case sbs:
		if has("fsbs", "fullsbs") || aspect >= 3 {
			return Detected{Layout: LayoutSBSFull}
		}
		return Detected{Layout: LayoutSBSHalf}
	case tb:
		if has("ftab", "fou") || (aspect > 0 && aspect <= 1) {
			return Detected{Layout: LayoutTBFull}
		}
		return Detected{Layout: LayoutTBHalf}
	}
	return Detected{}
}
