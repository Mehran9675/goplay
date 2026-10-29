// Package player drives an mpv core for goplay: it configures mpv,
// mirrors the properties the UI shows into State, and offers typed
// commands for everything the UI does. It knows nothing about windows or
// widgets; the app renders the video (mpv.RenderContext) and draws the
// UI over it.
//
// Threading: Process and every command are called on the UI goroutine.
// mpv's own threads only call the wakeup function given to New, which
// must just wake that goroutine.
package player

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mehran9675/goplay/internal/mpv"
)

// Option is an mpv option name and value.
type Option struct{ Name, Value string }

// baseOptions are what goplay itself needs from mpv, set before the
// user's settings (which come after and can override most of these).
var baseOptions = []Option{
	// Video goes through the render API, drawn by the app.
	{"vo", "libmpv"},
	// The UI is goplay's own: no on-screen controller or OSD bar, and
	// no keyboard handling inside mpv (keys are forwarded on purpose).
	{"osc", "no"},
	{"osd-bar", "no"},
	{"input-default-bindings", "no"},
	{"input-vo-keyboard", "no"},
	// Stay on the last frame at the end of the last file instead of
	// going idle, so the picture doesn't vanish and Play replays.
	{"keep-open", "yes"},
	{"idle", "yes"},
	{"screenshot-template", "GoPlay-%F-%wH.%wM.%wS"},
	// Built-in scripts that goplay's own UI replaces. Older mpv
	// versions lack some of these options; they're skipped there.
	{"load-console", "no"},
	{"load-select", "no"},
	{"load-positioning", "no"},
	{"load-commands", "no"},
	{"load-context-menu", "no"},
}

// Config configures a Player.
type Config struct {
	// Options are mpv options applied after goplay's base options,
	// typically from the user's settings.
	Options []Option
	// ConfigDir is where mpv reads mpv.conf, input.conf and scripts, and
	// writes resume data. Empty disables config files.
	ConfigDir string
	// LoadConfig loads mpv.conf, input.conf and scripts from ConfigDir.
	LoadConfig bool
	// Wakeup is called from mpv's threads when events are waiting. It
	// must only wake the UI goroutine, which then calls Process.
	Wakeup func()
	// LogLevel is mpv's log level for Log ("warn" if empty).
	LogLevel string
}

// Player is an mpv core with the state the UI shows.
type Player struct {
	h     *mpv.Handle
	State State

	// OnFileLoaded runs when a file starts playing; OnEndFile when one
	// ends, with the reason (mpv.EndEOF, mpv.EndError, ...) and error.
	OnFileLoaded func()
	OnEndFile    func(reason int, err error)
	// OnLog receives mpv's log messages at the configured level.
	OnLog func(level, prefix, text string)

	filters map[string]string // labeled video filters goplay manages
	// hwdecSaved is the user's hardware decoding mode while goplay's
	// filters need decoded frames in system memory ("" when not
	// switched).
	hwdecSaved string
	// audioDevice is the output device the user chose ("auto" for the
	// default); devicesSeen is set once mpv has sent its device list.
	// audioTrack is the audio track playing, or last playing before mpv
	// gave up on its output; 0 when the user turned audio off.
	audioDevice string
	devicesSeen bool
	audioTrack  int

	wake chan struct{} // tests only: woken with the core
}

