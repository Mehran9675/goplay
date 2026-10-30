package settings

import "strings"

// Sections are the settings dialog's pages, in order.
var Sections = []string{"General", "Playback", "Video", "Audio", "Subtitles", "3D & VR", "Casting", "Advanced"}

// Keys of the settings the app (not mpv) acts on.
const (
	KeyLanguage        = "language"
	KeyAlwaysOnTop     = "always_on_top"
	KeyResizeToVideo   = "resize_to_video"
	KeyRemainingTime   = "remaining_time"
	KeyHideDelay       = "hide_delay"
	KeyClickToPause    = "click_to_pause"
	KeyWheel           = "wheel"
	KeyOSD             = "osd"
	KeyRememberVolume  = "remember_volume"
	KeyPauseMinimized  = "pause_minimized"
	KeyQueueFolder     = "queue_folder"
	KeySeekStep        = "seek_step"
	KeySeekStepLarge   = "seek_step_large"
	KeyVolumeStep      = "volume_step"
	KeyAudioDevice     = "audio_device"
	KeyAuto3D          = "auto_3d"
	KeyStereoView      = "stereo_view"
	KeyVRFOV           = "vr_fov"
	KeyVRSensitivity   = "vr_sensitivity"
	KeyVRInvert        = "vr_invert"
	KeyCastConvert     = "cast_convert"
	KeyCastSubtitles   = "cast_subtitles"
	KeyMediaServers    = "media_servers"
	KeyPassKeys        = "pass_keys"
	KeyMPVConf         = "mpv_conf"
	KeyLogLevel        = "log_level"
	KeyExtraMPVOptions = "extra_mpv"

	// KeyVolume stores the last volume (percent) for KeyRememberVolume.
	// It has no Def: it isn't shown in the dialog.
	KeyVolume = "volume"
)

// languages are the choices for the preferred audio and subtitle
// language. Values are mpv language lists: ISO 639-2 codes (both B and
// T forms where they differ) and the 639-1 code, so tracks tagged either
// way match.
var languages = []Option{
	{"Automatic", ""},
	{"English", "eng,en"},
	{"Arabic", "ara,ar"},
	{"Chinese", "chi,zho,zh"},
	{"Dutch", "dut,nld,nl"},
	{"French", "fre,fra,fr"},
	{"German", "ger,deu,de"},
	{"Hindi", "hin,hi"},
	{"Italian", "ita,it"},
	{"Japanese", "jpn,ja"},
	{"Korean", "kor,ko"},
	{"Persian", "per,fas,fa"},
	{"Polish", "pol,pl"},
	{"Portuguese", "por,pt"},
	{"Russian", "rus,ru"},
	{"Spanish", "spa,es"},
	{"Swedish", "swe,sv"},
	{"Turkish", "tur,tr"},
	{"Ukrainian", "ukr,uk"},
	{"Vietnamese", "vie,vi"},
}

// uiLanguages are the languages GoPlay's interface is translated into,
// each named in itself. "" follows the system.
var uiLanguages = []Option{
	{"Automatic", ""},
	{"English", "en"},
	{"Български", "bg"},
	{"Català", "ca"},
	{"Čeština", "cs"},
	{"Dansk", "da"},
	{"Deutsch", "de"},
	{"Eesti", "et"},
	{"Ελληνικά", "el"},
	{"Español", "es"},
	{"Français", "fr"},
	{"Hrvatski", "hr"},
	{"Italiano", "it"},
	{"Latviešu", "lv"},
	{"Lietuvių", "lt"},
	{"Magyar", "hu"},
	{"Nederlands", "nl"},
	{"Norsk bokmål", "nb"},
	{"Polski", "pl"},
	{"Português", "pt"},
	{"Română", "ro"},
	{"Русский", "ru"},
	{"Slovenčina", "sk"},
	{"Slovenščina", "sl"},
	{"Српски", "sr"},
	{"Suomi", "fi"},
	{"Svenska", "sv"},
	{"Українська", "uk"},
}

