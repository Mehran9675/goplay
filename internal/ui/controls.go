package ui

import (
	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/goplay-ebiten/internal/settings"
)

// Button and icon sizes in dp. Buttons are given an explicit square size
// so the icon inside never gets a non-square box (which would stretch the
// round play button into a pill).
const (
	btnSize      = 40
	btnIcon      = 22
	playSize     = 44
	playIcon     = 26
	volumeWidth  = 96
	timeFontSize = 13
)

// bar is the control bar pinned to the bottom of the window: the seek
// row on top, then the transport buttons. Outside fullscreen it has its
// own strip below the video and a solid background; in fullscreen it
// overlays the video, fading from transparent into a dark scrim.
func (u *UI) bar(c Controller) giode.Element {
	st := giode.Styles{
		Width:         -1,
		Height:        properties.Height(controlH),
		Background:    barColor,
		FlexDirection: giode.FlexDirectionColumn,
		Justify:       giode.JustifyCenter,
		Padding:       giode.SymmetricInset(16, 6),
		Gap:           2,
	}
	if u.Fullscreen {
		st.Background = ""
		st.BackgroundGradient = giode.Linear(180, "#0b0e1400", "#0b0e14b3", "#0b0e14eb")
		st.Justify = giode.JustifyEnd
		st.Padding = giode.Inset{Right: 16, Bottom: 10, Left: 16}
	}
	return giode.Box(st, u.seekRow(c), u.transportRow(c))
}

// seekRow draws elapsed time, the progress slider and the duration.
// Clicking or dragging the slider seeks. While the user drags, the slider
// keeps the dragged value (see syncSliders); otherwise it tracks
// c.Pos()/c.Duration(). The right-hand label shows the duration, or the
// time remaining when that setting is on.
func (u *UI) seekRow(c Controller) giode.Element {
	pos, dur := "--:--", "--:--"
	if c.Duration() > 0 {
		pos, dur = formatTime(c.Pos()), formatTime(c.Duration())
		if c.Settings().Bool(settings.KeyRemainingTime) {
			dur = "-" + formatTime(c.Duration()-c.Pos())
		}
	}
	return giode.Box(giode.Styles{
		Width:  -1,
		Height: 24,
		Align:  giode.AlignCenter,
		Gap:    12,
		Wrap:   giode.WrapNoWrap,
	},
		timeLabel(pos),
		giode.Box(giode.Styles{Width: -1}, u.seek),
		timeLabel(dur),
	)
}

// transportRow holds the playback controls and volume on the left and
// the settings and fullscreen controls on the right.
func (u *UI) transportRow(c Controller) giode.Element {
	return giode.Box(giode.Styles{
		Width: -1,
		Align: giode.AlignCenter,
		Gap:   4,
		// The row must not wrap: wrapping measures the expanding
		// spacer at the full available width, pushing the right-hand
		// controls onto a clipped second line.
		Wrap: giode.WrapNoWrap,
	},
		u.playBtn,
		u.stopBtn,
		u.nextBtn,
		u.muteBtn,
		giode.Box(giode.Styles{
			Width:  properties.Width(volumeWidth),
			Height: 24,
			Align:  giode.AlignCenter,
		}, u.vol),
		// Spacer: pushes the settings and fullscreen controls right.
		giode.Box(giode.Styles{Width: -1}),
		u.settingsBtn,
		u.fullscreenBtn,
	)
}

// updateIcons refreshes the icons that depend on playback, volume and
// fullscreen state. Icons are stateless, so they are rebuilt every frame.
func (u *UI) updateIcons(c Controller) {
	play := "play"
	if c.HasMedia() && !c.Paused() && !c.Finished() {
		play = "pause"
	}
	fullscreen := "fullscreen"
	if c.Fullscreen() {
		fullscreen = "fullscreen-exit"
	}

	u.playBtn.Icon(glyph(play, playIcon, onHighlight))
	u.stopBtn.Icon(glyph("stop", btnIcon, iconColor))
	u.nextBtn.Icon(glyph("skip-next", btnIcon, iconColor))
	u.muteBtn.Icon(glyph(volumeIcon(c.Volume()), btnIcon, iconColor))
	u.settingsBtn.Icon(glyph("settings", btnIcon, iconColor))
	u.fullscreenBtn.Icon(glyph(fullscreen, btnIcon, iconColor))
}

// volumeIcon picks the speaker glyph for volume v in [0, 1].
func volumeIcon(v float64) string {
	switch {
	case v <= 0:
		return "volume-off"
	case v < 0.5:
		return "volume-down"
	default:
		return "volume-up"
	}
}

// timeLabel is a fixed-width time readout, so the seek bar does not
// shift as the digits change.
func timeLabel(s string) giode.Element {
	return giode.Box(giode.Styles{MinWidth: 44, Justify: giode.JustifyCenter},
		giode.Text(s, giode.Styles{Color: fgColor, FontSize: timeFontSize, FontWeight: giode.FontWeightMedium}))
}
