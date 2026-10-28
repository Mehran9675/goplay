package app

import (
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/virtuallist"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/components/spinner"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/actions"
	"github.com/mehran9675/goplay/internal/cast"
	"github.com/mehran9675/goplay/internal/library"
	"github.com/mehran9675/goplay/internal/player"
)

// dialogs are the modal windows. Each has an open flag the Giode dialog
// watches.
type dialogs struct {
	u *ui

	urlOpen  bool
	url      *dialog.Dialog
	urlInput *input.Input

	jumpOpen  bool
	jump      *dialog.Dialog
	jumpInput *input.Input

	castOpen bool
	cast     *dialog.Dialog

	paletteOpen  bool
	palette      *dialog.Dialog
	paletteInput *input.Input
	paletteList  *virtuallist.List
	matches      []*actions.Action

	logOpen bool
	log     *dialog.Dialog
	logList *virtuallist.List
	lines   []string

	aboutOpen bool
	about     *dialog.Dialog

	settingsOpen bool
	settings     *dialog.Dialog
	prefs        *prefsView
}

func dialogStyles(width int) styles.Styles {
	return styles.Styles{
		Background:   colSurface,
		BorderRadius: 14,
		Padding:      properties.UniformInset(22),
		Width:        properties.Width(width),
		BoxShadow:    properties.BoxShadow{Y: 18, Blur: 48, Color: "#000000e6"},
	}
}

func fieldStyles() styles.Styles {
	return styles.Styles{Background: colSurface2, Color: colText, BorderColor: colBorder, BorderRadius: 8, Padding: properties.SymmetricInset(12, 10)}
}

func primaryButton(label string, fn func()) *button.Button {
	return button.New(label, styles.Styles{Background: colHighlight, Color: colOnLight, FontSize: 14,
		FontWeight: properties.FontWeightSemiBold, Padding: properties.SymmetricInset(18, 9), BorderRadius: 8}).OnClick(fn)
}

func secondaryButton(label string, fn func()) *button.Button {
	return button.New(label, styles.Styles{Background: colSurface3, Color: colText, FontSize: 14,
		Padding: properties.SymmetricInset(18, 9), BorderRadius: 8}).OnClick(fn)
}

func heading(s string) elements.Element {
	return elements.Text(s, styles.Styles{Color: colText, FontSize: 18, FontWeight: properties.FontWeightSemiBold})
}

func note(s string) elements.Element {
	return elements.Text(s, styles.Styles{Color: colMuted, FontSize: 13})
}

func column(gap int, els ...elements.Element) elements.Element {
	return elements.Box(styles.Styles{Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: properties.Gap(gap)}, els...)
}

func buttonsRow(els ...elements.Element) elements.Element {
	all := append([]elements.Element{elements.Box(styles.Styles{FlexGrow: 1, Width: -1})}, els...)
	return elements.Row(styles.Styles{Width: -1, Gap: 8, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap, Margin: properties.Inset{Top: 6}}, all...)
}