// New creates and starts the mpv core.
func New(cfg Config) (*Player, error) {
	h, err := mpv.Create()
	if err != nil {
		return nil, err
	}
	p := &Player{h: h, filters: map[string]string{}}
	p.State.ABLoopA, p.State.ABLoopB = math.NaN(), math.NaN()
	p.State.Idle = true
	p.State.Speed = 1
	p.State.Chapter = -1
	p.State.PlaylistPos = -1

	base := append([]Option(nil), baseOptions...)
	if pics := picturesDir(); pics != "" {
		base = append(base, Option{"screenshot-dir", pics})
	}
	if cfg.ConfigDir != "" {
		base = append(base,
			Option{"config-dir", cfg.ConfigDir},
			Option{"watch-later-dir", filepath.Join(cfg.ConfigDir, "watch_later")},
		)
	}
	base = append(base, Option{"config", yesNo(cfg.LoadConfig && cfg.ConfigDir != "")})
	for _, o := range base {
		if err := h.SetOption(o.Name, o.Value); err != nil && o.Name == "vo" {
			h.Destroy()
			return nil, fmt.Errorf("mpv: %s=%s: %w", o.Name, o.Value, err)
		}
	}
	p.audioDevice = "auto"
	var bad []string
	for _, o := range cfg.Options {
		if o.Name == "audio-device" {
			p.audioDevice = o.Value
		}
		if err := h.SetOption(o.Name, o.Value); err != nil {
			// A bad saved value must not stop the player from starting:
			// mpv keeps its default and the caller is told.
			bad = append(bad, fmt.Sprintf("%s=%s: %v", o.Name, o.Value, err))
		}
	}
	if err := h.Initialize(); err != nil {
		h.Destroy()
		return nil, err
	}
	level := cfg.LogLevel
	if level == "" {
		level = "warn"
	}
	h.RequestLog(level)
	for i, o := range observed {
		h.Observe(uint64(i+1), o.name, o.format)
	}
	if cfg.Wakeup != nil {
		h.SetWakeupCallback(cfg.Wakeup)
	}
	if len(bad) > 0 {
		return p, &OptionError{Options: bad}
	}
	return p, nil
}

// OptionError lists options mpv rejected at startup. The player still
// works; those options have their defaults.
type OptionError struct{ Options []string }

func (e *OptionError) Error() string {
	return "mpv rejected some settings: " + strings.Join(e.Options, "; ")
}

// Handle is the underlying mpv core, for rendering and for commands the
// Player doesn't wrap.
func (p *Player) Handle() *mpv.Handle { return p.h }

// Process handles all waiting mpv events, updating State and running
// the callbacks. It reports whether anything changed.
func (p *Player) Process() bool {
	changed := false
	for {
		ev := p.h.Wait(0)
		switch ev.ID {
		case mpv.EventNone:
			return changed
		case mpv.EventPropertyChange:
			if i := int(ev.Reply) - 1; i >= 0 && i < len(observed) {
				observed[i].apply(p, &ev)
				changed = true
			}
		case mpv.EventStartFile:
			p.State.Loading = true
			p.audioTrack = 0
			changed = true
		case mpv.EventFileLoaded:
			p.State.Loading = false
			changed = true
			// The property updates for the new file can still be queued
			// behind this event: read what the callback needs now.
			p.State.Path, _ = p.h.String("path")
			p.State.Title, _ = p.h.String("media-title")
			p.State.Duration, _ = p.h.Double("duration")
			p.State.Tracks = nil
			p.h.JSON("track-list", &p.State.Tracks)
			p.noteAudioTrack()
			if p.OnFileLoaded != nil {
				p.OnFileLoaded()
			}
		case mpv.EventEndFile:
			changed = true
			if ev.EndReason != mpv.EndRedirect {
				p.State.Loading = false
			}
			if p.OnEndFile != nil {
				p.OnEndFile(ev.EndReason, ev.Error)
			}
		case mpv.EventLogMessage:
			if p.OnLog != nil {
				p.OnLog(ev.LogLevel, ev.LogPrefix, ev.LogText)
			}
		case mpv.EventShutdown:
			return true
		}
	}
}

// Destroy stops mpv. Free the render context first.
func (p *Player) Destroy() { p.h.Destroy() }

// Opening.

// Open plays files and URLs, replacing the playlist, or appends them.
func (p *Player) Open(items []string, appendToPlaylist bool) error {
	if len(items) == 0 {
		return nil
	}
	mode := "replace"
	if appendToPlaylist {
		mode = "append-play"
	}
	for i, it := range items {
		if err := p.h.Command("loadfile", it, mode); err != nil {
			return fmt.Errorf("open %s: %w", it, err)
		}
		if i == 0 {
			mode = "append"
			if appendToPlaylist {
				mode = "append-play"
			}
		}
	}
	return nil
}

// Stop ends playback and clears the playlist.
func (p *Player) Stop() { p.h.Command("stop") }

// Playback.

// TogglePause pauses or resumes; at the end of the file it replays.
func (p *Player) TogglePause() {
	if p.State.EOF && p.State.Paused {
		p.h.Command("seek", "0", "absolute")
		p.h.SetFlag("pause", false)
		return
	}
	p.h.Command("cycle", "pause")
}

// SetPause pauses or resumes.
func (p *Player) SetPause(paused bool) { p.h.SetFlag("pause", paused) }

