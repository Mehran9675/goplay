package cast

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server serves local files to cast devices over HTTP, on the address
// the device can reach. Each file gets an unguessable token, so nothing
// else on the network can browse the computer.
type Server struct {
	srv      *http.Server
	listener net.Listener
	host     string

	mu     sync.Mutex
	shares map[string]*share
}

type share struct {
	path      string
	convert   bool
	videoCopy bool
	audioCopy bool
	subtitles string // subtitle file, or "embedded"
	subIndex  int    // subtitle stream for "embedded"
}

// NewServer starts a server on the local address used to reach
// deviceAddr ("host:port" or a host).
func NewServer(deviceAddr string) (*Server, error) {
	ip, err := LocalIPFor(deviceAddr)
	if err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		return nil, err
	}
	s := &Server{listener: l, host: l.Addr().String(), shares: map[string]*share{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/media/", s.serveMedia)
	mux.HandleFunc("/subs/", s.serveSubs)
	s.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(l)
	return s, nil
}

// Close stops the server.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.srv.Shutdown(ctx)
}

// Source describes a local file to share.
type Source struct {
	Path string
	// Convert remuxes (and where needed transcodes) the file to
	// fragmented MP4 with FFmpeg as it streams.
	Convert bool
	// VideoCodec and AudioCodec are the file's codecs (FFmpeg names),
	// deciding what Convert can copy.
	VideoCodec, AudioCodec string
	// Subtitles is an external subtitle file, or "embedded" to extract
	// subtitle stream SubtitleIndex; "" for none.
	Subtitles     string
	SubtitleIndex int
}

// Share makes src reachable and returns its URL, MIME type and the
// WebVTT subtitle URL (or "").
func (s *Server) Share(src Source) (media, contentType, subs string) {
	tok := token()
	sh := &share{path: src.Path, convert: src.Convert, subtitles: src.Subtitles, subIndex: src.SubtitleIndex}
	sh.videoCopy = castableVideo(src.VideoCodec)
	sh.audioCopy = castableAudio(src.AudioCodec)
	s.mu.Lock()
	s.shares[tok] = sh
	s.mu.Unlock()

	base := "http://" + s.host
	ext := strings.ToLower(filepath.Ext(src.Path))
	if src.Convert {
		media = base + "/media/" + tok + "/video.mp4"
		contentType = "video/mp4"
	} else {
		media = base + "/media/" + tok + "/media" + ext
		contentType = ContentType(src.Path)
	}
	if src.Subtitles != "" {
		subs = base + "/subs/" + tok + "/subtitles.vtt"
	}
	return media, contentType, subs
}

func (s *Server) lookup(r *http.Request, prefix string) (*share, bool) {
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	tok, _, _ := strings.Cut(rest, "/")
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shares[tok]
	return sh, ok
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.lookup(r, "/media/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	// DLNA renderers want these to stream instead of downloading.
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if sh.convert {
		w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=00;DLNA.ORG_CI=1;DLNA.ORG_FLAGS=01700000000000000000000000000000")
		s.streamConverted(w, r, sh)
		return
	}
	w.Header().Set("contentFeatures.dlna.org", "DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000")
	f, err := os.Open(sh.path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ContentType(sh.path))
	http.ServeContent(w, r, "", fi.ModTime(), f)
}

// streamConverted remuxes the file to fragmented MP4 with FFmpeg,
// copying streams the device plays and transcoding the rest. A "t"
// query parameter starts at that many seconds (devices can't seek in a
// live remux, so seeking restarts the stream there).
func (s *Server) streamConverted(w http.ResponseWriter, r *http.Request, sh *share) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		http.Error(w, "FFmpeg is not installed", http.StatusServiceUnavailable)
		return
	}
	args := []string{"-hide_banner", "-loglevel", "error"}
	if t, err := strconv.ParseFloat(r.URL.Query().Get("t"), 64); err == nil && t > 0 {
		args = append(args, "-ss", strconv.FormatFloat(t, 'f', 3, 64))
	}
	args = append(args, "-i", sh.path, "-map", "0:v:0?", "-map", "0:a:0?", "-sn", "-dn")
	if sh.videoCopy {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-pix_fmt", "yuv420p")
	}
	if sh.audioCopy {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
	}
	args = append(args, "-movflags", "frag_keyframe+empty_moov+default_base_moof", "-f", "mp4", "pipe:1")
	w.Header().Set("Content-Type", "video/mp4")
	if r.Method == http.MethodHead {
		return
	}
	cmd := exec.CommandContext(r.Context(), ffmpeg, args...)
	cmd.Stdout = w
	cmd.Run()
}

