package app

import (
	"math"

	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/iconbutton"
	"github.com/mehran9675/giode/components/slider"
	"github.com/mehran9675/giode/components/tooltip"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/actions"
	"github.com/mehran9675/goplay/internal/cast"
	"github.com/mehran9675/goplay/internal/player"
	"github.com/mehran9675/goplay/internal/settings"
)

// controlBar is the bottom bar: the timeline, then transport and volume
// on the left and the media controls on the right.
type controlBar struct {
	u    *ui
	seek *seekBar
	vol  *slider.Slider

	prev, play, next, mute              *iconbutton.Button
	normalize, night                    *iconbutton.Button
	subs, audio, cast, list, more, full *iconbutton.Button
	speed                               *button.Button
	timeToggle                          *elements.Pressable
	subsA, audioA, speedA, castA, moreA *anchor
	tips                                map[*iconbutton.Button]*tooltip.Tooltip
	tipText                             map[*iconbutton.Button]func() string
}

// ghostButton is a round, transparent icon button that lights up on
// hover.
func ghostButton(icon string, size int, label string, onClick func()) *iconbutton.Button {
	pad := (buttonSize - size) / 2
	return iconbutton.New(ic(icon, size, colText), styles.Styles{
		Color:        colText,
		Width:        buttonSize,
		Height:       buttonSize,
		Padding:      properties.UniformInset(pad),
		BorderRadius: buttonSize / 2,
	}).Variant(iconbutton.VariantGhost).Label(label).OnClick(onClick)
}

func newControlBar(u *ui) *controlBar {
	a := u.a
	b := &controlBar{u: u, seek: &seekBar{u: u}, tips: map[*iconbutton.Button]*tooltip.Tooltip{}, tipText: map[*iconbutton.Button]func() string{}}
	run := func(id string) func() { return func() { a.actions.Run(id) } }

	b.prev = ghostButton("previous", iconSize, "Previous", run("previous"))
	b.next = ghostButton("next", iconSize, "Next", run("next"))
	b.play = iconbutton.New(ic("play", playIconSize, colOnLight), styles.Styles{
		Background:   colHighlight,
		Width:        playSize,
		Height:       playSize,
		Padding:      properties.UniformInset((playSize - playIconSize) / 2),
		BorderRadius: playSize / 2,
	}).Label("Play").OnClick(func() {
		if a.player.State.Idle {
			a.openFileDialog()
			return
		}
		a.togglePause()
	})
	b.mute = ghostButton("volume-high", iconSize, "Mute", a.toggleMute)
	b.vol = slider.New(styles.Styles{Color: colHighlight, Background: colTrack, BorderColor: colHighlight}).
		OnChange(func(v float32) {
			if a.casting() {
				a.castVolume(float64(v))
				return
			}
			max := a.player.State.VolumeMax
			if max <= 0 {
				max = 100
			}
			a.setVolume(float64(v) * max)
		})

	b.normalize = ghostButton("normalize", iconSize, "Even out loudness", func() { a.toggleSetting("normalize", "Even out loudness") })
	b.night = ghostButton("night", iconSize, "Night mode", func() { a.toggleSetting("night_mode", "Night mode") })

	b.subs = ghostButton("subtitles", iconSize, "Subtitles", nil)
	b.subsA = &anchor{el: b.subs}
	b.subs.OnClick(func() { u.openMenuOver(u.subtitleMenu(), b.subsA) })
	b.audio = ghostButton("audio", iconSize, "Audio", nil)
	b.audioA = &anchor{el: b.audio}
	b.audio.OnClick(func() { u.openMenuOver(u.audioMenu(), b.audioA) })
	b.speed = button.New("1×", styles.Styles{
		Color: colText, FontSize: 13, FontWeight: properties.FontWeightSemiBold,
		Height: 32, Padding: properties.SymmetricInset(10, 0), BorderRadius: 16,
	}).Variant(button.VariantGhost)
	b.speedA = &anchor{el: b.speed}
	b.speed.OnClick(func() { u.openMenuOver(u.speedMenu(), b.speedA) })
	b.cast = ghostButton("cast", iconSize, "Cast", run("cast"))
	b.castA = &anchor{el: b.cast}
	b.list = ghostButton("playlist", iconSize, "Playlist", run("playlist"))
	b.more = ghostButton("more", iconSize, "More", nil)
	b.moreA = &anchor{el: b.more}
	b.more.OnClick(func() { u.openMenuOver(u.moreMenu(), b.moreA) })
	b.full = ghostButton("fullscreen", iconSize, "Fullscreen", run("fullscreen"))

	b.timeToggle = elements.NewPressable(func(s elements.PressState) elements.Element {
		_, dur := a.timeline()
		pos, _ := a.timeline()
		text := player.FormatTime(dur)
		if a.settings.Bool(settings.KeyRemainingTime) && dur > 0 {
			text = "−" + player.FormatTime(dur-pos)
		}
		if dur <= 0 {
			text = "--:--"
		}
		c := colMuted
		if s.Hovered {
			c = colText
		}
		return timeText(text, c)
	}).Label("Duration").Focusable(false).OnClick(func(elements.ClickEvent) {
		a.toggleSetting(settings.KeyRemainingTime, "Remaining time")
	})

	tip := func(btn *iconbutton.Button, text func() string) {
		b.tips[btn] = tooltip.New(btn, text(), styles.Styles{Background: colSurface2, Color: colText, FontSize: 12})
		b.tipText[btn] = text
	}
	key := func(title, id string) func() string {
		return func() string {
			if ks := a.actions.Keys(id); len(ks) > 0 {
				return title + "  " + actions.Display(ks[0])
			}
			return title
		}
	}
	tip(b.prev, key("Previous", "previous"))
	tip(b.next, key("Next", "next"))
	tip(b.play, func() string {
		if a.player.State.Idle {
			return "Open a file"
		}
		return key("Play / Pause", "play_pause")()
	})
	tip(b.mute, key("Mute", "mute"))
	tip(b.normalize, func() string { return onOff("Even out loudness", a.settings.Bool("normalize")) })
	tip(b.night, func() string { return onOff("Night mode", a.settings.Bool("night_mode")) })
	tip(b.subs, key("Subtitles", "subs_toggle"))
	tip(b.audio, func() string { return "Audio" })
	tip(b.cast, key("Cast", "cast"))
	tip(b.list, key("Playlist", "playlist"))
	tip(b.more, func() string { return "More" })
	tip(b.full, key("Fullscreen", "fullscreen"))
	return b
}

