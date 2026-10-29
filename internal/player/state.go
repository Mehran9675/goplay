package player

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/mehran9675/goplay/internal/mpv"
)

// TrackKind is a track type, named as mpv names it.
type TrackKind string

const (
	Video    TrackKind = "video"
	Audio    TrackKind = "audio"
	Subtitle TrackKind = "sub"
)

// Track is one video, audio or subtitle track of the current file.
type Track struct {
	ID               int       `json:"id"`
	Kind             TrackKind `json:"type"`
	Title            string    `json:"title"`
	Lang             string    `json:"lang"`
	Codec            string    `json:"codec"`
	Selected         bool      `json:"selected"`
	Default          bool      `json:"default"`
	Forced           bool      `json:"forced"`
	External         bool      `json:"external"`
	ExternalFilename string    `json:"external-filename"`
	AlbumArt         bool      `json:"albumart"`
	Width            int       `json:"demux-w"`
	Height           int       `json:"demux-h"`
	FPS              float64   `json:"demux-fps"`
	Channels         int       `json:"demux-channel-count"`
	SampleRate       int       `json:"demux-samplerate"`
	// MainSelection is mpv's index of the selection slot this track is in:
	// 0 for the primary track, 1 for the secondary subtitle.
	MainSelection int `json:"main-selection"`
}

// Name is a readable label: the title (or file name of an external
// track), the codec and size for video, and the language.
func (t Track) Name() string {
	name := t.Title
	if name == "" && t.ExternalFilename != "" {
		name = baseName(t.ExternalFilename)
	}
	if name == "" {
		switch {
		case t.Kind == Video && t.Width > 0:
			name = strings.ToUpper(t.Codec) + " " + strconv.Itoa(t.Width) + "×" + strconv.Itoa(t.Height)
		case t.Kind == Audio && t.Channels > 0:
			name = strings.ToUpper(t.Codec) + " " + channelName(t.Channels)
		default:
			name = "Track " + strconv.Itoa(t.ID)
		}
	}
	if t.Lang != "" {
		name += " · " + LanguageName(t.Lang)
	}
	if t.Forced {
		name += " (forced)"
	}
	return name
}