// serveSubs serves subtitles as WebVTT, which both Chromecast and most
// DLNA renderers take.
func (s *Server) serveSubs(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.lookup(r, "/subs/")
	if !ok || sh.subtitles == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ext := strings.ToLower(filepath.Ext(sh.subtitles))
	switch {
	case ext == ".vtt":
		http.ServeFile(w, r, sh.subtitles)
	case ext == ".srt":
		f, err := os.Open(sh.subtitles)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		SRTToVTT(w, f)
	default:
		// Anything else (ASS, embedded tracks): FFmpeg converts.
		ffmpeg, err := exec.LookPath("ffmpeg")
		if err != nil {
			http.Error(w, "FFmpeg is not installed", http.StatusServiceUnavailable)
			return
		}
		in, mapArg := sh.subtitles, "0:s:0"
		if sh.subtitles == "embedded" {
			in, mapArg = sh.path, "0:s:"+strconv.Itoa(sh.subIndex)
		}
		cmd := exec.CommandContext(r.Context(), ffmpeg, "-hide_banner", "-loglevel", "error",
			"-i", in, "-map", mapArg, "-f", "webvtt", "pipe:1")
		cmd.Stdout = w
		cmd.Run()
	}
}

var srtTime = regexp.MustCompile(`(\d+:\d\d:\d\d),(\d\d\d)`)

// SRTToVTT converts SubRip subtitles to WebVTT.
func SRTToVTT(w io.Writer, r io.Reader) error {
	bw := bufio.NewWriter(w)
	bw.WriteString("WEBVTT\n\n")
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimPrefix(sc.Text(), "\ufeff")
		if strings.Contains(line, "-->") {
			line = srtTime.ReplaceAllString(line, "$1.$2")
		}
		bw.WriteString(line)
		bw.WriteByte('\n')
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return bw.Flush()
}

// ContentType is the MIME type for a media file, by extension.
func ContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".mov":
		return "video/quicktime"
	case ".avi":
		return "video/x-msvideo"
	case ".ts", ".m2ts", ".mts":
		return "video/mp2t"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".flac":
		return "audio/flac"
	case ".ogg", ".oga", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	}
	if t := mime.TypeByExtension(filepath.Ext(path)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// NeedsConversion reports whether a file likely won't play on a cast
// device as is: its container or codecs are outside what Chromecasts
// and most TVs take (MP4/WebM with H.264/VP8/VP9 and AAC/MP3/Opus).
func NeedsConversion(path, videoCodec, audioCodec string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".webm", ".mp3", ".m4a", ".aac":
	default:
		return true
	}
	if videoCodec != "" && !castableVideo(videoCodec) {
		return true
	}
	return audioCodec != "" && !castableAudio(audioCodec)
}

func castableVideo(c string) bool {
	switch strings.ToLower(c) {
	case "h264", "vp8", "vp9":
		return true
	}
	return false
}

func castableAudio(c string) bool {
	switch strings.ToLower(c) {
	case "aac", "mp3", "opus", "vorbis", "flac":
		return true
	}
	return false
}

// LocalIPFor returns this computer's address on the network that reaches
// addr. No packets are sent: dialing UDP only picks the route.
func LocalIPFor(addr string) (string, error) {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	c, err := net.Dial("udp", net.JoinHostPort(host, "9"))
	if err != nil {
		return "", fmt.Errorf("cast: no route to %s: %w", host, err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func token() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
