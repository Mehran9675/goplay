package app

import (
	"context"
	"sync"
	"time"

	"github.com/mehran9675/goplay/internal/cast"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// castState is device discovery and the active cast.
type castState struct {
	scanMu     sync.Mutex
	scanCancel context.CancelFunc
	scanning   bool
	devices    []cast.Device

	session    *cast.Session
	device     string // name of the device being cast to
	connecting string // name of the device being connected to
	status     cast.Status
	path       string // what's being cast
}

func (a *App) casting() bool { return a.cast.session != nil }

// startCastScan looks for devices for a while; results arrive through
// Post.
func (a *App) startCastScan() {
	c := &a.cast
	c.scanMu.Lock()
	if c.scanCancel != nil {
		c.scanCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	c.scanCancel = cancel
	c.scanMu.Unlock()
	c.scanning = true
	for _, d := range c.devices {
		d.Close()
	}
	c.devices = nil
	go func() {
		cast.Discover(ctx, func(d cast.Device) {
			a.Post(func() {
				c.devices = append(c.devices, d)
				cast.SortDevices(c.devices)
			})
		})
		a.Post(func() { c.scanning = false })
	}()
}

func (a *App) stopCastScan() {
	c := &a.cast
	c.scanMu.Lock()
	if c.scanCancel != nil {
		c.scanCancel()
		c.scanCancel = nil
	}
	c.scanMu.Unlock()
	c.scanning = false
}

// castTo starts playing the current file on d, from where it is now.
func (a *App) castTo(d cast.Device) {
	st := &a.player.State
	if st.Idle {
		a.toast(tr("Open a video first, then cast it"))
		return
	}
	a.stopCastScan()
	if a.casting() {
		a.stopCasting()
	}
	item := a.castItem()
	a.cast.connecting = d.Name()
	a.player.SetPause(true)
	go func() {
		s, err := cast.Start(context.Background(), d, item, func(status cast.Status) {
			a.Post(func() { a.castStatus(status) })
		})
		a.Post(func() {
			a.cast.connecting = ""
			if err != nil {
				a.toastError(tr("Couldn't cast to {device}: {error}", "device", d.Name(), "error", err))
				return
			}
			a.cast.session = s
			a.cast.device = d.Name()
			a.cast.path = item.Path
			a.cast.status = cast.Status{State: cast.StateLoading, Position: item.Start, Duration: item.Duration, Volume: -1}
			msg := tr("Casting to {device}", "device", d.Name())
			if s.Converted() {
				msg += " (converting with FFmpeg)"
			}
			a.toast(msg)
			a.afterPlayerChange()
		})
	}()
}

// castItem describes what's playing for a cast device.
func (a *App) castItem() cast.Item {
	st := &a.player.State
	item := cast.Item{
		Path:         st.Path,
		Title:        st.Title,
		Start:        st.Position,
		Duration:     st.Duration,
		AllowConvert: a.settings.Bool(settings.KeyCastConvert),
	}
	if v, ok := st.SelectedTrack(player.Video); ok {
		item.VideoCodec = v.Codec
	}
	if au, ok := st.SelectedTrack(player.Audio); ok {
		item.AudioCodec = au.Codec
	}
	if a.settings.Bool(settings.KeyCastSubtitles) {
		if s, ok := st.SelectedTrack(player.Subtitle); ok {
			item.SubtitleLang = s.Lang
			if s.External {
				item.Subtitles = s.ExternalFilename
			} else {
				item.Subtitles = "embedded"
				// The index among the file's own subtitle streams.
				n := 0
				for _, t := range st.TracksOf(player.Subtitle) {
					if t.ID == s.ID {
						break
					}
					if !t.External {
						n++
					}
				}
				item.SubtitleIndex = n
			}
		}
	}
	return item
}

// castCurrent sends a newly opened file to the device being cast to.
func (a *App) castCurrent() {
	if !a.casting() || a.player.State.Path == a.cast.path {
		return
	}
	d := a.cast.session.Device
	a.cast.session.Stop()
	a.cast.session = nil
	a.castTo(d)
}

func (a *App) castStatus(s cast.Status) {
	if !a.casting() {
		return
	}
	if s.Volume < 0 {
		s.Volume = a.cast.status.Volume
	}
	a.cast.status = s
	if s.State == cast.StateStopped && s.Duration > 0 && s.Position >= s.Duration-2 {
		// Finished on the TV: move on like local playback would.
		if len(a.player.State.Playlist) > a.player.State.PlaylistPos+1 {
			a.player.PlaylistNext()
			a.Post(a.castCurrent)
		}
	}
}

// stopCasting ends the cast and continues here from where the device
// got to.
func (a *App) stopCasting() {
	if !a.casting() {
		return
	}
	s := a.cast.session
	pos := a.cast.status.Position
	a.cast.session = nil
	name := a.cast.device
	go s.Stop()
	if a.player.State.Path == a.cast.path && pos > 0 {
		a.player.Seek(pos, false)
	}
	a.toast(tr("Stopped casting to {device}", "device", name))
	a.afterPlayerChange()
}

func (a *App) castTogglePause() {
	s := a.cast.session
	if a.cast.status.State == cast.StatePlaying || a.cast.status.State == cast.StateBuffering {
		a.cast.status.State = cast.StatePaused
		go s.Pause()
	} else {
		a.cast.status.State = cast.StatePlaying
		go s.Play()
	}
}

func (a *App) castSeek(t float64) {
	t = max(0, t)
	if d := a.cast.status.Duration; d > 0 {
		t = min(t, d)
	}
	a.cast.status.Position = t
	s := a.cast.session
	go s.Seek(t)
}

func (a *App) castVolume(v float64) {
	a.cast.status.Volume = v
	s := a.cast.session
	go s.SetVolume(v)
}