// busy reports whether the user is operating the bar (keep it shown).
func (b *controlBar) busy() bool { return b.seek.dragging || b.vol.Dragging() }

// refresh updates icons, labels and the volume slider from the state.
func (b *controlBar) refresh() {
	a := b.u.a
	st := &a.player.State
	playing := !st.Idle && !st.Paused
	icon := "play"
	switch {
	case a.casting():
		if s := a.cast.status.State; s == cast.StatePlaying || s == cast.StateBuffering || s == cast.StateLoading {
			icon = "pause"
		}
	case st.EOF && st.Paused:
		icon = "replay"
	case playing:
		icon = "pause"
	}
	b.play.Icon(ic(icon, playIconSize, colOnLight))

	vol, max := st.Volume, st.VolumeMax
	if max <= 0 {
		max = 100
	}
	level := vol / max
	if a.casting() {
		level = math.Max(0, a.cast.status.Volume)
		vol = level * 100
		max = 100
	}
	mic := "volume-high"
	switch {
	case st.Muted || vol <= 0:
		mic = "volume-off"
	case vol < 50:
		mic = "volume-low"
	}
	b.mute.Icon(ic(mic, iconSize, colText))
	// The audio effects light up while on.
	effect := func(btn *iconbutton.Button, icon, setting string) {
		c := colFaint
		if a.settings.Bool(setting) {
			c = colHighlight
		}
		btn.Icon(ic(icon, iconSize, c))
	}
	effect(b.normalize, "normalize", "normalize")
	effect(b.night, "night", "night_mode")
	if !b.vol.Dragging() {
		b.vol.SetValue(float32(level))
	}

	full := "fullscreen"
	if a.g.Fullscreen() {
		full = "fullscreen-exit"
	}
	b.full.Icon(ic(full, iconSize, colText))
	castIcon := "cast"
	if a.casting() {
		castIcon = "cast-on"
	}
	b.cast.Icon(ic(castIcon, iconSize, colText))
	b.speed.Label(speedLabel(st.Speed))
	for btn, t := range b.tips {
		t.SetContent(b.tipText[btn]())
	}
}

