package app

import (
	"math"
	"strconv"

	"gioui.org/io/key"

	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/input"
	"github.com/mehran9675/giode/components/scroll"
	"github.com/mehran9675/giode/components/slider"
	"github.com/mehran9675/giode/components/toggle"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/actions"
	"github.com/mehran9675/goplay/internal/settings"
)

// The settings dialog: a page per section from settings.Defs, plus the
// shortcut editor.

const shortcutsPage = "Shortcuts"

type prefsView struct {
	u        *ui
	page     string
	controls map[string]*prefControl
	scroll   *scroll.Scroll
	keys     *scroll.Scroll

	// capture is the action whose shortcut is being recorded, and which
	// of its shortcuts (-1 adds one).
	capture    string
	captureIdx int

	done, reset, folder, mpvConf, inputConf, resetKeys *button.Button
}

type prefControl struct {
	def  *settings.Def
	sw   *toggle.Switch
	sel  *dropdown.Select
	opts []settings.Option
	sl   *slider.Slider
	txt  *input.Input
}

func newPrefsView(u *ui) *prefsView {
	a := u.a
	p := &prefsView{u: u, page: settings.Sections[0], controls: map[string]*prefControl{}}
	for i := range settings.Defs {
		d := &settings.Defs[i]
		p.controls[d.Key] = p.newControl(d, d.Options)
	}
	p.scroll = scroll.New(func() elements.Element { return clearOfBar(p.pageContent()) }, scrollStyles())
	p.keys = scroll.New(func() elements.Element { return clearOfBar(p.shortcutsContent()) }, scrollStyles())
	p.done = primaryButton("Done", func() { u.dialogs.settingsOpen = false })
	p.reset = secondaryButton("Reset All", func() {
		a.settings.Reset()
		for i := range settings.Defs {
			a.applySetting(settings.Defs[i].Key)
		}
		a.toast("Settings reset to their defaults")
	})
	p.folder = secondaryButton("Open Config Folder", a.openConfigFolder)
	p.mpvConf = secondaryButton("Edit mpv.conf", func() { a.editConfigFile("mpv.conf") })
	p.inputConf = secondaryButton("Edit input.conf", func() { a.editConfigFile("input.conf") })
	p.resetKeys = secondaryButton("Reset All Shortcuts", func() {
		a.settings.ResetKeys()
		for _, act := range a.actions.All() {
			a.actions.SetCustom(act.ID, nil)
		}
		a.toast("Shortcuts reset")
	})
	return p
}

func (p *prefsView) newControl(d *settings.Def, opts []settings.Option) *prefControl {
	a := p.u.a
	c := &prefControl{def: d, opts: opts}
	switch d.Kind {
	case settings.Toggle:
		c.sw = toggle.New("", styles.Styles{Background: colSwitchOn, BorderColor: colSurface3}).OnChange(func(on bool) {
			v := "no"
			if on {
				v = "yes"
			}
			a.setSetting(d.Key, v)
		})
	case settings.Choice:
		labels := make([]string, len(opts))
		for i, o := range opts {
			labels[i] = o.Label
		}
		c.sel = dropdown.New(styles.Styles{Background: colSurface2, Color: colText, BorderColor: colBorder,
			BorderRadius: 8, Width: 250, FontSize: 13}, labels...).HoverColor(colSurface3).OnChange(func(i int) {
			if i >= 0 && i < len(c.opts) {
				a.setSetting(d.Key, c.opts[i].Value)
			}
		})
	case settings.Range:
		c.sl = slider.New(styles.Styles{Color: colHighlight, Background: colTrack, BorderColor: colHighlight}).
			OnChange(func(v float32) { a.setSetting(d.Key, formatRange(d, rangeValue(d, v))) })
	case settings.Text:
		c.txt = input.New("", fieldStyles()).Styles(styles.Styles{Width: 250, Background: colSurface2, Color: colText,
			BorderColor: colBorder, BorderRadius: 8, FontSize: 13})
		apply := func() { a.setSetting(d.Key, c.txt.Text()) }
		if d.Lines > 1 {
			c.txt.Lines(d.Lines, d.Lines).Styles(styles.Styles{Width: -1, Background: colSurface2, Color: colText,
				BorderColor: colBorder, BorderRadius: 8, FontSize: 12})
			c.txt.SubmitOn(input.SubmitNever)
		} else {
			c.txt.Submit(func(string) { apply() })
		}
		c.txt.OnBlur(apply)
	}
	return c
}