// Seek jumps to t seconds; exact seeks land on the exact frame (slower).
func (p *Player) Seek(t float64, exact bool) {
	mode := "absolute+keyframes"
	if exact {
		mode = "absolute+exact"
	}
	p.h.Command("seek", ftoa(t), mode)
}

// SeekBy moves by d seconds.
func (p *Player) SeekBy(d float64) { p.h.Command("seek", ftoa(d), "relative") }

// SeekFraction jumps to a fraction [0, 1] of the duration.
func (p *Player) SeekFraction(f float64, exact bool) {
	if p.State.Duration > 0 {
		p.Seek(math.Max(0, math.Min(1, f))*p.State.Duration, exact)
	}
}

// FrameStep advances one frame, or goes back one; both pause.
func (p *Player) FrameStep(back bool) {
	if back {
		p.h.Command("frame-back-step")
	} else {
		p.h.Command("frame-step")
	}
}

// SetSpeed sets the playback speed (1 is normal).
func (p *Player) SetSpeed(s float64) { p.h.SetDouble("speed", math.Max(0.01, math.Min(100, s))) }

// Chapters.

// ChapterBy moves d chapters (negative goes back).
func (p *Player) ChapterBy(d int) { p.h.Command("add", "chapter", strconv.Itoa(d)) }

// SetChapter jumps to chapter i.
func (p *Player) SetChapter(i int) { p.h.SetString("chapter", strconv.Itoa(i)) }

// A-B loop and repeat.

// CycleABLoop sets point A, then B, then clears the loop.
func (p *Player) CycleABLoop() { p.h.Command("ab-loop") }

// ClearABLoop removes the A-B loop.
func (p *Player) ClearABLoop() {
	p.h.SetString("ab-loop-a", "no")
	p.h.SetString("ab-loop-b", "no")
}

// SetLoopFile repeats the current file.
func (p *Player) SetLoopFile(on bool) { p.h.SetString("loop-file", infOrNo(on)) }

// SetLoopPlaylist repeats the playlist.
func (p *Player) SetLoopPlaylist(on bool) { p.h.SetString("loop-playlist", infOrNo(on)) }

// Audio.

// SetVolume sets the volume in percent (up to volume-max).
func (p *Player) SetVolume(v float64) {
	max := p.State.VolumeMax
	if max <= 0 {
		max = 100
	}
	p.h.SetDouble("volume", math.Max(0, math.Min(max, v)))
}

// SetMute mutes or unmutes.
func (p *Player) SetMute(m bool) { p.h.SetFlag("mute", m) }

// SetAudioDelay sets the audio delay in seconds (positive delays it).
func (p *Player) SetAudioDelay(d float64) { p.h.SetDouble("audio-delay", d) }

// Tracks.

// SelectTrack plays track id of kind; 0 turns the kind off.
func (p *Player) SelectTrack(k TrackKind, id int) {
	if k == Audio {
		p.audioTrack = id
	}
	p.h.SetString(trackProperty(k), idOrNo(id))
}

// SelectSecondarySubtitle shows track id as a second subtitle line; 0
// turns it off.
func (p *Player) SelectSecondarySubtitle(id int) { p.h.SetString("secondary-sid", idOrNo(id)) }

// AddSubtitle loads an external subtitle file and shows it.
func (p *Player) AddSubtitle(path string) error {
	return p.h.Command("sub-add", path, "select")
}

// AddAudio loads an external audio track and plays it.
func (p *Player) AddAudio(path string) error {
	return p.h.Command("audio-add", path, "select")
}

// SetSubDelay sets the subtitle delay in seconds.
func (p *Player) SetSubDelay(d float64) { p.h.SetDouble("sub-delay", d) }

// SubStep shows the next (or previous) subtitle line by seeking to it.
func (p *Player) SubStep(d int) { p.h.Command("sub-seek", strconv.Itoa(d)) }

func trackProperty(k TrackKind) string {
	switch k {
	case Video:
		return "vid"
	case Audio:
		return "aid"
	}
	return "sid"
}

// Playlist.

// PlaylistNext and PlaylistPrev move through the playlist.
func (p *Player) PlaylistNext() { p.h.Command("playlist-next") }
func (p *Player) PlaylistPrev() { p.h.Command("playlist-prev") }

// PlaylistPlay plays entry i.
func (p *Player) PlaylistPlay(i int) { p.h.Command("playlist-play-index", strconv.Itoa(i)) }

