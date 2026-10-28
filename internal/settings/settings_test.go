package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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
		if d.Group != "" && d.GroupFunc == nil {
			t.Errorf("%s: group without GroupFunc", d.Key)
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
	s.Set(KeyVolume, "40")
	s.SetKeys("open", []string{"Ctrl+O", "F2"})
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
	if got := s2.Float(KeyVolume); got != 40 {
		t.Errorf("reloaded volume = %v, want 40 (keys without a Def must persist)", got)
	}
	if keys, ok := s2.Keys("open"); !ok || !reflect.DeepEqual(keys, []string{"Ctrl+O", "F2"}) {
		t.Errorf("reloaded keys = %v, %v", keys, ok)
	}

	// Back to the default: dropped from the file.
	s2.Set(KeySeekStep, "5")
	if _, ok := s2.values[KeySeekStep]; ok {
		t.Error("default value still stored")
	}

	// Reset clears settings but keeps bindings and remembered values.
	s2.Set(KeySeekStep, "15")
	s2.Reset()
	if got := s2.Get(KeySeekStep); got != "5" {
		t.Errorf("after Reset, seek step = %q", got)
	}
	if got := s2.Get(KeyVolume); got != "40" {
		t.Errorf("after Reset, volume = %q, want kept", got)
	}
	if _, ok := s2.Keys("open"); !ok {
		t.Error("Reset dropped key bindings")
	}
	s2.SetKeys("open", []string{})
	if keys, ok := s2.Keys("open"); !ok || len(keys) != 0 {
		t.Errorf("unbound action keys = %v, %v", keys, ok)
	}
	s2.ResetKeys()
	if _, ok := s2.Keys("open"); ok {
		t.Error("ResetKeys kept a binding")
	}
}

func TestMPVOptions(t *testing.T) {
	s, _ := Load("")
	s.Set("interpolation", "yes")
	s.Set("normalize", "yes")
	s.Set("equalizer", "bass")
	got := map[string]string{}
	for _, o := range s.MPVOptions() {
		got[o.Name] = o.Value
	}
	for name, want := range map[string]string{
		"interpolation": "yes", "video-sync": "display-resample",
		"hwdec": "auto-safe", "save-position-on-quit": "yes", "keep-open": "yes",
	} {
		if got[name] != want {
			t.Errorf("%s = %q, want %q", name, got[name], want)
		}
	}
	if af := got["af"]; !strings.Contains(af, "@goplay-norm:") || !strings.Contains(af, "@goplay-eq:") || strings.Contains(af, "night") {
		t.Errorf("af = %q", af)
	}
	if _, ok := got[KeySeekStep]; ok {
		t.Error("app-only setting passed to mpv")
	}
	if _, ok := got["sub-font"]; ok {
		t.Error("empty font passed to mpv")
	}
}

func TestParseOptions(t *testing.T) {
	got := ParseOptions("# comment\n--deband-iterations=2\n\n  sub-scale = 1.2 \nfullscreen\n")
	want := []MPVOption{{"deband-iterations", "2"}, {"sub-scale", "1.2"}, {"fullscreen", "yes"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestLoadBOM reads a file saved with a UTF-8 byte-order mark, as
// Windows Notepad writes them.
func TestLoadBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, "{\"seek_step\": \"10\"}"...), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get(KeySeekStep); got != "10" {
		t.Errorf("seek step = %q, want 10", got)
	}
}
