package player

import (
	"math"
	"strings"
	"testing"
)

func TestStereoFilter(t *testing.T) {
	if f := StereoFilter(Layout2D, ViewRedCyan); f != "" {
		t.Errorf("2D file got filter %q", f)
	}
	if f := StereoFilter(LayoutSBSHalf, ViewSideBySide); f != "" {
		t.Errorf("same layout in and out got filter %q", f)
	}
	if f, want := StereoFilter(LayoutSBSHalf, ViewRedCyan), "lavfi=[stereo3d=in=sbs2l:out=arcd]"; f != want {
		t.Errorf("got %q, want %q", f, want)
	}
}

func TestVRFilter(t *testing.T) {
	if f := VRFilter(ProjectionFlat, VRMono, DefaultVRView, 1920, 1080); f != "" {
		t.Errorf("flat video got filter %q", f)
	}
	f := VRFilter(ProjectionEquirect, VRTopBottom, VRView{Yaw: 370, Pitch: 120, FOV: 90}, 1921, 1080)
	for _, want := range []string{"input=e", "in_stereo=tb", "yaw=10.00", "pitch=90.00", "h_fov=90.00", "w=1920", "h=1080"} {
		if !strings.Contains(f, want) {
			t.Errorf("%q lacks %q", f, want)
		}
	}
}

func TestVerticalFOV(t *testing.T) {
	v := VRView{FOV: 90}
	if got := v.VerticalFOV(100, 100); math.Abs(got-90) > 1e-9 {
		t.Errorf("square output: %v, want 90", got)
	}
	if got := v.VerticalFOV(1920, 1080); got >= 90 || got < 50 {
		t.Errorf("16:9 output: %v, want narrower than 90", got)
	}
}

func TestDetectStereo(t *testing.T) {
	for _, c := range []struct {
		name string
		w, h int
		want Detected
	}{
		{"Avatar.2009.3D.HSBS.1080p.mkv", 1920, 1080, Detected{Layout: LayoutSBSHalf}},
		{"Movie 3D Full-SBS.mkv", 3840, 1080, Detected{Layout: LayoutSBSFull}},
		{"movie.sbs.mkv", 3840, 1080, Detected{Layout: LayoutSBSFull}},
		{"Movie.3D.H-OU.mkv", 1920, 1080, Detected{Layout: LayoutTBHalf}},
		{"movie_tab.mp4", 1920, 2160, Detected{Layout: LayoutTBFull}},
		{"Alps_360.mp4", 3840, 1920, Detected{Projection: ProjectionEquirect, VRStereo: VRMono}},
		{"Alps 360 TB.mp4", 3840, 3840, Detected{Projection: ProjectionEquirect, VRStereo: VRTopBottom}},
		{"concert_VR180_LR.mp4", 5760, 2880, Detected{Projection: ProjectionHalf, VRStereo: VRSideBySide}},
		{"Holiday 2019.mp4", 1920, 1080, Detected{}},
		{"Ocean's 11.mkv", 1920, 800, Detected{}},
	} {
		if got := DetectStereo(c.name, c.w, c.h); got != c.want {
			t.Errorf("DetectStereo(%q) = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestLanguageName(t *testing.T) {
	for code, want := range map[string]string{"eng": "English", "fre": "French", "fra": "French", "pt-BR": "Portuguese (BR)", "xx": "xx"} {
		if got := LanguageName(code); got != want {
			t.Errorf("LanguageName(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestFormatTime(t *testing.T) {
	for in, want := range map[float64]string{0: "0:00", 61.9: "1:01", 3600: "1:00:00", -5: "0:00", math.NaN(): "0:00"} {
		if got := FormatTime(in); got != want {
			t.Errorf("FormatTime(%v) = %q, want %q", in, got, want)
		}
	}
}
