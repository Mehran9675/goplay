package settings

// Sections are the settings dialog's tabs, in order.
var Sections = []string{"Playback", "Video", "Audio", "Subtitles", "Interface", "Advanced"}

// Keys of the settings the app (not mpv) acts on.
const (
	KeyAutoplayNext   = "autoplay_next"
	KeyRepeatFolder   = "repeat_folder"
	KeyResume         = "resume"
	KeySeekStep       = "seek_step"
	KeyVolumeStep     = "volume_step"
	KeyRememberVolume = "remember_volume"
	KeyAudioDevice    = "audio_device"
	KeyAlwaysOnTop    = "always_on_top"
	KeyRemainingTime  = "remaining_time"
	KeyResizeToVideo  = "resize_to_video"
	KeyMPVConf        = "mpv_conf"

	// KeyVolume stores the last volume (0-1) for KeyRememberVolume. It
	// has no Def: it isn't shown in the dialog.
	KeyVolume = "volume"
)

// languages are the choices for the preferred audio and subtitle
// language. Values are mpv language lists: ISO 639-2 codes (both B and
// T forms where they differ) and the 639-1 code, so tracks tagged
// either way match.
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
	{"Turkish", "tur,tr"},
}

// Defs is every setting, grouped by Section in display order.
var Defs = []Def{
	// Playback
	{Key: KeyResume, Section: "Playback", Label: "Resume where you left off", Kind: Toggle, Default: "yes",
		MPVFunc: func(v string) []MPVOption {
			return []MPVOption{{"save-position-on-quit", v}, {"resume-playback", v}}
		}},
	{Key: KeyAutoplayNext, Section: "Playback", Label: "Play the next file in the folder", Kind: Toggle, Default: "yes"},
	{Key: KeyRepeatFolder, Section: "Playback", Label: "Repeat the folder", Kind: Toggle, Default: "no"},
	{Key: KeySeekStep, Section: "Playback", Label: "Arrow keys seek by", Kind: Choice, Default: "5",
		Options: []Option{{"1 second", "1"}, {"2 seconds", "2"}, {"5 seconds", "5"}, {"10 seconds", "10"}, {"30 seconds", "30"}}},
	{Key: KeyVolumeStep, Section: "Playback", Label: "Arrow keys change volume by", Kind: Choice, Default: "5",
		Options: []Option{{"1%", "1"}, {"2%", "2"}, {"5%", "5"}, {"10%", "10"}}},
	{Key: KeyRememberVolume, Section: "Playback", Label: "Remember volume", Kind: Toggle, Default: "yes"},
	{Key: "hwdec", Section: "Playback", Label: "Hardware decoding", Kind: Choice, Default: "auto-safe", MPV: "hwdec",
		Options: []Option{{"Automatic (safe)", "auto-safe"}, {"Automatic (any)", "auto"}, {"Off", "no"}}},

	// Video
	{Key: "scale", Section: "Video", Label: "Upscaling quality", Kind: Choice, Default: "lanczos", MPV: "scale",
		Options: []Option{{"Bilinear (fastest)", "bilinear"}, {"Spline36", "spline36"}, {"Lanczos", "lanczos"},
			{"EWA Lanczos", "ewa_lanczos"}, {"EWA Lanczos sharp", "ewa_lanczossharp"}}},
	{Key: "deband", Section: "Video", Label: "Remove banding", Kind: Toggle, Default: "no", MPV: "deband"},
	{Key: "deinterlace", Section: "Video", Label: "Deinterlace", Kind: Toggle, Default: "no", MPV: "deinterlace"},
	{Key: "interpolation", Section: "Video", Label: "Smooth motion (interpolation)", Kind: Toggle, Default: "no",
		MPVFunc: func(v string) []MPVOption {
			sync := "audio"
			if v == "yes" {
				sync = "display-resample"
			}
			return []MPVOption{{"video-sync", sync}, {"interpolation", v}}
		}},
	{Key: "tone_mapping", Section: "Video", Label: "HDR tone mapping", Kind: Choice, Default: "auto", MPV: "tone-mapping",
		Options: []Option{{"Automatic", "auto"}, {"BT.2390", "bt.2390"}, {"Hable", "hable"}, {"Mobius", "mobius"},
			{"Reinhard", "reinhard"}, {"Clip", "clip"}}},
	{Key: "brightness", Section: "Video", Label: "Brightness", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "brightness"},
	{Key: "contrast", Section: "Video", Label: "Contrast", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "contrast"},
	{Key: "saturation", Section: "Video", Label: "Saturation", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "saturation"},
	{Key: "gamma", Section: "Video", Label: "Gamma", Kind: Range, Default: "0", Min: -100, Max: 100, Step: 1, MPV: "gamma"},

	// Audio
	{Key: KeyAudioDevice, Section: "Audio", Label: "Output device", Kind: Choice, Default: "auto", MPV: "audio-device",
		// The app replaces these with the devices mpv reports.
		Options: []Option{{"Automatic", "auto"}}},
	{Key: "alang", Section: "Audio", Label: "Preferred language", Kind: Choice, Default: "", MPV: "alang", Options: languages},
	{Key: "audio_channels", Section: "Audio", Label: "Channels", Kind: Choice, Default: "auto-safe", MPV: "audio-channels",
		Options: []Option{{"Automatic (safe)", "auto-safe"}, {"Automatic (any)", "auto"}, {"Stereo", "stereo"},
			{"5.1", "5.1"}, {"7.1", "7.1"}}},
	{Key: "normalize", Section: "Audio", Label: "Normalize loudness", Kind: Toggle, Default: "no",
		MPVFunc: func(v string) []MPVOption {
			af := ""
			if v == "yes" {
				af = "lavfi=[dynaudnorm=f=250:g=31]"
			}
			return []MPVOption{{"af", af}}
		}},
	{Key: "pitch_correction", Section: "Audio", Label: "Keep pitch when changing speed", Kind: Toggle, Default: "yes", MPV: "audio-pitch-correction"},
	{Key: "audio_exclusive", Section: "Audio", Label: "Exclusive mode", Kind: Toggle, Default: "no", MPV: "audio-exclusive"},
	{Key: "audio_delay", Section: "Audio", Label: "Audio delay", Kind: Range, Default: "0", Min: -2, Max: 2, Step: 0.05, Unit: " s", MPV: "audio-delay"},

	// Subtitles
	{Key: "sub_visibility", Section: "Subtitles", Label: "Show subtitles", Kind: Toggle, Default: "yes", MPV: "sub-visibility"},
	{Key: "slang", Section: "Subtitles", Label: "Preferred language", Kind: Choice, Default: "", MPV: "slang", Options: languages},
	{Key: "sub_auto", Section: "Subtitles", Label: "Load subtitle files from the folder", Kind: Choice, Default: "exact", MPV: "sub-auto",
		Options: []Option{{"Matching the file name", "exact"}, {"Containing the file name", "fuzzy"}, {"All", "all"}, {"Off", "no"}}},
	{Key: "sub_scale", Section: "Subtitles", Label: "Text size", Kind: Range, Default: "1", Min: 0.5, Max: 2.5, Step: 0.1, Unit: "×", MPV: "sub-scale"},
	{Key: "sub_pos", Section: "Subtitles", Label: "Vertical position", Kind: Range, Default: "100", Min: 50, Max: 100, Step: 1, Unit: "%", MPV: "sub-pos"},
	{Key: "sub_color", Section: "Subtitles", Label: "Text color", Kind: Choice, Default: "#FFFFFF", MPV: "sub-color",
		Options: []Option{{"White", "#FFFFFF"}, {"Light gray", "#D0D0D0"}, {"Yellow", "#FFE600"}, {"Cyan", "#7FE7FF"}}},
	{Key: "sub_outline", Section: "Subtitles", Label: "Outline thickness", Kind: Range, Default: "1.65", Min: 0, Max: 6, Step: 0.05, MPV: "sub-outline-size"},
	{Key: "sub_delay", Section: "Subtitles", Label: "Subtitle delay", Kind: Range, Default: "0", Min: -5, Max: 5, Step: 0.1, Unit: " s", MPV: "sub-delay"},
	{Key: "ass_override", Section: "Subtitles", Label: "Restyle styled (ASS) subtitles", Kind: Choice, Default: "scale", MPV: "sub-ass-override",
		Options: []Option{{"Size only", "scale"}, {"Yes", "yes"}, {"Always", "force"}, {"Never", "no"}}},

	// Interface
	{Key: KeyAlwaysOnTop, Section: "Interface", Label: "Keep the window on top", Kind: Toggle, Default: "no"},
	{Key: KeyResizeToVideo, Section: "Interface", Label: "Resize the window to fit the video", Kind: Toggle, Default: "yes"},
	{Key: KeyRemainingTime, Section: "Interface", Label: "Show remaining time", Kind: Toggle, Default: "no"},

	// Advanced
	{Key: "cache", Section: "Advanced", Label: "Network cache", Kind: Choice, Default: "auto", MPV: "cache",
		Options: []Option{{"Automatic", "auto"}, {"On", "yes"}, {"Off", "no"}}},
	{Key: "cache_size", Section: "Advanced", Label: "Read-ahead buffer", Kind: Choice, Default: "64MiB", MPV: "demuxer-max-bytes",
		Options: []Option{{"32 MB", "32MiB"}, {"64 MB", "64MiB"}, {"150 MB", "150MiB"}, {"512 MB", "512MiB"}, {"1 GB", "1024MiB"}}},
	{Key: "ytdl", Section: "Advanced", Label: "Play web links with yt-dlp", Kind: Toggle, Default: "yes", MPV: "ytdl"},
	{Key: KeyMPVConf, Section: "Advanced", Label: "Load mpv.conf", Kind: Toggle, Default: "yes", Restart: true,
		MPVFunc: func(v string) []MPVOption { return []MPVOption{{"config", v}} }},
}
