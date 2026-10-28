// Package cast plays media on network devices: Chromecasts (Google Cast
// protocol) and DLNA/UPnP media renderers (smart TVs, game consoles,
// receivers). It discovers devices on the local network, serves local
// files to them over HTTP (converting what they can't play with FFmpeg),
// and controls playback.
package cast

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// State is a device's playback state.
type State string

const (
	StateIdle      State = "idle"
	StateLoading   State = "loading"
	StatePlaying   State = "playing"
	StatePaused    State = "paused"
	StateBuffering State = "buffering"
	StateStopped   State = "stopped"
)

// Status is what a device reports.
type Status struct {
	State              State
	Position, Duration float64
	Volume             float64 // 0 to 1, -1 when unknown
	Muted              bool
}

// Media is what to play on a device.
type Media struct {
	URL         string // reachable by the device
	Title       string
	ContentType string  // MIME type
	Start       float64 // seconds
	Duration    float64 // seconds, when known
	// Subtitles is a WebVTT URL, or "".
	Subtitles    string
	SubtitleLang string
}

// Device is a cast target.
type Device interface {
	ID() string
	Name() string
	// Kind is "Chromecast" or "DLNA".
	Kind() string
	// Model describes the hardware, when the device says.
	Model() string

	Load(ctx context.Context, m Media) error
	Play() error
	Pause() error
	Seek(seconds float64) error
	Stop() error
	SetVolume(level float64) error
	// Status asks the device for its current state.
	Status(ctx context.Context) (Status, error)
	Close() error
}

// ErrUnsupported is returned for controls a device doesn't offer.
var ErrUnsupported = errors.New("cast: not supported by this device")

// Discover looks for devices of every supported kind until ctx ends,
// calling found (from a background goroutine) for each new one.
func Discover(ctx context.Context, found func(Device)) {
	var (
		mu   sync.Mutex
		seen = map[string]bool{}
	)
	report := func(d Device) {
		mu.Lock()
		defer mu.Unlock()
		if seen[d.ID()] {
			d.Close()
			return
		}
		seen[d.ID()] = true
		found(d)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); discoverChromecasts(ctx, report) }()
	go func() { defer wg.Done(); discoverDLNA(ctx, report) }()
	wg.Wait()
}

// SortDevices orders devices by name.
func SortDevices(ds []Device) {
	sort.Slice(ds, func(i, j int) bool {
		return strings.ToLower(ds[i].Name()) < strings.ToLower(ds[j].Name())
	})
}

// requestTimeout bounds every network request to a device.
const requestTimeout = 8 * time.Second