func newDialogs(u *ui) *dialogs {
	a := u.a
	d := &dialogs{u: u}

	// Open URL.
	d.urlInput = input.New("https://…", fieldStyles()).Keyboard(input.KeyboardURL).
		Submit(func(s string) { d.urlOpen = false; a.openURL(s) })
	openBtn := primaryButton("Open", func() { d.urlOpen = false; a.openURL(d.urlInput.Text()) })
	cancelURL := secondaryButton("Cancel", func() { d.urlOpen = false })
	d.url = dialog.New(&d.urlOpen, func() elements.Element {
		return column(12,
			heading("Open URL"),
			note("A video or stream link, or a page from YouTube, Vimeo, Twitch and thousands of other sites."),
			d.urlInput,
			buttonsRow(cancelURL, openBtn))
	}, dialogStyles(520))

	// Go to time.
	d.jumpInput = input.New("1:23:45, 83, or 50%", fieldStyles()).
		Submit(func(s string) { d.jumpTo(s) })
	goBtn := primaryButton("Go", func() { d.jumpTo(d.jumpInput.Text()) })
	cancelJump := secondaryButton("Cancel", func() { d.jumpOpen = false })
	d.jump = dialog.New(&d.jumpOpen, func() elements.Element {
		return column(12, heading("Go to Time"), d.jumpInput, buttonsRow(cancelJump, goBtn))
	}, dialogStyles(360))

	// Cast.
	rescan := secondaryButton("Search Again", a.startCastScan)
	stopCast := secondaryButton("Stop Casting", func() { a.stopCasting(); d.castOpen = false })
	closeCast := secondaryButton("Close", func() { d.castOpen = false })
	d.cast = dialog.New(&d.castOpen, func() elements.Element { return d.castContent(rescan, stopCast, closeCast) }, dialogStyles(460))

	// Command palette.
	d.paletteInput = input.New("Type a command…", fieldStyles()).
		Submit(func(string) { d.runMatch(0) })
	d.paletteList = virtuallist.New(func() int { return len(d.matches) }, 40, d.paletteRow, styles.Styles{BorderColor: colBorder}).
		OnActivate(d.runMatch).ClickFocusesRow(false)
	d.palette = dialog.New(&d.paletteOpen, func() elements.Element {
		d.matches = a.actions.Search(d.paletteInput.Text())
		h := min(len(d.matches), 9) * 40
		if h == 0 {
			return column(10, d.paletteInput, note("No matching command."))
		}
		return column(10, d.paletteInput, elements.Box(styles.Styles{Width: -1, Height: properties.Height(h)}, d.paletteList))
	}, dialogStyles(560))

	// Log.
	copyLog := secondaryButton("Copy", func() { a.win.SetClipboard(strings.Join(d.lines, "\n")); a.toast("Log copied") })
	closeLog := secondaryButton("Close", func() { d.logOpen = false })
	d.logList = virtuallist.New(func() int { return len(d.lines) }, 20, func(i int) elements.Element {
		if i >= len(d.lines) {
			return elements.Box(styles.Styles{})
		}
		return elements.Text(d.lines[i], styles.Styles{Color: colMuted, FontSize: 11, MaxLines: 1, TextOverflow: properties.TextOverflowEllipsis})
	}, styles.Styles{BorderColor: colBorder})
	d.log = dialog.New(&d.logOpen, func() elements.Element {
		d.lines = a.logLines()
		body := elements.Element(elements.Box(styles.Styles{Width: -1, Height: 420}, d.logList))
		if len(d.lines) == 0 {
			body = note("Nothing logged yet. Raise “mpv log detail” in Settings › Advanced for more.")
		}
		return column(12, heading("mpv Log"), body, buttonsRow(copyLog, closeLog))
	}, dialogStyles(760))

	// About.
	site := secondaryButton("Website", func() { openWebPage("https://github.com/Mehran9675/goplay") })
	closeAbout := primaryButton("Close", func() { d.aboutOpen = false })
	d.about = dialog.New(&d.aboutOpen, func() elements.Element {
		mpvVersion, _ := a.player.Get("mpv-version")
		ffVersion, _ := a.player.Get("ffmpeg-version")
		return column(10,
			elements.Row(styles.Styles{Gap: 12, Align: properties.AlignCenter}, ic("logo", 40, colHighlight), heading("goplay")),
			note("Version "+Version),
			note("Plays nearly anything with "+mpvVersion+" and FFmpeg "+ffVersion+"."),
			note("Interface built with Giode and Gio. Icons from Material Design."),
			buttonsRow(site, closeAbout))
	}, dialogStyles(440))

	d.prefs = newPrefsView(u)
	d.settings = dialog.New(&d.settingsOpen, d.prefs.content, styles.Styles{
		Background: colSurface, BorderRadius: 16, Padding: properties.UniformInset(0),
		BoxShadow: properties.BoxShadow{Y: 18, Blur: 48, Color: "#000000e6"},
	})
	return d
}

