package settings

import (
	"path/filepath"
	"strconv"
	"testing"
)

// TestDefs checks the table is consistent: unique keys, known sections,
// and defaults that are valid values.
func TestDefs(t *testing.T) {
	seen := map[string]bool{}
	sections := map[string]bool{}
	for _, s := range Sections {
		sections[s] = true
	}
	for _, d := range Defs {
		if seen[d.Key] {
			t.Errorf("duplicate key %q", d.Key)
		}
		seen[d.Key] = true
		if !sections[d.Section] {
			t.Errorf("%s: unknown section %q", d.Key, d.Section)
		}
		switch d.Kind {
		case Toggle:
			if d.Default != "yes" && d.Default != "no" {
				t.Errorf("%s: toggle default %q", d.Key, d.Default)
			}
		case Choice:
			if d.OptionIndex(d.Default) < 0 {
				t.Errorf("%s: default %q is not an option", d.Key, d.Default)
			}
		case Range:
			v, err := strconv.ParseFloat(d.Default, 64)
			if err != nil || v < d.Min || v > d.Max || d.Step <= 0 {
				t.Errorf("%s: bad range default %q in [%v, %v] step %v", d.Key, d.Default, d.Min, d.Max, d.Step)
			}
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "goplay", "settings.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(KeySeekStep); got != "5" {
		t.Fatalf("default seek step = %q, want 5", got)
	}
	s.Set(KeySeekStep, "10")
	s.Set(KeyVolume, "0.4")
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Float(KeySeekStep); got != 10 {
		t.Errorf("reloaded seek step = %v, want 10", got)
	}
	if got := s2.Float(KeyVolume); got != 0.4 {
		t.Errorf("reloaded volume = %v, want 0.4 (keys without a Def must persist)", got)
	}

	// Back to the default: dropped from the file.
	s2.Set(KeySeekStep, "5")
	if _, ok := s2.values[KeySeekStep]; ok {
		t.Error("default value still stored")
	}

	s2.Reset()
	if got := s2.Get(KeyVolume); got != "" {
		t.Errorf("after Reset, volume = %q", got)
	}
}

func TestMPVOptions(t *testing.T) {
	s, _ := Load("")
	s.Set("interpolation", "yes")
	got := map[string]string{}
	for _, o := range s.MPVOptions() {
		got[o.Name] = o.Value
	}
	for name, want := range map[string]string{
		"interpolation": "yes", "video-sync": "display-resample",
		"hwdec": "auto-safe", "save-position-on-quit": "yes", "config": "yes",
	} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	if _, ok := got[KeySeekStep]; ok {
		t.Error("app-only setting passed to mpv")
	}
}
