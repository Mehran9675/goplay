// Package settings holds the user's preferences: what each one is (the
// Defs table, which the settings dialog is built from), how it maps to
// mpv options, and the Store that persists the chosen values as JSON in
// the user's config folder.
//
// Anything not covered here can still be set in mpv.conf, which mpv
// loads from the same folder (see ConfigDir).
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Kind is how a setting is edited.
type Kind uint8

const (
	// Toggle is on/off, stored as "yes"/"no" (mpv's flag syntax).
	Toggle Kind = iota
	// Choice picks one of Choices.
	Choice
	// Range is a number between Min and Max in steps of Step.
	Range
	// Text is free text; Lines > 1 makes it a multi-line field.
	Text
)

// Option is one value a Choice setting can take.
type Option struct {
	Label, Value string
}

// MPVOption is an mpv option name and value.
type MPVOption struct {
	Name, Value string
}

// Def describes a setting.
type Def struct {
	Key     string // storage key
	Section string // dialog tab
	Label   string
	// Help is a sentence under the label, for settings whose effect
	// isn't obvious from the name.
	Help    string
	Kind    Kind
	Default string
	Lines   int // Text: visible lines

	Options        []Option // Choice
	Min, Max, Step float64  // Range
	Unit           string   // Range: shown after the value, e.g. "%"

	// MPV is the mpv option the value is passed to unchanged; empty for
	// a setting the app handles itself or one with an MPVFunc.
	MPV string
	// MPVFunc maps the value to mpv options when it isn't a 1:1 option.
	MPVFunc func(value string) []MPVOption
	// Group joins this setting with others into one mpv option: each
	// member's GroupFunc returns a fragment (or ""), and the fragments
	// are joined with commas - the audio filter chain, say.
	Group     string
	GroupFunc func(value string) string
	// Restart marks a setting that only takes effect when goplay
	// starts (an option mpv reads once, at initialization).
	Restart bool
}

// MPVOptions is the mpv options the setting's value v sets.
func (d *Def) MPVOptions(v string) []MPVOption {
	switch {
	case d.MPVFunc != nil:
		return d.MPVFunc(v)
	case d.MPV != "":
		return []MPVOption{{d.MPV, v}}
	}
	return nil
}

// OptionIndex is the index of the Choice option with value v, or -1.
func (d *Def) OptionIndex(v string) int {
	for i, o := range d.Options {
		if o.Value == v {
			return i
		}
	}
	return -1
}

// Lookup finds a setting by key, or returns nil.
func Lookup(key string) *Def {
	for i := range Defs {
		if Defs[i].Key == key {
			return &Defs[i]
		}
	}
	return nil
}

// ConfigDir is goplay's folder in the user's config directory, which
// holds settings.json, mpv.conf and mpv's watch-later (resume) data.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goplay"), nil
}

// Store holds the current values. Changes are saved by Flush, which the
// app calls periodically and on exit, so dragging a slider doesn't
// rewrite the file every frame. It is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	path   string
	values map[string]string
	dirty  bool
}

// Load reads the settings saved at path. A missing file yields the
// defaults; values for unknown keys are kept, so settings saved by a
// newer goplay survive.
func Load(path string) (*Store, error) {
	s := &Store{path: path, values: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	// Editors such as Windows Notepad may save with a byte-order mark.
	data = bytes.TrimPrefix(data, utf8BOM)
	if err := json.Unmarshal(data, &s.values); err != nil {
		return s, err
	}
	return s, nil
}

// Path is where the store saves.
func (s *Store) Path() string { return s.path }

// Get returns the value of key, or its default.
func (s *Store) Get(key string) string {
	s.mu.Lock()
	v, ok := s.values[key]
	s.mu.Unlock()
	if ok {
		return v
	}
	if d := Lookup(key); d != nil {
		return d.Default
	}
	return ""
}

// Bool reports whether a Toggle setting is on.
func (s *Store) Bool(key string) bool { return s.Get(key) == "yes" }

// Float returns a numeric setting, or 0 if it doesn't parse.
func (s *Store) Float(key string) float64 {
	f, _ := strconv.ParseFloat(s.Get(key), 64)
	return f
}

// Set changes a value; Flush saves it. Setting a value back to its
// default removes it from the file, so later default changes apply.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d := Lookup(key); d != nil && d.Default == value {
		if _, ok := s.values[key]; ok {
			delete(s.values, key)
			s.dirty = true
		}
		return
	}
	if s.values[key] != value {
		s.values[key] = value
		s.dirty = true
	}
}

// Reset restores every setting in the dialog to its default; Flush
// saves it. Key bindings and remembered values (the volume) stay.
func (s *Store) Reset() {
	s.mu.Lock()
	for i := range Defs {
		delete(s.values, Defs[i].Key)
	}
	s.dirty = true
	s.mu.Unlock()
}

// Flush saves the values if they changed since the last save.
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.dirty || s.path == "" {
		s.mu.Unlock()
		return nil
	}
	data, err := json.MarshalIndent(s.values, "", "  ")
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	// Write then rename, so a crash mid-write can't corrupt the file.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// MPVOptions is every mpv option the current values set, in Defs order,
// then the grouped options: what to apply when mpv starts.
func (s *Store) MPVOptions() []MPVOption {
	var opts []MPVOption
	groups := map[string]bool{}
	for i := range Defs {
		if g := Defs[i].Group; g != "" {
			groups[g] = true
			continue
		}
		opts = append(opts, Defs[i].MPVOptions(s.Get(Defs[i].Key))...)
	}
	for g := range groups {
		opts = append(opts, s.GroupOption(g))
	}
	return opts
}

// GroupOption is the value of a grouped mpv option, built from every
// setting in the group.
func (s *Store) GroupOption(group string) MPVOption {
	var parts []string
	for i := range Defs {
		d := &Defs[i]
		if d.Group == group && d.GroupFunc != nil {
			if f := d.GroupFunc(s.Get(d.Key)); f != "" {
				parts = append(parts, f)
			}
		}
	}
	return MPVOption{group, strings.Join(parts, ",")}
}

// Keys returns the user's key bindings for an action, or nil when they
// haven't changed the defaults. Bindings are stored as "key.<action>",
// a comma-separated list of shortcuts ("Ctrl+O, F1"); "none" unbinds.
func (s *Store) Keys(action string) ([]string, bool) {
	s.mu.Lock()
	v, ok := s.values["key."+action]
	s.mu.Unlock()
	if !ok {
		return nil, false
	}
	if v == "none" {
		return []string{}, true
	}
	var out []string
	for _, k := range strings.Split(v, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out, true
}

// SetKeys stores the bindings for an action; nil restores the default.
func (s *Store) SetKeys(action string, keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := "key." + action
	switch {
	case keys == nil:
		delete(s.values, k)
	case len(keys) == 0:
		s.values[k] = "none"
	default:
		s.values[k] = strings.Join(keys, ", ")
	}
	s.dirty = true
}

// ResetKeys restores every default key binding.
func (s *Store) ResetKeys() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.values {
		if strings.HasPrefix(k, "key.") {
			delete(s.values, k)
			s.dirty = true
		}
	}
}

// utf8BOM is the byte-order mark some editors (Windows Notepad) write
// at the start of UTF-8 files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}