// PlaylistRemove removes entry i.
func (p *Player) PlaylistRemove(i int) { p.h.Command("playlist-remove", strconv.Itoa(i)) }

// PlaylistMove moves entry from to before entry to.
func (p *Player) PlaylistMove(from, to int) {
	p.h.Command("playlist-move", strconv.Itoa(from), strconv.Itoa(to))
}

// PlaylistShuffle shuffles the playlist.
func (p *Player) PlaylistShuffle() { p.h.Command("playlist-shuffle") }

// PlaylistClear removes every entry but the playing one.
func (p *Player) PlaylistClear() { p.h.Command("playlist-clear") }

// Picture.

// Screenshot saves a screenshot to the screenshot folder: "subtitles"
// includes subtitles as shown, "video" the bare frame, "window" the
// scaled picture.
func (p *Player) Screenshot(mode string) error { return p.h.Command("screenshot", mode) }

// SetAspect forces a display aspect ratio ("16:9"); "" uses the file's.
func (p *Player) SetAspect(ratio string) {
	if ratio == "" {
		ratio = "no"
	}
	p.h.SetString("video-aspect-override", ratio)
}

// Fit modes for SetFit.
const (
	FitContain = "contain" // whole picture, letterboxed
	FitCover   = "cover"   // fill the window, cropping
	FitStretch = "stretch" // fill the window, distorting
)

// SetFit chooses how the picture fills the window.
func (p *Player) SetFit(mode string) {
	switch mode {
	case FitCover:
		p.h.SetString("keepaspect", "yes")
		p.h.SetDouble("panscan", 1)
	case FitStretch:
		p.h.SetString("keepaspect", "no")
		p.h.SetDouble("panscan", 0)
	default:
		p.h.SetString("keepaspect", "yes")
		p.h.SetDouble("panscan", 0)
	}
}

// SetZoom zooms the picture: 0 is none, 1 doubles it (log2 scale).
func (p *Player) SetZoom(z float64) { p.h.SetDouble("video-zoom", z) }

// SetPan moves the zoomed picture, in fractions of its size.
func (p *Player) SetPan(x, y float64) {
	p.h.SetDouble("video-pan-x", x)
	p.h.SetDouble("video-pan-y", y)
}

// SetRotation rotates the picture by a multiple of 90 degrees.
func (p *Player) SetRotation(deg int) {
	p.h.SetString("video-rotate", strconv.Itoa(((deg%360)+360)%360))
}

// SetFlip mirrors the picture horizontally and/or vertically.
func (p *Player) SetFlip(h, v bool) {
	var f []string
	if h {
		f = append(f, "hflip")
	}
	if v {
		f = append(f, "vflip")
	}
	filter := ""
	if len(f) > 0 {
		filter = "lavfi=[" + strings.Join(f, ",") + "]"
	}
	p.SetFilter("goplay-flip", filter)
}

// SetFilter installs (or with "" removes) a video filter under a label,
// leaving other filters - the user's own from mpv.conf - alone.
//
// FFmpeg's filters work on frames in system memory, so while any of
// goplay's filters is active, hardware decoding switches to its
// copy-back mode (still decoding on the GPU), and returns to the user's
// mode once they're all removed.
func (p *Player) SetFilter(label, filter string) error {
	if p.filters[label] == filter {
		return nil
	}
	if _, ok := p.filters[label]; ok {
		p.h.Command("vf", "remove", "@"+label)
		delete(p.filters, label)
	}
	if filter != "" {
		p.copyBackHwdec(true)
		if err := p.h.Command("vf", "add", "@"+label+":"+filter); err != nil {
			p.copyBackHwdec(len(p.filters) > 0)
			return err
		}
		p.filters[label] = filter
	}
	p.copyBackHwdec(len(p.filters) > 0)
	return nil
}

// copyBackHwdec switches hardware decoding to copy-back mode, or back.
func (p *Player) copyBackHwdec(on bool) {
	switch {
	case on && p.hwdecSaved == "":
		cur, _ := p.h.String("hwdec")
		if cur == "" || cur == "no" || strings.HasSuffix(cur, "-copy") {
			return
		}
		p.hwdecSaved = cur
		p.h.SetString("hwdec", "auto-copy")
	case !on && p.hwdecSaved != "":
		p.h.SetString("hwdec", p.hwdecSaved)
		p.hwdecSaved = ""
	}
}

