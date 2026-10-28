package mpv

import (
	"os"
	"testing"
	"time"
)

// TestPlayback loads a real file with no video output and checks events,
// observed properties and JSON list properties. It needs a media file:
// set GOPLAY_TEST_MEDIA to one with chapters and subtitle tracks.
func TestPlayback(t *testing.T) {
	path := os.Getenv("GOPLAY_TEST_MEDIA")
	if path == "" {
		t.Skip("GOPLAY_TEST_MEDIA not set")
	}
	m, err := Create()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Destroy()
	for _, o := range [][2]string{{"vo", "null"}, {"ao", "null"}, {"config", "no"}, {"pause", "yes"}} {
		if err := m.SetOption(o[0], o[1]); err != nil {
			t.Fatal(o[0], err)
		}
	}
	if err := m.Initialize(); err != nil {
		t.Fatal(err)
	}
	woke := make(chan struct{}, 1)
	m.SetWakeupCallback(func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	})
	if err := m.Observe(1, "duration", FormatDouble); err != nil {
		t.Fatal(err)
	}
	if err := m.Observe(2, "track-list", FormatNone); err != nil {
		t.Fatal(err)
	}
	if err := m.Command("loadfile", path); err != nil {
		t.Fatal(err)
	}

	var duration float64
	loaded, tracksChanged := false, false
	deadline := time.After(10 * time.Second)
	for !loaded || duration == 0 || !tracksChanged {
		select {
		case <-woke:
		case <-deadline:
			t.Fatalf("timed out: loaded=%v duration=%v tracks=%v", loaded, duration, tracksChanged)
		}
		for {
			ev := m.Wait(0)
			if ev.ID == EventNone {
				break
			}
			switch {
			case ev.ID == EventFileLoaded:
				loaded = true
			case ev.ID == EventPropertyChange && ev.Name == "duration" && ev.Format == FormatDouble:
				duration = ev.Double
			case ev.ID == EventPropertyChange && ev.Name == "track-list":
				tracksChanged = true
			}
		}
	}
	if duration < 1 {
		t.Errorf("duration = %v", duration)
	}

	var tracks []struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
		Lang string `json:"lang"`
	}
	if err := m.JSON("track-list", &tracks); err != nil {
		t.Fatal(err)
	}
	subs := 0
	for _, tr := range tracks {
		if tr.Type == "sub" {
			subs++
		}
	}
	if subs < 1 {
		t.Errorf("track-list = %+v, want subtitle tracks", tracks)
	}
	if err := m.SetDouble("volume", 42); err != nil {
		t.Fatal(err)
	}
	if v, ok := m.Double("volume"); !ok || v != 42 {
		t.Errorf("volume = %v, %v", v, ok)
	}
}