// effectsMinWidth is how wide in dp the bar must be to show the audio
// effect buttons as well; narrower, they'd push the bar's last buttons
// off its edge (the audio menu still has them).
const effectsMinWidth = 760

// element builds the bar for this frame, width px wide: solid beside
// the video, a fade over it.
func (b *controlBar) element(overlay bool, width int) elements.Element {
	b.refresh()
	a := b.u.a
	st := &a.player.State
	pos, _ := a.timeline()

	bg := styles.Styles{
		Width: -1, Height: -1,
		FlexDirection: properties.FlexDirectionColumn,
		Justify:       properties.JustifyCenter,
		Padding:       properties.Inset{Top: 6, Right: 14, Bottom: 10, Left: 14},
		Gap:           4,
		Background:    colSurface,
	}
	if overlay {
		bg.Background = ""
		bg.BackgroundGradient = properties.Linear(180, "#00000000", "#000000a6", "#000000e0")
		bg.Justify = properties.JustifyEnd
	}

	posText := "--:--"
	if !st.Idle || a.casting() {
		posText = player.FormatTime(pos)
	}
	seekRow := elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Gap: 10, Wrap: properties.WrapNoWrap},
		elements.Box(styles.Styles{MinWidth: 52, Justify: properties.JustifyEnd}, timeText(posText, colText)),
		elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Raw(b.seek.Layout)),
		elements.Box(styles.Styles{MinWidth: 52}, b.timeToggle),
	)

	left := []elements.Element{b.tips[b.prev], b.tips[b.play], b.tips[b.next], spacer(8), b.tips[b.mute],
		elements.Box(styles.Styles{Width: 92, Height: 24, Align: properties.AlignCenter}, b.vol)}
	if width >= b.u.dp(effectsMinWidth) {
		left = append(left, spacer(6), b.tips[b.normalize], b.tips[b.night])
	}
	right := []elements.Element{b.speedA}
	if len(st.TracksOf(player.Subtitle)) > 0 || !st.Idle {
		right = append(right, &anchor{el: b.tips[b.subs]})
		b.subsA.el = b.tips[b.subs]
		right[len(right)-1] = b.subsA
	}
	if len(st.TracksOf(player.Audio)) > 0 {
		b.audioA.el = b.tips[b.audio]
		right = append(right, b.audioA)
	}
	if a.stereoActive() || a.vrActive() {
		right = append(right, stereoBadge(a))
	}
	b.castA.el = b.tips[b.cast]
	b.moreA.el = b.tips[b.more]
	right = append(right, b.castA, b.tips[b.list], b.moreA, b.tips[b.full])

	row := []elements.Element{}
	row = append(row, left...)
	row = append(row, elements.Box(styles.Styles{FlexGrow: 1, Width: -1}))
	row = append(row, right...)
	controls := elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Gap: 2, Wrap: properties.WrapNoWrap}, row...)
	return elements.Box(bg, seekRow, controls)
}

func spacer(w int) elements.Element { return elements.Box(styles.Styles{Width: properties.Width(w)}) }

// timeText is a time readout with tabular digits so it doesn't jitter.
func timeText(s, color string) elements.Element {
	return elements.Text(s, styles.Styles{Color: color, FontSize: 13, FontWeight: properties.FontWeightMedium})
}

// stereoBadge marks 3D or 360° mode in the bar; clicking opens its menu.
func stereoBadge(a *App) elements.Element {
	label := "3D"
	if a.vrActive() {
		label = "360°"
	}
	return elements.Keyed("bar", "stereo-badge", func() *elements.Pressable {
		return elements.NewPressable(func(s elements.PressState) elements.Element {
			bg := colSurface3
			if s.Hovered {
				bg = colBorder
			}
			return elements.Box(styles.Styles{Background: bg, BorderRadius: 6, Padding: properties.SymmetricInset(8, 4), Margin: properties.SymmetricInset(4, 0)},
				elements.Text(label, styles.Styles{Color: colText, FontSize: 12, FontWeight: properties.FontWeightSemiBold}))
		}).Label("3D and 360° options").Focusable(false).OnClick(func(elements.ClickEvent) {
			a.ui.openMenuAt(a.ui.stereoMenu(), a.ui.lastPointer.Round())
		})
	})
}
