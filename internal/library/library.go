// Package library keeps the recently played list and finds the media
// files in a folder.
package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

// MediaExts are the file extensions goplay treats as media when
// scanning folders.
var MediaExts = map[string]bool{
	".mp4": true, ".m4v": true, ".mkv": true, ".mov": true, ".avi": true, ".webm": true,
	".ts": true, ".m2ts": true, ".mts": true, ".flv": true, ".wmv": true, ".mpg": true,
	".mpeg": true, ".ogv": true, ".3gp": true, ".vob": true, ".divx": true, ".rmvb": true,
	".mp3": true, ".flac": true, ".wav": true, ".ogg": true, ".oga": true, ".opus": true,
	".m4a": true, ".aac": true, ".mka": true, ".wma": true, ".alac": true, ".ape": true,
}

// SubtitleExts are subtitle file extensions.
var SubtitleExts = map[string]bool{
	".srt": true, ".ass": true, ".ssa": true, ".vtt": true, ".sub": true, ".idx": true,
	".sup": true, ".smi": true, ".lrc": true,
}

// IsMedia reports whether path has a media file extension.
func IsMedia(path string) bool { return MediaExts[strings.ToLower(filepath.Ext(path))] }

// IsSubtitle reports whether path has a subtitle file extension.
func IsSubtitle(path string) bool { return SubtitleExts[strings.ToLower(filepath.Ext(path))] }

// IsURL reports whether s looks like a URL rather than a path.
func IsURL(s string) bool {
	i := strings.Index(s, "://")
	return i > 1 && !strings.ContainsAny(s[:i], `\/.`)
}

// Folder lists the media files in dir, naturally sorted.
func Folder(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && IsMedia(e.Name()) && !strings.HasPrefix(e.Name(), ".") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	slices.SortFunc(files, func(a, b string) int {
		switch {
		case NaturalLess(a, b):
			return -1
		case NaturalLess(b, a):
			return 1
		}
		return 0
	})
	if len(files) == 0 {
		return nil, errors.New("no video or audio files in " + filepath.Base(dir))
	}
	return files, nil
}

// Expand turns what the user opened - files, folders, URLs - into a
// playlist, expanding folders to their media files. With queueFolder, a
// single file opened alone brings the rest of its folder along; start is
// then its index in the list.
func Expand(items []string, queueFolder bool) (list []string, start int) {
	for _, it := range items {
		if IsURL(it) {
			list = append(list, it)
			continue
		}
		if fi, err := os.Stat(it); err == nil && fi.IsDir() {
			if files, err := Folder(it); err == nil {
				list = append(list, files...)
			}
			continue
		}
		list = append(list, it)
	}
	if queueFolder && len(items) == 1 && len(list) == 1 && !IsURL(list[0]) {
		if files, err := Folder(filepath.Dir(list[0])); err == nil && len(files) > 1 {
			for i, f := range files {
				if sameFile(f, list[0]) {
					return files, i
				}
			}
		}
	}
	return list, 0
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// NaturalLess orders strings as people expect: digit runs compare by
// value ("ep2" before "ep10"), letters case-insensitively.
func NaturalLess(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		ca, cb := unicode.ToLower(ar[i]), unicode.ToLower(br[j])
		if ca != cb {
			return ca < cb
		}
		i++
		j++
	}
	return len(ar)-i < len(br)-j
}

// Recent is one recently played file or URL.
type Recent struct {
	Path     string    `json:"path"`
	Title    string    `json:"title,omitempty"`
	Position float64   `json:"position,omitempty"`
	Duration float64   `json:"duration,omitempty"`
	Played   time.Time `json:"played"`
}

// Progress is how far through it playback got, from 0 to 1.
func (r Recent) Progress() float64 {
	if r.Duration <= 0 {
		return 0
	}
	return min(1, max(0, r.Position/r.Duration))
}

// Name is the title, or the file name.
func (r Recent) Name() string {
	if r.Title != "" {
		return r.Title
	}
	if IsURL(r.Path) {
		return r.Path
	}
	return filepath.Base(r.Path)
}

// maxRecent is how many entries the list keeps.
const maxRecent = 30

// History is the recently played list, saved as JSON. It is safe for
// concurrent use.
type History struct {
	mu    sync.Mutex
	path  string
	items []Recent
}

// LoadHistory reads the list saved at path; a missing file is empty.
func LoadHistory(path string) (*History, error) {
	h := &History{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return h, nil
	}
	if err != nil {
		return h, err
	}
	return h, json.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &h.items)
}

// Items returns the list, most recent first.
func (h *History) Items() []Recent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.items)
}

// Played moves path to the top of the list, updating its details.
func (h *History) Played(r Recent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.Played.IsZero() {
		r.Played = time.Now()
	}
	h.items = slices.DeleteFunc(h.items, func(x Recent) bool { return x.Path == r.Path })
	h.items = slices.Insert(h.items, 0, r)
	if len(h.items) > maxRecent {
		h.items = h.items[:maxRecent]
	}
}

// Update records the position reached in path, without reordering.
func (h *History) Update(path, title string, position, duration float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.items {
		if h.items[i].Path == path {
			if title != "" {
				h.items[i].Title = title
			}
			h.items[i].Position, h.items[i].Duration = position, duration
			return
		}
	}
}

// Remove drops one entry; Clear drops them all.
func (h *History) Remove(path string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = slices.DeleteFunc(h.items, func(x Recent) bool { return x.Path == path })
}

func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.items = nil
}

// Save writes the list.
func (h *History) Save() error {
	if h.path == "" {
		return nil
	}
	h.mu.Lock()
	data, err := json.MarshalIndent(h.items, "", "  ")
	h.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}

// utf8BOM is the byte-order mark some editors (Windows Notepad) write
// at the start of UTF-8 files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}