func (d *dialogs) layout(gtx layout.Context) {
	for _, dl := range []*dialog.Dialog{d.url, d.jump, d.cast, d.palette, d.log, d.about, d.settings} {
		dl.Layout(gtx)
	}
	if !d.castOpen && d.u.a.cast.scanning {
		d.u.a.stopCastScan()
	}
}

func (d *dialogs) anyOpen() bool {
	return d.urlOpen || d.jumpOpen || d.castOpen || d.paletteOpen || d.logOpen || d.aboutOpen || d.settingsOpen
}

// closeTop closes the open dialog; false if none was.
func (d *dialogs) closeTop() bool {
	for _, f := range []*bool{&d.settingsOpen, &d.paletteOpen, &d.urlOpen, &d.jumpOpen, &d.castOpen, &d.logOpen, &d.aboutOpen} {
		if *f {
			if f == &d.settingsOpen && d.prefs.capture != "" {
				d.prefs.capture = ""
				return true
			}
			*f = false
			return true
		}
	}
	return false
}

func (u *ui) showOpenURL() {
	d := u.dialogs
	text := strings.TrimSpace(u.a.win.Clipboard())
	if !library.IsURL(text) {
		text = ""
	}
	d.urlInput.SetText(text)
	d.urlOpen = true
	d.urlInput.Focus()
	d.urlInput.SelectAll()
}

func (u *ui) showJumpTo() {
	d := u.dialogs
	d.jumpInput.SetText(player.FormatTime(u.a.player.State.Position))
	d.jumpOpen = true
	d.jumpInput.Focus()
	d.jumpInput.SelectAll()
}

// jumpTo seeks to a time typed as h:mm:ss, m:ss, seconds or a percentage.
func (d *dialogs) jumpTo(s string) {
	a := d.u.a
	t, ok := parseTimeInput(s, a.player.State.Duration)
	if !ok {
		a.toastError("That isn't a time: try 1:23:45, 83 or 50%")
		return
	}
	d.jumpOpen = false
	a.seekTo(t)
}

func parseTimeInput(s string, duration float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		f, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil || duration <= 0 {
			return 0, false
		}
		return duration * f / 100, true
	}
	var total float64
	for _, part := range strings.Split(s, ":") {
		f, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || f < 0 {
			return 0, false
		}
		total = total*60 + f
	}
	return total, s != ""
}

func (u *ui) showCast() {
	u.dialogs.castOpen = true
	if !u.a.casting() {
		u.a.startCastScan()
	}
}

func (d *dialogs) castContent(rescan, stop, closeBtn *button.Button) elements.Element {
	a := d.u.a
	c := &a.cast
	rows := []elements.Element{heading("Cast to a device")}
	if a.casting() {
		rows = append(rows, note("Playing on "+c.device+"."), buttonsRow(stop, closeBtn))
		return column(12, rows...)
	}
	rows = append(rows, note("Chromecast, Google TV and Android TV, and smart TVs, consoles and receivers that support DLNA."))
	if c.connecting != "" {
		rows = append(rows, elements.Row(styles.Styles{Gap: 10, Align: properties.AlignCenter},
			spinner.Spinner(styles.Styles{Width: 18, Height: 18, Color: colText}), note("Connecting to "+c.connecting+"…")))
	}
	for _, dev := range c.devices {
		rows = append(rows, d.deviceRow(dev))
	}
	if c.scanning {
		rows = append(rows, elements.Row(styles.Styles{Gap: 10, Align: properties.AlignCenter, Padding: properties.SymmetricInset(4, 6)},
			spinner.Spinner(styles.Styles{Width: 18, Height: 18, Color: colMuted}), note("Looking for devices on your network…")))
	} else if len(c.devices) == 0 {
		rows = append(rows, note("No devices found. Make sure they're on and on the same network as this computer."))
	}
	rows = append(rows, buttonsRow(rescan, closeBtn))
	return column(10, rows...)
}