// FilterCommand sends a runtime command to a labeled filter, e.g. new
// angles to the 360° projection without rebuilding it.
func (p *Player) FilterCommand(label, cmd, arg string) error {
	return p.h.Command("vf-command", label, cmd, arg)
}

// Generic access, for settings and power users.

// Set sets any mpv property or option at runtime.
func (p *Player) Set(name, value string) error { return p.h.SetString(name, value) }

// Get reads any mpv property as a string.
func (p *Player) Get(name string) (string, bool) { return p.h.String(name) }

// Command runs any mpv command.
func (p *Player) Command(args ...string) error { return p.h.Command(args...) }

// Keypress forwards a key to mpv, so bindings in input.conf and scripts
// work. name uses mpv's key names ("ctrl+a", "F1", "KP_ENTER").
func (p *Player) Keypress(name string) { p.h.Command("keypress", name) }

// SaveResumePosition stores the current file's position so it resumes
// there next time (mpv saves on quit by itself).
func (p *Player) SaveResumePosition() {
	if !p.State.Idle && !p.State.EOF {
		p.h.Command("write-watch-later-config")
	}
}

// MediaInfo collects technical details of what's playing, for the
// statistics panel: name and value pairs in display order.
func (p *Player) MediaInfo() [][2]string {
	get := func(name string) string {
		v, _ := p.h.String(name)
		return v
	}
	var out [][2]string
	add := func(label, value string) {
		if value != "" && value != "no" {
			out = append(out, [2]string{label, value})
		}
	}
	add("File", get("filename"))
	add("Format", get("file-format"))
	if size, ok := p.h.Int("file-size"); ok {
		add("Size", humanBytes(size))
	}
	add("Video codec", get("video-codec"))
	if w, ok := p.h.Int("video-params/w"); ok {
		h, _ := p.h.Int("video-params/h")
		add("Resolution", fmt.Sprintf("%d × %d", w, h))
	}
	add("Pixel format", get("video-params/pixelformat"))
	add("Color", strings.Trim(get("video-params/colormatrix")+" "+get("video-params/primaries")+" "+get("video-params/gamma"), " "))
	if fps, ok := p.h.Double("container-fps"); ok {
		add("Frame rate", fmt.Sprintf("%.3f fps", fps))
	}
	if fps, ok := p.h.Double("estimated-display-fps"); ok {
		add("Display", fmt.Sprintf("%.2f Hz", fps))
	}
	if br, ok := p.h.Int("video-bitrate"); ok && br > 0 {
		add("Video bitrate", fmt.Sprintf("%d kbps", br/1000))
	}
	add("Hardware decoding", get("hwdec-current"))
	add("Audio codec", get("audio-codec-name"))
	if ch, rate := get("audio-params/hr-channels"), get("audio-params/samplerate"); ch != "" && rate != "" {
		add("Audio", ch+" "+rate+" Hz")
	}
	if br, ok := p.h.Int("audio-bitrate"); ok && br > 0 {
		add("Audio bitrate", fmt.Sprintf("%d kbps", br/1000))
	}
	add("Audio output", get("current-ao"))
	if d, ok := p.h.Int("frame-drop-count"); ok {
		add("Dropped frames", strconv.FormatInt(d, 10))
	}
	if d, ok := p.h.Int("decoder-frame-drop-count"); ok {
		add("Decoder drops", strconv.FormatInt(d, 10))
	}
	if c, ok := p.h.Double("demuxer-cache-duration"); ok {
		add("Buffered", fmt.Sprintf("%.1f s", c))
	}
	add("Video filters", get("vf"))
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func infOrNo(b bool) string {
	if b {
		return "inf"
	}
	return "no"
}

func idOrNo(id int) string {
	if id <= 0 {
		return "no"
	}
	return strconv.Itoa(id)
}

// picturesDir is where screenshots go: the user's Pictures folder, or
// the home folder.
func picturesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if dir := filepath.Join(home, "Pictures"); isDir(dir) {
		return dir
	}
	return home
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// ErrNotLoaded is returned by commands that need a file.
var ErrNotLoaded = errors.New("nothing is playing")

// FormatTime formats seconds as m:ss or h:mm:ss.
func FormatTime(secs float64) string {
	if math.IsNaN(secs) || secs < 0 {
		secs = 0
	}
	d := time.Duration(secs * float64(time.Second))
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