// StereoViews are the ways to show 3D video, for the default view and
// the 3D menu. Values are player.StereoView names.
var StereoViews = []Option{
	{"Anaglyph red/cyan glasses", "arcd"},
	{"Anaglyph red/cyan (more color)", "arcc"},
	{"Anaglyph green/magenta glasses", "agmd"},
	{"Anaglyph yellow/blue glasses", "aybd"},
	{"2D (left eye)", "ml"},
	{"2D (right eye)", "mr"},
	{"Side by side (3D TV)", "sbs2l"},
	{"Top and bottom (3D TV)", "ab2l"},
	{"Interleaved rows (passive 3D monitor)", "irl"},
	{"Interleaved columns", "icl"},
	{"Checkerboard (3D projector)", "chl"},
}

// Defs is every setting, grouped by Section in display order.
var Defs = []Def{
	// General
	{Key: KeyLanguage, Section: "General", Label: "Language", Kind: Choice, Default: "", Options: uiLanguages},
	{Key: KeyAlwaysOnTop, Section: "General", Label: "Keep the window on top", Kind: Toggle, Default: "no"},
	{Key: KeyResizeToVideo, Section: "General", Label: "Resize the window to fit each video", Kind: Toggle, Default: "yes"},
	{Key: KeyClickToPause, Section: "General", Label: "Click the video to pause", Kind: Toggle, Default: "yes",
		Help: "Double-click always toggles fullscreen."},
	{Key: KeyWheel, Section: "General", Label: "Mouse wheel over the video", Kind: Choice, Default: "volume",
		Options: []Option{{"Changes the volume", "volume"}, {"Seeks", "seek"}, {"Does nothing", "none"}}},
	{Key: KeyHideDelay, Section: "General", Label: "Hide the controls after", Kind: Choice, Default: "2",
		Help:    "When maximized or fullscreen, where the controls cover the video.",
		Options: []Option{{"1 second", "1"}, {"2 seconds", "2"}, {"3 seconds", "3"}, {"5 seconds", "5"}}},
	{Key: KeyRemainingTime, Section: "General", Label: "Show remaining time instead of duration", Kind: Toggle, Default: "no"},
	{Key: KeyOSD, Section: "General", Label: "Show on-screen messages", Kind: Toggle, Default: "yes",
		Help: "Briefly shows volume, speed and seek changes over the video."},
	{Key: KeyPauseMinimized, Section: "General", Label: "Pause when minimized", Kind: Toggle, Default: "no"},

	// Playback
	{Key: "resume", Section: "Playback", Label: "Resume where you left off", Kind: Toggle, Default: "yes",
		MPVFunc: func(v string) []MPVOption {
			return []MPVOption{{"save-position-on-quit", v}, {"resume-playback", v}}
		}},
	{Key: "autoplay_next", Section: "Playback", Label: "Play the next file automatically", Kind: Toggle, Default: "yes",
		MPVFunc: func(v string) []MPVOption {
			keep := "always"
			if v == "yes" {
				keep = "yes"
			}
			return []MPVOption{{"keep-open", keep}}
		}},
	{Key: KeyQueueFolder, Section: "Playback", Label: "Queue the rest of the folder", Kind: Toggle, Default: "yes",
		Help: "Opening one file also adds the other videos next to it, so Next plays them."},
	{Key: KeyRememberVolume, Section: "Playback", Label: "Remember the volume", Kind: Toggle, Default: "yes"},
	{Key: "volume_max", Section: "Playback", Label: "Allow volume up to", Kind: Choice, Default: "130", MPV: "volume-max",
		Options: []Option{{"100%", "100"}, {"130%", "130"}, {"150%", "150"}, {"200%", "200"}}},
	{Key: KeySeekStep, Section: "Playback", Label: "Arrow keys seek by", Kind: Choice, Default: "5",
		Options: []Option{{"1 second", "1"}, {"3 seconds", "3"}, {"5 seconds", "5"}, {"10 seconds", "10"}, {"15 seconds", "15"}}},
	{Key: KeySeekStepLarge, Section: "Playback", Label: "Shift + arrow keys seek by", Kind: Choice, Default: "60",
		Options: []Option{{"30 seconds", "30"}, {"1 minute", "60"}, {"2 minutes", "120"}, {"5 minutes", "300"}}},
	{Key: KeyVolumeStep, Section: "Playback", Label: "Up and down keys change volume by", Kind: Choice, Default: "5",
		Options: []Option{{"1%", "1"}, {"2%", "2"}, {"5%", "5"}, {"10%", "10"}}},
	{Key: "hwdec", Section: "Playback", Label: "Hardware decoding", Kind: Choice, Default: "auto-safe", MPV: "hwdec",
		Help:    "Decodes on the graphics card: much less CPU and power. Turn off if the picture shows glitches.",
		Options: []Option{{"Automatic", "auto-safe"}, {"Automatic (any method)", "auto"}, {"Automatic, copy back", "auto-copy"}, {"Off", "no"}}},

	// Video
	{Key: "fit", Section: "Video", Label: "Picture size", Kind: Choice, Default: "contain",
		Options: []Option{{"Fit the window", "contain"}, {"Fill the window (crop)", "cover"}, {"Stretch to the window", "stretch"}},
		MPVFunc: func(v string) []MPVOption {
			switch v {
			case "cover":
				return []MPVOption{{"keepaspect", "yes"}, {"panscan", "1"}}
			case "stretch":
				return []MPVOption{{"keepaspect", "no"}, {"panscan", "0"}}
			}
			return []MPVOption{{"keepaspect", "yes"}, {"panscan", "0"}}
		}},
	{Key: "scale", Section: "Video", Label: "Upscaling quality", Kind: Choice, Default: "lanczos", MPV: "scale",
		Help: "Higher quality takes more GPU power.",
		Options: []Option{{"Fast (bilinear)", "bilinear"}, {"Good (spline36)", "spline36"}, {"Sharp (lanczos)", "lanczos"},
			{"Best (EWA lanczos)", "ewa_lanczos"}, {"Best, sharper", "ewa_lanczossharp"}}},
	{Key: "deband", Section: "Video", Label: "Remove color banding", Kind: Toggle, Default: "no", MPV: "deband"},
	{Key: "deinterlace", Section: "Video", Label: "Deinterlace", Kind: Toggle, Default: "no", MPV: "deinterlace",
		Help: "For TV recordings and DVDs that show comb-like lines in motion."},
	{Key: "interpolation", Section: "Video", Label: "Smooth motion", Kind: Toggle, Default: "no",
		Help: "Blends frames to match the display's refresh rate, removing judder.",
		MPVFunc: func(v string) []MPVOption {
			sync := "audio"
			if v == "yes" {
				sync = "display-resample"
			}
			return []MPVOption{{"video-sync", sync}, {"interpolation", v}}
		}},
	{Key: "tone_mapping", Section: "Video", Label: "HDR tone mapping", Kind: Choice, Default: "auto", MPV: "tone-mapping",
		Help: "How HDR video is shown on a regular display.",
		Options: []Option{{"Automatic", "auto"}, {"BT.2390", "bt.2390"}, {"Hable (filmic)", "hable"}, {"Mobius", "mobius"},
			{"Reinhard", "reinhard"}, {"Clip", "clip"}}},
	{Key: "icc", Section: "Video", Label: "Use the display's color profile", Kind: Toggle, Default: "no", MPV: "icc-profile-auto"},
	{Key: "dither", Section: "Video", Label: "Dither", Kind: Toggle, Default: "no",
		Help: "Reduces banding on 8-bit displays.",
		MPVFunc: func(v string) []MPVOption {
			d := "no"
			if v == "yes" {
				d = "auto"
			}
			return []MPVOption{{"dither-depth", d}}
		}},
	{Key: "brightness", Section: "Video", Label: "Brightness", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "brightness"},
	{Key: "contrast", Section: "Video", Label: "Contrast", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "contrast"},
	{Key: "saturation", Section: "Video", Label: "Saturation", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "saturation"},
	{Key: "gamma", Section: "Video", Label: "Gamma", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "gamma"},
	{Key: "hue", Section: "Video", Label: "Hue", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "hue"},

	// Audio
	{Key: KeyAudioDevice, Section: "Audio", Label: "Output device", Kind: Choice, Default: "auto", MPV: "audio-device",
		// The app replaces these with the devices mpv reports.
		Options: []Option{{"Automatic", "auto"}}},
	{Key: "alang", Section: "Audio", Label: "Preferred language", Kind: Choice, Default: "", MPV: "alang", Options: languages},
	{Key: "audio_channels", Section: "Audio", Label: "Channels", Kind: Choice, Default: "auto-safe", MPV: "audio-channels",
		Options: []Option{{"Automatic", "auto-safe"}, {"Automatic (any layout)", "auto"}, {"Stereo", "stereo"},
			{"5.1", "5.1"}, {"7.1", "7.1"}}},
	{Key: "spdif", Section: "Audio", Label: "Pass surround sound to the receiver", Kind: Toggle, Default: "no",
		Help: "Sends Dolby and DTS audio undecoded over HDMI or S/PDIF.",
		MPVFunc: func(v string) []MPVOption {
			codecs := ""
			if v == "yes" {
				codecs = "ac3,eac3,dts,dts-hd,truehd"
			}
			return []MPVOption{{"audio-spdif", codecs}}
		}},
	{Key: "normalize", Section: "Audio", Label: "Even out loudness", Kind: Toggle, Default: "no", Group: "af",
		Help: "Raises quiet passages and tames loud ones.",
		GroupFunc: func(v string) string {
			if v == "yes" {
				return "@goplay-norm:lavfi=[dynaudnorm=f=250:g=31]"
			}
			return ""
		}},
	{Key: "night_mode", Section: "Audio", Label: "Night mode", Kind: Toggle, Default: "no", Group: "af",
		Help: "Compresses explosions and music so dialogue stays audible at low volume.",
		GroupFunc: func(v string) string {
			if v == "yes" {
				return "@goplay-night:lavfi=[acompressor=threshold=0.05:ratio=6:attack=5:release=200:makeup=4]"
			}
			return ""
		}},
	{Key: "equalizer", Section: "Audio", Label: "Equalizer", Kind: Choice, Default: "", Group: "af",
		Options: []Option{{"Off", ""}, {"Bass boost", "bass"}, {"Treble boost", "treble"}, {"Clear voices", "voice"},
			{"Loudness (small speakers)", "loud"}},
		GroupFunc: func(v string) string {
			switch v {
			case "bass":
				return "@goplay-eq:lavfi=[bass=g=6:f=110]"
			case "treble":
				return "@goplay-eq:lavfi=[treble=g=5:f=5000]"
			case "voice":
				return "@goplay-eq:lavfi=[equalizer=f=2500:t=q:w=1:g=5,highpass=f=100]"
			case "loud":
				return "@goplay-eq:lavfi=[bass=g=5:f=120,treble=g=3:f=6000]"
			}
			return ""
		}},
	{Key: "pitch_correction", Section: "Audio", Label: "Keep the pitch when changing speed", Kind: Toggle, Default: "yes", MPV: "audio-pitch-correction"},
	{Key: "audio_exclusive", Section: "Audio", Label: "Exclusive mode", Kind: Toggle, Default: "no", MPV: "audio-exclusive",
		Help: "Takes over the audio device for bit-perfect output; other apps go silent."},

	// Subtitles
	{Key: "sub_visibility", Section: "Subtitles", Label: "Show subtitles", Kind: Toggle, Default: "yes", MPV: "sub-visibility"},
	{Key: "slang", Section: "Subtitles", Label: "Preferred language", Kind: Choice, Default: "", MPV: "slang", Options: languages},
	{Key: "sub_auto", Section: "Subtitles", Label: "Load subtitle files from the folder", Kind: Choice, Default: "fuzzy", MPV: "sub-auto",
		Options: []Option{{"Named like the video", "exact"}, {"Containing the video's name", "fuzzy"}, {"All of them", "all"}, {"Off", "no"}}},
	{Key: "sub_scale", Section: "Subtitles", Label: "Text size", Kind: Range, Default: "1", Min: 0.5, Max: 2.5, Step: 0.1, Unit: "×", MPV: "sub-scale"},
	{Key: "sub_pos", Section: "Subtitles", Label: "Position", Kind: Range, Default: "100", Min: 50, Max: 100, Step: 1, Unit: "%", MPV: "sub-pos",
		Help: "100% is the bottom of the picture."},
	{Key: "sub_font", Section: "Subtitles", Label: "Font", Kind: Text, Default: "",
		Help: "A font installed on this computer; empty uses the default.",
		MPVFunc: func(v string) []MPVOption {
			if strings.TrimSpace(v) == "" {
				return nil
			}
			return []MPVOption{{"sub-font", strings.TrimSpace(v)}}
		}},
	{Key: "sub_color", Section: "Subtitles", Label: "Text color", Kind: Choice, Default: "#FFFFFF", MPV: "sub-color",
		Options: []Option{{"White", "#FFFFFF"}, {"Light gray", "#D0D0D0"}, {"Yellow", "#FFE600"}, {"Cyan", "#7FE7FF"}}},
	{Key: "sub_outline", Section: "Subtitles", Label: "Outline", Kind: Range, Default: "1.65", Min: 0, Max: 6, Step: 0.05, MPV: "sub-outline-size"},
	{Key: "sub_box", Section: "Subtitles", Label: "Dark box behind the text", Kind: Toggle, Default: "no",
		MPVFunc: func(v string) []MPVOption {
			style := "outline-and-shadow"
			if v == "yes" {
				style = "opaque-box"
			}
			return []MPVOption{{"sub-border-style", style}}
		}},
	{Key: "sub_codepage", Section: "Subtitles", Label: "Text encoding", Kind: Choice, Default: "auto", MPV: "sub-codepage",
		Help: "For old subtitle files that show garbled characters.",
		Options: []Option{{"Automatic", "auto"}, {"Unicode (UTF-8)", "utf-8"}, {"Western (Windows-1252)", "cp1252"},
			{"Central European (Windows-1250)", "cp1250"}, {"Cyrillic (Windows-1251)", "cp1251"}, {"Arabic (Windows-1256)", "cp1256"},
			{"Greek (Windows-1253)", "cp1253"}, {"Turkish (Windows-1254)", "cp1254"}, {"Hebrew (Windows-1255)", "cp1255"},
			{"Chinese (GB18030)", "gb18030"}, {"Chinese traditional (Big5)", "big5"}, {"Japanese (Shift JIS)", "shift_jis"},
			{"Korean (EUC-KR)", "euc-kr"}}},
	{Key: "ass_override", Section: "Subtitles", Label: "Restyle styled (ASS) subtitles", Kind: Choice, Default: "scale", MPV: "sub-ass-override",
		Options: []Option{{"Size only", "scale"}, {"Yes", "yes"}, {"Always", "force"}, {"Never", "no"}}},

	// 3D & VR
	{Key: KeyAuto3D, Section: "3D & VR", Label: "Detect 3D and 360° videos", Kind: Toggle, Default: "yes",
		Help: "From names like “3D.HSBS”, “TAB”, “360” or “VR180”. You can always choose in the 3D menu."},
	{Key: KeyStereoView, Section: "3D & VR", Label: "Show 3D video as", Kind: Choice, Default: "arcd", Options: StereoViews},
	{Key: KeyVRFOV, Section: "3D & VR", Label: "360° field of view", Kind: Range, Default: "100", Min: 50, Max: 140, Step: 1, Unit: "°"},
	{Key: KeyVRSensitivity, Section: "3D & VR", Label: "360° drag speed", Kind: Range, Default: "1", Min: 0.25, Max: 3, Step: 0.05, Unit: "×"},
	{Key: KeyVRInvert, Section: "3D & VR", Label: "Invert dragging in 360° video", Kind: Toggle, Default: "no",
		Help: "Drag the picture instead of the view."},

	// Casting
	{Key: KeyCastConvert, Section: "Casting", Label: "Convert files the TV can't play", Kind: Toggle, Default: "yes",
		Help: "Repackages MKV and other files on the fly so Chromecast and DLNA devices accept them. Needs FFmpeg."},
	{Key: KeyCastSubtitles, Section: "Casting", Label: "Send subtitles", Kind: Toggle, Default: "yes"},
	{Key: KeyMediaServers, Section: "Casting", Label: "Media server addresses", Kind: Text, Default: "", Lines: 3,
		Help: "Media servers that aren't found by themselves, one per line: an IP address or name (192.168.1.20), " +
			"with the port of its web page for Emby or Jellyfin (192.168.1.20:8096), or the URL of its UPnP description."},

	// Advanced
	{Key: "cache", Section: "Advanced", Label: "Network cache", Kind: Choice, Default: "auto", MPV: "cache",
		Options: []Option{{"Automatic", "auto"}, {"On", "yes"}, {"Off", "no"}}},
	{Key: "cache_size", Section: "Advanced", Label: "Read-ahead buffer", Kind: Choice, Default: "48MiB", MPV: "demuxer-max-bytes",
		Help:    "Memory for buffering ahead. Larger helps slow networks; smaller saves memory.",
		Options: []Option{{"16 MB", "16MiB"}, {"48 MB", "48MiB"}, {"150 MB", "150MiB"}, {"512 MB", "512MiB"}}},
	{Key: "cache_back", Section: "Advanced", Label: "Seek-back buffer", Kind: Choice, Default: "16MiB", MPV: "demuxer-max-back-bytes",
		Options: []Option{{"Off", "0"}, {"16 MB", "16MiB"}, {"48 MB", "48MiB"}, {"150 MB", "150MiB"}}},
	{Key: "ytdl", Section: "Advanced", Label: "Play web pages with yt-dlp", Kind: Toggle, Default: "yes", MPV: "ytdl",
		Help: "Lets you open YouTube and thousands of other sites. Needs yt-dlp installed."},
	{Key: "ytdl_format", Section: "Advanced", Label: "Web video quality", Kind: Choice, Default: "1080",
		Options: []Option{{"Best available", "best"}, {"Up to 4K", "2160"}, {"Up to 1440p", "1440"}, {"Up to 1080p", "1080"},
			{"Up to 720p", "720"}, {"Up to 480p", "480"}, {"Audio only", "audio"}},
		MPVFunc: func(v string) []MPVOption {
			switch v {
			case "best":
				return []MPVOption{{"ytdl-format", "bestvideo+bestaudio/best"}}
			case "audio":
				return []MPVOption{{"ytdl-format", "bestaudio/best"}}
			}
			return []MPVOption{{"ytdl-format", "bestvideo[height<=?" + v + "]+bestaudio/best[height<=?" + v + "]/best"}}
		}},
	{Key: KeyPassKeys, Section: "Advanced", Label: "Send other keys to mpv", Kind: Toggle, Default: "yes",
		Help: "Keys GoPlay doesn't use go to mpv, so bindings in input.conf and scripts work."},
	{Key: KeyMPVConf, Section: "Advanced", Label: "Load mpv.conf, input.conf and scripts", Kind: Toggle, Default: "yes", Restart: true},
	{Key: KeyLogLevel, Section: "Advanced", Label: "mpv log detail", Kind: Choice, Default: "warn", Restart: true,
		Help:    "What the log window shows.",
		Options: []Option{{"Errors and warnings", "warn"}, {"Information", "info"}, {"Verbose", "v"}, {"Debug", "debug"}}},
	{Key: KeyExtraMPVOptions, Section: "Advanced", Label: "Extra mpv options", Kind: Text, Default: "", Lines: 4,
		Help:    "One per line, as name=value. Applied after everything above.",
		MPVFunc: ParseOptions},
}

// ParseOptions reads "name=value" lines (blank lines and # comments
// ignored; a bare name means "yes").
func ParseOptions(text string) []MPVOption {
	var out []MPVOption
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "--")
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			value = "yes"
		}
		out = append(out, MPVOption{strings.TrimSpace(name), strings.TrimSpace(value)})
	}
	return out
}
