package library

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNaturalLess(t *testing.T) {
	in := []string{"Ep10.mkv", "ep2.mkv", "Ep1.mkv", "ep02b.mkv", "Bonus.mkv", "ep2a.mkv"}
	slices.SortFunc(in, func(a, b string) int {
		if NaturalLess(a, b) {
			return -1
		}
		if NaturalLess(b, a) {
			return 1
		}
		return 0
	})
	want := []string{"Bonus.mkv", "Ep1.mkv", "ep2.mkv", "ep2a.mkv", "ep02b.mkv", "Ep10.mkv"}
	if !slices.Equal(in, want) {
		t.Errorf("got %v, want %v", in, want)
	}
}

func TestExpand(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"b 10.mkv", "b 2.mp4", "notes.txt", "a.srt", ".hidden.mkv"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o644)
	}
	list, start := Expand([]string{filepath.Join(dir, "b 10.mkv")}, true)
	if len(list) != 2 || filepath.Base(list[0]) != "b 2.mp4" || start != 1 {
		t.Errorf("queue folder: %v start %d", list, start)
	}
	list, start = Expand([]string{filepath.Join(dir, "b 10.mkv")}, false)
	if len(list) != 1 || start != 0 {
		t.Errorf("single: %v start %d", list, start)
	}
	list, _ = Expand([]string{dir, "https://example.com/v"}, true)
	if len(list) != 3 || list[2] != "https://example.com/v" {
		t.Errorf("folder + url: %v", list)
	}
}

func TestIsURL(t *testing.T) {
	for s, want := range map[string]bool{
		"https://youtu.be/x": true, "rtsp://cam/1": true, "C:\\videos\\a.mkv": false,
		"/home/a.mkv": false, "file.mkv": false, "ytdl://abc": true,
	} {
		if got := IsURL(s); got != want {
			t.Errorf("IsURL(%q) = %v", s, got)
		}
	}
}

func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	h, _ := LoadHistory(path)
	h.Played(Recent{Path: "a"})
	h.Played(Recent{Path: "b"})
	h.Played(Recent{Path: "a", Title: "A"})
	h.Update("b", "", 30, 60)
	if err := h.Save(); err != nil {
		t.Fatal(err)
	}
	h2, err := LoadHistory(path)
	if err != nil {
		t.Fatal(err)
	}
	items := h2.Items()
	if len(items) != 2 || items[0].Name() != "A" || items[1].Progress() != 0.5 {
		t.Errorf("items = %+v", items)
	}
	for i := range 40 {
		h2.Played(Recent{Path: string(rune('a' + i))})
	}
	if n := len(h2.Items()); n != maxRecent {
		t.Errorf("kept %d, want %d", n, maxRecent)
	}
}
