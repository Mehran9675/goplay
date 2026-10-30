package player

import "strconv"

// Audio output devices. mpv opens its output once, when a file's audio
// starts, and doesn't look again: a new default device (headphones
// plugged in, the system's output switched) keeps playing on the old
// one, and when an output fails to open mpv gives up on audio for the
// file, so changing the device afterwards does nothing. The player
// follows mpv's device list (it changes when devices come and go, and on
// macOS when the default changes) and reopens the output itself.

// SetAudioDevice sets the output device by its name in mpv's
// audio-device-list, or "auto" for the system's default, and moves the
// playing audio to it. While a chosen device is missing the default
// plays; the device is used again when it comes back.
func (p *Player) SetAudioDevice(name string) {
	if name == "" {
		name = "auto"
	}
	p.audioDevice = name
	p.applyAudioDevice()
	p.restartAudio()
}

// audioDevicesChanged runs when mpv's device list changes.
func (p *Player) audioDevicesChanged() {
	if !p.devicesSeen {
		// The first notice is the list mpv starts with.
		p.devicesSeen = true
		p.applyAudioDevice()
		return
	}
	if p.applyAudioDevice() || p.audioDevice == "auto" {
		// A chosen device came or went, or the default may have
		// changed: mpv opened its output on what was the default.
		p.restartAudio()
	}
}

// applyAudioDevice points mpv at the wanted device, or the default
// while it's missing. It reports whether that changed mpv's device.
func (p *Player) applyAudioDevice() bool {
	want := p.audioDevice
	if want == "" {
		want = "auto"
	}
	if want != "auto" && !p.hasAudioDevice(want) {
		want = "auto"
	}
	if cur, _ := p.h.String("audio-device"); cur == want {
		return false
	}
	p.h.SetString("audio-device", want)
	return true
}

func (p *Player) hasAudioDevice(name string) bool {
	var list []struct {
		Name string `json:"name"`
	}
	if p.h.JSON("audio-device-list", &list) != nil {
		// Without a list there's nothing to compare against.
		return true
	}
	for _, d := range list {
		if d.Name == name {
			return true
		}
	}
	return false
}

// restartAudio reopens the audio output on mpv's current device. When
// mpv has no output (it failed to open one and gave up), reselecting
// the audio track starts the audio again, output and all.
func (p *Player) restartAudio() {
	if _, ok := p.h.String("current-ao"); ok {
		p.h.Command("ao-reload")
		return
	}
	if p.audioTrack == 0 {
		return
	}
	// mpv deselects the track when its output fails; it may also still
	// count it selected, so it's turned off first.
	p.h.SetString(fileLocal("aid"), "no")
	p.h.SetString(fileLocal("aid"), strconv.Itoa(p.audioTrack))
}

// noteAudioTrack remembers the playing audio track from the track list,
// for restartAudio.
func (p *Player) noteAudioTrack() {
	if t, ok := p.State.SelectedTrack(Audio); ok {
		p.audioTrack = t.ID
	}
}