func channelName(n int) string {
	switch n {
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	return strconv.Itoa(n) + "ch"
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Chapter is a chapter of the current file.
type Chapter struct {
	Title string  `json:"title"`
	Time  float64 `json:"time"`
}

// PlaylistEntry is one entry of mpv's playlist.
type PlaylistEntry struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Current  bool   `json:"current"`
}

// Name is the entry's title, or its file name.
func (e PlaylistEntry) Name() string {
	if e.Title != "" {
		return e.Title
	}
	return baseName(strings.TrimRight(e.Filename, `/\`))
}

// State mirrors the player's observed properties. It is updated on the
// UI goroutine by Player.Process and read freely there.
type State struct {
	Path  string
	Title string
	// Idle is true with nothing loaded.
	Idle bool
	// Loading is true from opening a file until it starts playing.
	Loading bool

	Duration, Position float64
	Paused, Buffering  bool
	EOF, Seekable      bool
	// CacheAhead is how many seconds are buffered ahead of the position.
	CacheAhead float64
	// CacheRanges are the buffered, seekable time ranges (streams).
	CacheRanges [][2]float64

	Volume, VolumeMax float64 // percent
	Muted             bool
	Speed             float64

	Tracks       []Track
	Chapters     []Chapter
	Chapter      int // -1 before the first chapter or without chapters
	Playlist     []PlaylistEntry
	PlaylistPos  int
	VideoW       int
	VideoH       int
	ABLoopA      float64 // NaN when unset
	ABLoopB      float64
	LoopFile     bool
	LoopPlaylist bool
	SubDelay     float64
	AudioDelay   float64
	Hwdec        string
}

// HasVideo reports whether the file has a (non-cover-art) video track
// playing.
func (s *State) HasVideo() bool {
	for _, t := range s.Tracks {
		if t.Kind == Video && t.Selected && !t.AlbumArt {
			return true
		}
	}
	return false
}

// TracksOf returns the tracks of one kind.
func (s *State) TracksOf(k TrackKind) []Track {
	var out []Track
	for _, t := range s.Tracks {
		if t.Kind == k {
			out = append(out, t)
		}
	}
	return out
}

// SelectedTrack returns the primary selected track of kind k, if any.
func (s *State) SelectedTrack(k TrackKind) (Track, bool) {
	for _, t := range s.Tracks {
		if t.Kind == k && t.Selected && t.MainSelection == 0 {
			return t, true
		}
	}
	return Track{}, false
}

// SecondarySubtitle returns the secondary subtitle track, if any.
func (s *State) SecondarySubtitle() (Track, bool) {
	for _, t := range s.Tracks {
		if t.Kind == Subtitle && t.Selected && t.MainSelection == 1 {
			return t, true
		}
	}
	return Track{}, false
}

// ChapterAt returns the index of the chapter containing t, or -1.
func (s *State) ChapterAt(t float64) int {
	idx := -1
	for i, c := range s.Chapters {
		if c.Time <= t {
			idx = i
		}
	}
	return idx
}

// observed lists the properties the player follows, the format each is
// delivered in, and how it updates State. Lists are observed without a
// format and re-read as JSON when they change.
var observed = []struct {
	name   string
	format mpv.Format
	apply  func(p *Player, ev *mpv.Event)
}{
	{"path", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.Path = ev.String }},
	{"media-title", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.Title = ev.String }},
	{"idle-active", mpv.FormatFlag, func(p *Player, ev *mpv.Event) {
		p.State.Idle = ev.Flag
		if ev.Flag {
			p.State.Loading = false
		}
	}},
	{"duration", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.Duration = ev.Double }},
	{"time-pos", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.Position = ev.Double }},
	{"pause", mpv.FormatFlag, func(p *Player, ev *mpv.Event) { p.State.Paused = ev.Flag }},
	{"paused-for-cache", mpv.FormatFlag, func(p *Player, ev *mpv.Event) { p.State.Buffering = ev.Flag }},
	{"eof-reached", mpv.FormatFlag, func(p *Player, ev *mpv.Event) { p.State.EOF = ev.Flag }},
	{"seekable", mpv.FormatFlag, func(p *Player, ev *mpv.Event) { p.State.Seekable = ev.Flag }},
	{"demuxer-cache-duration", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.CacheAhead = ev.Double }},
	{"demuxer-cache-state", mpv.FormatNone, func(p *Player, ev *mpv.Event) { p.readCacheRanges() }},
	{"volume", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.Volume = ev.Double }},
	{"volume-max", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.VolumeMax = ev.Double }},
	{"mute", mpv.FormatFlag, func(p *Player, ev *mpv.Event) { p.State.Muted = ev.Flag }},
	{"speed", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.Speed = ev.Double }},
	{"track-list", mpv.FormatNone, func(p *Player, ev *mpv.Event) {
		p.State.Tracks = nil
		p.h.JSON("track-list", &p.State.Tracks)
		p.noteAudioTrack()
	}},
	{"audio-device-list", mpv.FormatNone, func(p *Player, ev *mpv.Event) { p.audioDevicesChanged() }},
	{"chapter-list", mpv.FormatNone, func(p *Player, ev *mpv.Event) {
		p.State.Chapters = nil
		p.h.JSON("chapter-list", &p.State.Chapters)
	}},
	{"chapter", mpv.FormatInt64, func(p *Player, ev *mpv.Event) { p.State.Chapter = intOr(ev, -1) }},
	{"playlist", mpv.FormatNone, func(p *Player, ev *mpv.Event) {
		p.State.Playlist = nil
		p.h.JSON("playlist", &p.State.Playlist)
	}},
	{"playlist-pos", mpv.FormatInt64, func(p *Player, ev *mpv.Event) { p.State.PlaylistPos = intOr(ev, -1) }},
	{"dwidth", mpv.FormatInt64, func(p *Player, ev *mpv.Event) { p.State.VideoW = intOr(ev, 0) }},
	{"dheight", mpv.FormatInt64, func(p *Player, ev *mpv.Event) { p.State.VideoH = intOr(ev, 0) }},
	{"ab-loop-a", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.ABLoopA = parseTime(ev.String) }},
	{"ab-loop-b", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.ABLoopB = parseTime(ev.String) }},
	{"loop-file", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.LoopFile = isLooping(ev.String) }},
	{"loop-playlist", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.LoopPlaylist = isLooping(ev.String) }},
	{"sub-delay", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.SubDelay = ev.Double }},
	{"audio-delay", mpv.FormatDouble, func(p *Player, ev *mpv.Event) { p.State.AudioDelay = ev.Double }},
	{"hwdec-current", mpv.FormatString, func(p *Player, ev *mpv.Event) { p.State.Hwdec = ev.String }},
}

func intOr(ev *mpv.Event, def int) int {
	if ev.Format != mpv.FormatInt64 {
		return def
	}
	return int(ev.Int)
}

// parseTime parses an A-B loop point; "no" and unset give NaN.
func parseTime(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

func isLooping(s string) bool { return s != "" && s != "no" }

// readCacheRanges reads the buffered ranges from demuxer-cache-state.
func (p *Player) readCacheRanges() {
	var st struct {
		Ranges []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"seekable-ranges"`
	}
	p.State.CacheRanges = p.State.CacheRanges[:0]
	s, ok := p.h.String("demuxer-cache-state")
	if !ok || json.Unmarshal([]byte(s), &st) != nil {
		return
	}
	for _, r := range st.Ranges {
		p.State.CacheRanges = append(p.State.CacheRanges, [2]float64{r.Start, r.End})
	}
}