// rangeValue maps slider position v to d's range, snapped to its step.
func rangeValue(d *settings.Def, v float32) float64 {
	f := d.Min + math.Round(float64(v)*(d.Max-d.Min)/d.Step)*d.Step
	return math.Min(d.Max, math.Max(d.Min, f))
}

// formatRange writes a Range value with as many decimals as its step.
func formatRange(d *settings.Def, f float64) string {
	decimals := 0
	for s := d.Step; s < 0.999 && decimals < 3; s *= 10 {
		decimals++
	}
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

func (u *ui) showSettings(page string) {
	d := u.dialogs
	if page != "" {
		d.prefs.page = page
	}
	d.prefs.refresh()
	d.settingsOpen = true
}

// refresh reloads what changes while the dialog is closed: the audio
// devices and every value.
func (p *prefsView) refresh() {
	d := settings.Lookup(settings.KeyAudioDevice)
	p.controls[d.Key] = p.newControl(d, p.u.a.audioDevices())
	p.sync()
}

// sync sets the controls from the stored values, except one in use.
func (p *prefsView) sync() {
	store := p.u.a.settings
	for key, c := range p.controls {
		v := store.Get(key)
		switch {
		case c.sw != nil:
			c.sw.SetChecked(v == "yes")
		case c.sel != nil && !c.sel.Opened():
			i := 0
			for j, o := range c.opts {
				if o.Value == v {
					i = j
					break
				}
			}
			c.sel.SetSelected(i)
		case c.sl != nil && !c.sl.Dragging():
			f, _ := strconv.ParseFloat(v, 64)
			c.sl.SetValue(float32((f - c.def.Min) / (c.def.Max - c.def.Min)))
		case c.txt != nil && !c.txt.Focused() && c.txt.Text() != v:
			c.txt.SetText(v)
		}
	}
}

func (p *prefsView) content() elements.Element {
	p.sync()
	pages := append(append([]string{}, settings.Sections...), shortcutsPage)
	nav := []elements.Element{elements.Box(styles.Styles{Padding: properties.Inset{Left: 10, Bottom: 10}}, heading("Settings"))}
	for _, pg := range pages {
		nav = append(nav, p.navItem(pg))
	}
	sidebar := elements.Box(styles.Styles{Width: 200, Height: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 2,
		Padding: properties.UniformInset(14), Background: "#111113"}, nav...)

	body := elements.Element(p.scroll)
	var footer []elements.Element
	switch p.page {
	case shortcutsPage:
		body = p.keys
		footer = append(footer, p.resetKeys)
	case "Advanced":
		footer = append(footer, p.folder, p.mpvConf, p.inputConf)
	default:
		footer = append(footer, p.reset)
	}
	footer = append(footer, elements.Box(styles.Styles{FlexGrow: 1, Width: -1}), p.done)
	main := elements.Box(styles.Styles{FlexGrow: 1, Width: -1, Height: -1, FlexDirection: properties.FlexDirectionColumn},
		elements.Box(styles.Styles{FlexGrow: 1, Width: -1, Height: -1, Padding: properties.Inset{Top: 18, Left: 24, Right: 12}}, body),
		elements.Row(styles.Styles{Width: -1, Gap: 8, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap,
			Padding: properties.SymmetricInset(20, 14), BorderColor: colBorder}, footer...),
	)
	// As large as comfortable, but never larger than the window.
	u := p.u
	w := min(860, int(float32(u.fb.X)/u.scale)-48)
	h := min(620, int(float32(u.fb.Y)/u.scale)-48)
	return elements.Row(styles.Styles{Width: properties.Width(max(w, 320)), Height: properties.Height(max(h, 300)), Wrap: properties.WrapNoWrap}, sidebar, main)
}

func (p *prefsView) navItem(page string) elements.Element {
	return elements.Keyed("settings-nav", page, func() *elements.Pressable {
		return elements.NewPressable(func(s elements.PressState) elements.Element {
			bg, fg := "", colMuted
			switch {
			case p.page == page:
				bg, fg = colSurface3, colText
			case s.Hovered:
				bg, fg = colSurface2, colText
			}
			return elements.Box(styles.Styles{Width: -1, Background: bg, BorderRadius: 8, Padding: properties.SymmetricInset(10, 8)},
				elements.Text(page, styles.Styles{Color: fg, FontSize: 13, FontWeight: properties.FontWeightMedium}))
		}).Label(page).OnClick(func(elements.ClickEvent) {
			p.page = page
			p.capture = ""
		})
	})
}

// pageContent is the current section's settings.
func (p *prefsView) pageContent() elements.Element {
	var rows []elements.Element
	for i := range settings.Defs {
		d := &settings.Defs[i]
		if d.Section == p.page {
			rows = append(rows, p.row(p.controls[d.Key]))
		}
	}
	return elements.Box(styles.Styles{Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 4, Padding: properties.Inset{Right: 12, Bottom: 12}}, rows...)
}

func (p *prefsView) row(c *prefControl) elements.Element {
	d := c.def
	labels := []elements.Element{elements.Text(d.Label, styles.Styles{Color: colText, FontSize: 14})}
	help := d.Help
	if d.Restart {
		if help != "" {
			help += " "
		}
		help += "Takes effect when GoPlay restarts."
	}
	if help != "" {
		labels = append(labels, elements.Text(help, styles.Styles{Color: colFaint, FontSize: 12}))
	}
	label := elements.Box(styles.Styles{FlexGrow: 1, Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 3}, labels...)
	var control elements.Element
	switch {
	case c.sw != nil:
		control = c.sw
	case c.sel != nil:
		control = c.sel
	case c.sl != nil:
		v, _ := strconv.ParseFloat(p.u.a.settings.Get(d.Key), 64)
		control = elements.Row(styles.Styles{Gap: 10, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap},
			elements.Box(styles.Styles{Width: 180, Height: 24, Align: properties.AlignCenter}, c.sl),
			elements.Box(styles.Styles{Width: 60, Justify: properties.JustifyEnd},
				elements.Text(formatRange(d, v)+d.Unit, styles.Styles{Color: colMuted, FontSize: 13})))
	case c.txt != nil && d.Lines > 1:
		return elements.Box(styles.Styles{Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 8, Padding: properties.SymmetricInset(0, 10)}, label, c.txt)
	case c.txt != nil:
		control = c.txt
	}
	return elements.Row(styles.Styles{Width: -1, Gap: 16, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap, Padding: properties.SymmetricInset(0, 10)},
		label, control)
}

// shortcutsContent lists every command with its shortcuts.
func (p *prefsView) shortcutsContent() elements.Element {
	a := p.u.a
	rows := []elements.Element{note("Click a shortcut to change it, then press the new keys. Backspace removes it; Escape cancels.")}
	for _, cat := range a.actions.Categories() {
		rows = append(rows, elements.Box(styles.Styles{Padding: properties.Inset{Top: 14, Bottom: 4}},
			elements.Text(cat, styles.Styles{Color: colMuted, FontSize: 12, FontWeight: properties.FontWeightSemiBold})))
		for _, act := range a.actions.All() {
			if act.Category != cat || act.Hidden {
				continue
			}
			rows = append(rows, p.shortcutRow(act))
		}
	}
	return elements.Box(styles.Styles{Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 2, Padding: properties.Inset{Right: 12, Bottom: 12}}, rows...)
}

func (p *prefsView) shortcutRow(act *actions.Action) elements.Element {
	a := p.u.a
	keys := a.actions.Keys(act.ID)
	chips := []elements.Element{}
	for i, k := range keys {
		chips = append(chips, p.chip(act.ID, i, actions.Display(k)))
	}
	if p.capture == act.ID && p.captureIdx < 0 {
		chips = append(chips, keyChip("Press keys…", true))
	} else if len(keys) < 3 {
		chips = append(chips, p.chip(act.ID, -1, "+"))
	}
	if a.actions.Customized(act.ID) {
		chips = append(chips, elements.Keyed("shortcut-reset", act.ID, func() *elements.Pressable {
			return elements.NewPressable(func(s elements.PressState) elements.Element {
				c := colFaint
				if s.Hovered {
					c = colText
				}
				return elements.Text("Reset", styles.Styles{Color: c, FontSize: 12})
			}).Label("Reset " + act.Title).OnClick(func(elements.ClickEvent) {
				a.actions.SetCustom(act.ID, nil)
				a.settings.SetKeys(act.ID, nil)
			})
		}))
	}
	return elements.Row(styles.Styles{Width: -1, Gap: 6, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap, Padding: properties.SymmetricInset(0, 5)},
		append([]elements.Element{elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Text(act.Title, styles.Styles{Color: colText, FontSize: 13}))}, chips...)...)
}