func (d *dialogs) deviceRow(dev cast.Device) elements.Element {
	a := d.u.a
	return elements.Keyed("cast-device", dev.ID(), func() *elements.Pressable {
		return elements.NewPressable(func(s elements.PressState) elements.Element {
			bg := colSurface2
			if s.Hovered {
				bg = colSurface3
			}
			icon := "tv"
			if dev.Kind() == "Chromecast" {
				icon = "cast"
			}
			sub := dev.Kind()
			if dev.Model() != "" {
				sub += " · " + dev.Model()
			}
			return elements.Row(styles.Styles{Width: -1, Gap: 12, Align: properties.AlignCenter, Padding: properties.SymmetricInset(12, 10),
				Background: bg, BorderRadius: 10, Wrap: properties.WrapNoWrap},
				ic(icon, 22, colText),
				elements.Box(styles.Styles{FlexGrow: 1, Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 2},
					elements.Text(dev.Name(), styles.Styles{Color: colText, FontSize: 14, FontWeight: properties.FontWeightMedium}),
					elements.Text(sub, styles.Styles{Color: colFaint, FontSize: 12})),
			)
		}).Label(dev.Name()).OnClick(func(elements.ClickEvent) {
			d.castOpen = false
			a.castTo(dev)
		})
	})
}

func (u *ui) showPalette() {
	d := u.dialogs
	d.paletteInput.SetText("")
	d.paletteOpen = true
	d.paletteInput.Focus()
}

func (d *dialogs) runMatch(i int) {
	if i < 0 || i >= len(d.matches) {
		return
	}
	act := d.matches[i]
	d.paletteOpen = false
	if act.IsEnabled() {
		elements.AfterLayout(act.Run)
	}
}

func (d *dialogs) paletteRow(i int) elements.Element {
	if i >= len(d.matches) {
		return elements.Box(styles.Styles{})
	}
	act := d.matches[i]
	a := d.u.a
	return elements.Keyed("palette", act.ID, func() *elements.Pressable {
		return elements.NewPressable(nil)
	}).Content(func(s elements.PressState) elements.Element {
		bg := ""
		if s.Hovered || i == 0 {
			bg = colSurface2
		}
		titleColor := colText
		if !act.IsEnabled() {
			titleColor = colFaint
		}
		children := []elements.Element{
			elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Text(act.Title, styles.Styles{Color: titleColor, FontSize: 14})),
			elements.Text(act.Category, styles.Styles{Color: colFaint, FontSize: 12}),
		}
		if ks := a.actions.Keys(act.ID); len(ks) > 0 {
			children = append(children, keyChip(actions.Display(ks[0]), false))
		}
		return elements.Row(styles.Styles{Width: -1, Height: -1, Gap: 10, Align: properties.AlignCenter, Padding: properties.SymmetricInset(10, 0),
			Background: bg, BorderRadius: 8, Wrap: properties.WrapNoWrap}, children...)
	}).Label(act.Title).OnClick(func(elements.ClickEvent) { d.runMatch(i) })
}

// keyChip shows a shortcut like a keycap.
func keyChip(text string, highlight bool) elements.Element {
	bg, fg := colSurface3, colText
	if highlight {
		bg, fg = colHighlight, colOnLight
	}
	return elements.Box(styles.Styles{Background: bg, BorderRadius: 5, Padding: properties.SymmetricInset(7, 3)},
		elements.Text(text, styles.Styles{Color: fg, FontSize: 12, FontWeight: properties.FontWeightMedium}))
}

func (u *ui) showLog()   { u.dialogs.logOpen = true }
func (u *ui) showAbout() { u.dialogs.aboutOpen = true }
