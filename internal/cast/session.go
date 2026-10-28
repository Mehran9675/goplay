package cast

import (
	"bufio"
	"context"
	"errors"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Item is what to cast: a local file or a URL, with what goplay knows
// about it.
type Item struct {
	Path     string // file path or URL
	Title    string
	Start    float64
	Duration float64
	// VideoCodec and AudioCodec (FFmpeg names) decide whether a local
	// file needs converting.
	VideoCodec, AudioCodec string
	// AllowConvert lets the session convert files the device can't
	// play, with FFmpeg.
	AllowConvert bool
	// Subtitles is an external subtitle file, "embedded" for stream
	// SubtitleIndex of the file, or "" for none.
	Subtitles     string
	SubtitleIndex int
	SubtitleLang  string
}

// Session is media playing on a device.
type Session struct {
	Device Device

	server    *Server
	media     Media
	converted bool
	// offset is where a converted stream started: such streams restart
	// at the seek point, so the device counts from zero again.
	offset float64

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
}

type addresser interface{ Addr() string }

// Start casts item to d. onStatus is called from a background goroutine
// about once a second with the device's state (positions already in the
// file's time).
func Start(ctx context.Context, d Device, item Item, onStatus func(Status)) (*Session, error) {
	s := &Session{Device: d}
	m := Media{Title: item.Title, Start: item.Start, Duration: item.Duration, SubtitleLang: item.SubtitleLang}

	if isURL(item.Path) {
		u, err := ResolveURL(ctx, item.Path)
		if err != nil {
			return nil, err
		}
		m.URL, m.ContentType = u, guessURLType(u)
	} else {
		addr := ""
		if a, ok := d.(addresser); ok {
			addr = a.Addr()
		}
		srv, err := NewServer(addr)
		if err != nil {
			return nil, err
		}
		s.server = srv
		convert := item.AllowConvert && NeedsConversion(item.Path, item.VideoCodec, item.AudioCodec)
		if convert {
			if _, err := exec.LookPath("ffmpeg"); err != nil {
				convert = false
			}
		}
		s.converted = convert
		m.URL, m.ContentType, m.Subtitles = srv.Share(Source{
			Path: item.Path, Convert: convert,
			VideoCodec: item.VideoCodec, AudioCodec: item.AudioCodec,
			Subtitles: item.Subtitles, SubtitleIndex: item.SubtitleIndex,
		})
		if convert && item.Start > 0 {
			s.offset = item.Start
			m.URL = withStart(m.URL, item.Start)
			m.Start = 0
		}
	}
	s.media = m
	if err := d.Load(ctx, m); err != nil {
		s.close()
		return nil, err
	}
	pollCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.poll(pollCtx, onStatus)
	return s, nil
}

func (s *Session) poll(ctx context.Context, onStatus func(Status)) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		st, err := s.Device.Status(ctx)
		if err != nil && ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		st.Position += s.offset
		if s.converted && s.media.Duration > 0 {
			st.Duration = s.media.Duration
		}
		s.status = st
		s.mu.Unlock()
		if onStatus != nil {
			onStatus(st)
		}
	}
}

// Status is the last state polled from the device.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// Converted reports whether the file is being converted for the device.
func (s *Session) Converted() bool { return s.converted }

func (s *Session) Play() error  { return s.Device.Play() }
func (s *Session) Pause() error { return s.Device.Pause() }

// Seek jumps to t seconds in the file. A converted stream can't seek, so
// it restarts there.
func (s *Session) Seek(t float64) error {
	if !s.converted {
		return s.Device.Seek(t)
	}
	m := s.media
	m.URL = withStart(stripStart(m.URL), t)
	m.Start = 0
	s.mu.Lock()
	s.offset = t
	s.mu.Unlock()
	return s.Device.Load(context.Background(), m)
}

func (s *Session) SetVolume(v float64) error { return s.Device.SetVolume(v) }

// Stop ends playback on the device and the session.
func (s *Session) Stop() error {
	err := s.Device.Stop()
	s.close()
	return err
}

func (s *Session) close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.server != nil {
		s.server.Close()
	}
	s.Device.Close()
}

func withStart(u string, t float64) string {
	return u + "?t=" + strconv.FormatFloat(t, 'f', 3, 64)
}

func stripStart(u string) string {
	base, _, _ := strings.Cut(u, "?")
	return base
}

func isURL(s string) bool {
	i := strings.Index(s, "://")
	return i > 1 && !strings.ContainsAny(s[:i], `\/.`)
}

// ResolveURL turns a web page (YouTube and other sites yt-dlp knows) into
// a direct media URL a device can fetch; direct media URLs come back
// unchanged.
func ResolveURL(ctx context.Context, u string) (string, error) {
	pu, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	if pu.Scheme != "http" && pu.Scheme != "https" {
		return "", errors.New("cast: only http and https links can be cast")
	}
	if ContentType(pu.Path) != "application/octet-stream" || strings.HasSuffix(strings.ToLower(pu.Path), ".m3u8") {
		return u, nil
	}
	ytdlp, err := exec.LookPath("yt-dlp")
	if err != nil {
		return u, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// A single file with audio and video over plain HTTP: what a cast
	// device can play by itself.
	out, err := exec.CommandContext(ctx, ytdlp, "--no-playlist", "-f",
		"best[ext=mp4][protocol^=http][vcodec!=none][acodec!=none]/best[protocol^=http][vcodec!=none][acodec!=none]/best",
		"-g", u).Output()
	if err != nil {
		return "", errors.New("cast: yt-dlp couldn't get a playable link for this page")
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	if sc.Scan() && sc.Text() != "" {
		return sc.Text(), nil
	}
	return "", errors.New("cast: yt-dlp returned no link")
}

func guessURLType(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return "video/mp4"
	}
	p := strings.ToLower(pu.Path)
	switch {
	case strings.HasSuffix(p, ".m3u8"):
		return "application/x-mpegURL"
	case strings.HasSuffix(p, ".mpd"):
		return "application/dash+xml"
	}
	if t := ContentType(p); t != "application/octet-stream" {
		return t
	}
	return "video/mp4"
}