// chip is a clickable shortcut; i is its index, or -1 for "add".
func (p *prefsView) chip(id string, i int, text string) elements.Element {
	capturing := p.capture == id && p.captureIdx == i
	if capturing {
		text = "Press keys…"
	}
	return elements.Keyed("shortcut-chip", id+"#"+strconv.Itoa(i), func() *elements.Pressable {
		return elements.NewPressable(nil)
	}).Content(func(s elements.PressState) elements.Element {
		return keyChip(text, capturing || s.Hovered)
	}).Label("Change shortcut").Focusable(false).OnClick(func(elements.ClickEvent) {
		p.capture, p.captureIdx = id, i
	})
}

func (u *ui) capturingKey() bool {
	return u.dialogs.settingsOpen && u.dialogs.prefs.capture != ""
}

// captureKey records a key press as the shortcut being edited.
func (u *ui) captureKey(e key.Event) {
	p := u.dialogs.prefs
	a := u.a
	switch e.Name {
	case key.NameCtrl, key.NameShift, key.NameAlt, key.NameSuper, key.NameCommand:
		return // wait for the key itself
	case key.NameEscape:
		p.capture = ""
		return
	}
	id, idx := p.capture, p.captureIdx
	p.capture = ""
	keys := append([]string(nil), a.actions.Keys(id)...)
	if (e.Name == key.NameDeleteBackward || e.Name == key.NameDeleteForward) && e.Modifiers == 0 {
		if idx >= 0 && idx < len(keys) {
			keys = append(keys[:idx], keys[idx+1:]...)
			a.actions.SetCustom(id, keys)
			a.settings.SetKeys(id, keys)
		}
		return
	}
	sc := actions.FromEvent(e)
	if owner := a.actions.Owner(sc); owner != nil && owner.ID != id {
		var rest []string
		for _, k := range a.actions.Keys(owner.ID) {
			if actions.Normalize(k) != sc {
				rest = append(rest, k)
			}
		}
		a.actions.SetCustom(owner.ID, rest)
		a.settings.SetKeys(owner.ID, rest)
		a.toast(actions.Display(sc) + " moved from “" + owner.Title + "”")
	}
	if idx >= 0 && idx < len(keys) {
		keys[idx] = sc
	} else {
		keys = append(keys, sc)
	}
	a.actions.SetCustom(id, keys)
	a.settings.SetKeys(id, keys)
}

// audioDevices lists mpv's outputs for the device setting.
func (a *App) audioDevices() []settings.Option {
	out := []settings.Option{{Label: "Automatic", Value: "auto"}}
	var list []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if a.player.Handle().JSON("audio-device-list", &list) != nil {
		return out
	}
	for _, d := range list {
		if d.Name == "" || d.Name == "auto" {
			continue
		}
		label := d.Description
		if label == "" {
			label = d.Name
		}
		out = append(out, settings.Option{Label: label, Value: d.Name})
	}
	return out
}

// scrollStyles fill the settings page and use goplay's scroll bar.
func scrollStyles() styles.Styles {
	return styles.Styles{Width: -1, Height: -1, ScrollBar: scrollBar()}
}

// clearOfBar keeps a scrolling page's content clear of its scroll bar.
func clearOfBar(el elements.Element) elements.Element {
	return elements.Box(styles.Styles{Width: -1, Padding: properties.Inset{Right: scrollGap}}, el)
}
