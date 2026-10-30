package player

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestPlayer starts a player with no audio or video output. It needs
// GOPLAY_TEST_MEDIA_DIR: a folder with the test videos (see the README's
// Testing section).
func newTestPlayer(t *testing.T) (*Player, string) {
	dir := os.Getenv("GOPLAY_TEST_MEDIA_DIR")
	if dir == "" {
		t.Skip("GOPLAY_TEST_MEDIA_DIR not set")
	}
	woke := make(chan struct{}, 1)
	p, err := New(Config{
		Options: []Option{{"vo", "null"}, {"ao", "null"}, {"pause", "yes"}},
		Wakeup: func() {
			select {
			case woke <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Destroy)
	go func() {
		for range time.Tick(20 * time.Millisecond) {
			select {
			case woke <- struct{}{}:
			default:
			}
		}
	}()
	p.wake = woke
	return p, dir
}

// waitFor processes events until cond holds.
func (p *Player) waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case <-p.wake:
			p.Process()
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func TestStereoFilterApplies(t *testing.T) {
	p, dir := newTestPlayer(t)
	if err := p.Open([]string{filepath.Join(dir, "sbs_half_3d.mp4")}, false); err != nil {
		t.Fatal(err)
	}
	loaded := false
	p.OnFileLoaded = func() { loaded = true }
	p.waitFor(t, "file to load", func() bool { return loaded })

	p.Set("hwdec", "auto-safe")
	if err := p.SetFilter("goplay-3d", StereoFilter(LayoutSBSHalf, ViewRedCyan)); err != nil {
		t.Fatal(err)
	}
	vf, _ := p.Get("vf")
	if !strings.Contains(vf, "stereo3d") {
		t.Fatalf("vf = %q", vf)
	}
	// Frames must actually go through it: the filter's output is
	// half as wide (two half-width views become one), and mpv drops a
	// filter that fails.
	p.SetPause(false)
	p.waitFor(t, "filtered frames", func() bool {
		w, _ := p.h.Int("video-out-params/w")
		return w == 960
	})
	if hw, _ := p.Get("hwdec"); hw != "auto-copy" {
		t.Errorf("hwdec during filtering = %q, want auto-copy", hw)
	}
	if err := p.SetFilter("goplay-3d", ""); err != nil {
		t.Fatal(err)
	}
	if vf, _ := p.Get("vf"); strings.Contains(vf, "stereo3d") {
		t.Fatalf("filter not removed: vf = %q", vf)
	}
	if hw, _ := p.Get("hwdec"); hw != "auto-safe" {
		t.Errorf("hwdec after removing the filter = %q, want auto-safe restored", hw)
	}
}

func TestVRFilterCommands(t *testing.T) {
	p, dir := newTestPlayer(t)
	if err := p.Open([]string{filepath.Join(dir, "vr360_equirect.mp4")}, false); err != nil {
		t.Fatal(err)
	}
	loaded := false
	p.OnFileLoaded = func() { loaded = true }
	p.waitFor(t, "file to load", func() bool { return loaded })

	if err := p.SetFilter("goplay-360", VRFilter(ProjectionEquirect, VRMono, DefaultVRView, 1280, 720)); err != nil {
		t.Fatal(err)
	}
	if err := p.FilterCommand("goplay-360", "yaw", "45"); err != nil {
		t.Logf("v360 doesn't take live commands here (%v); goplay rebuilds the filter instead", err)
	}
}

func TestPlaylistAndTracks(t *testing.T) {
	p, dir := newTestPlayer(t)
	files := []string{filepath.Join(dir, "chapters.mkv"), filepath.Join(dir, "h264_1080p.mp4")}
	if err := p.Open(files, false); err != nil {
		t.Fatal(err)
	}
	p.waitFor(t, "tracks and chapters", func() bool {
		return len(p.State.Tracks) > 0 && len(p.State.Chapters) == 3 && len(p.State.Playlist) == 2
	})
	if subs := p.State.TracksOf(Subtitle); len(subs) != 2 {
		t.Errorf("subtitle tracks = %+v", subs)
	}
	// FFmpeg versions round chapter times slightly differently.
	if c := p.State.Chapters[1]; c.Title != "Middle part" || math.Abs(c.Time-15) > 0.1 {
		t.Errorf("chapter 2 = %+v", c)
	}
	p.SelectTrack(Subtitle, 2)
	p.waitFor(t, "subtitle 2 selected", func() bool {
		tr, ok := p.State.SelectedTrack(Subtitle)
		return ok && tr.ID == 2
	})
	p.SetSpeed(1.5)
	p.waitFor(t, "speed", func() bool { return p.State.Speed == 1.5 })
	p.PlaylistNext()
	p.waitFor(t, "second file", func() bool { return p.State.PlaylistPos == 1 })
}

// A track picked in one file mustn't carry over: the next file's audio
// plays even when the last one's was turned off.
func TestTrackChoiceIsPerFile(t *testing.T) {
	p, dir := newTestPlayer(t)
	files := []string{filepath.Join(dir, "chapters.mkv"), filepath.Join(dir, "h264_1080p.mp4")}
	if err := p.Open(files, false); err != nil {
		t.Fatal(err)
	}
	p.waitFor(t, "audio", func() bool {
		_, ok := p.State.SelectedTrack(Audio)
		return ok && len(p.State.Playlist) == 2
	})
	p.SelectTrack(Audio, 0)
	p.waitFor(t, "audio off", func() bool {
		_, ok := p.State.SelectedTrack(Audio)
		return !ok
	})
	p.PlaylistNext()
	p.waitFor(t, "audio in the second file", func() bool {
		_, ok := p.State.SelectedTrack(Audio)
		return p.State.PlaylistPos == 1 && ok
	})
}
