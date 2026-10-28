package ui

import (
	"image"
	"math"
	"strconv"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/mehran9675/giode"
	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/checkbox"
	"github.com/mehran9675/giode/components/dropdown"
	"github.com/mehran9675/giode/components/iconbutton"
	"github.com/mehran9675/giode/components/slider"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/goplay-ebiten/internal/settings"
)

// Settings dialog sizes in dp.
const (
	prefsWidth    = 600
	selectWidth   = 230
	rangeSlider   = 160
	rangeValue    = 60
	prefsRowGap   = 10
	prefsFontSize = 14
)

// Dialog palette.
const (
	fieldBg     = "#1f2126"
	fieldBorder = "#34373e"
	fieldHover  = "#2b2e34"
	tabSelected = "#2a2c32"
)

// prefsView is the settings dialog's content: a tab per settings
// section, built from settings.Defs, with one control per setting.
type prefsView struct {
	u        *UI
	section  int
	tabs     []*tabButton
	controls map[string]*prefControl

	closeBtn *iconbutton.Button
	confBtn  *button.Button
	resetBtn *button.Button
	doneBtn  *button.Button
}

// prefControl edits one setting: a checkbox, select or slider by Kind.
type prefControl struct {
	def     *settings.Def
	check   *checkbox.Checkbox
	sel     *dropdown.Select
	options []settings.Option // sel's choices; audio devices change
	slide   *slider.Slider
}

func newPrefsView(u *UI) *prefsView {
	p := &prefsView{u: u, controls: map[string]*prefControl{}}
	for i, name := range settings.Sections {
		p.tabs = append(p.tabs, &tabButton{label: name, index: i, view: p})
	}
	for i := range settings.Defs {
		d := &settings.Defs[i]
		p.controls[d.Key] = p.newControl(d, d.Options)
	}

	p.closeBtn = iconbutton.New(glyph("close", 18, mutedColor), giode.Styles{
		Color: iconColor, Width: 32, Height: 32, Padding: giode.UniformInset(7), BorderRadius: 8,
	}).Variant(giode.IconButtonGhost).Label("Close").OnClick(p.close)
	outlined := giode.Styles{Color: fgColor, BorderColor: fieldBorder, BorderRadius: 8, Padding: giode.SymmetricInset(14, 8), FontSize: 13}
	p.confBtn = button.New("Edit mpv.conf…", outlined).Variant(button.VariantOutlined).
		OnClick(func() { p.with(Controller.EditMPVConf) })
	p.resetBtn = button.New("Reset to defaults", outlined).Variant(button.VariantOutlined).
		OnClick(func() {
			p.with(Controller.ResetSettings)
			p.refresh()
		})
	p.doneBtn = button.New("Done", giode.Styles{
		Background: highlight, Color: onHighlight, BorderRadius: 8, Padding: giode.SymmetricInset(18, 8), FontSize: 13,
	}).OnClick(p.close)
	return p
}

// newControl builds the control for d, with options as a Choice's
// choices.
func (p *prefsView) newControl(d *settings.Def, options []settings.Option) *prefControl {
	c := &prefControl{def: d, options: options}
	set := func(v string) {
		if p.u.c != nil {
			p.u.c.SetSetting(d.Key, v)
		}
	}
	switch d.Kind {
	case settings.Toggle:
		c.check = checkbox.New("", giode.Styles{Background: highlight, BorderColor: "#5d626c", BorderRadius: 4}).
			Icon(iconSrc("check"), giode.Styles{Color: onHighlight}).
			OnChange(func(on bool) {
				v := "no"
				if on {
					v = "yes"
				}
				set(v)
			})
	case settings.Choice:
		labels := make([]string, len(options))
		for i, o := range options {
			labels[i] = o.Label
		}
		c.sel = dropdown.New(giode.Styles{
			Background: fieldBg, Color: fgColor, BorderColor: fieldBorder, BorderRadius: 6,
			Width: selectWidth,
		}, labels...).HoverColor(fieldHover).OnChange(func(i int) {
			if i >= 0 && i < len(c.options) {
				set(c.options[i].Value)
			}
		})
	case settings.Range:
		c.slide = slider.New(giode.Styles{Color: highlight, Background: trackColor, BorderColor: thumbColor}).
			OnChange(func(v float32) { set(formatRange(d, rangeValueAt(d, v))) })
	}
	return c
}

// refresh reloads what can change while the dialog is closed: the
// audio devices, and every control's value.
func (p *prefsView) refresh() {
	if p.u.c != nil {
		d := settings.Lookup(settings.KeyAudioDevice)
		p.controls[d.Key] = p.newControl(d, p.u.c.AudioDevices())
	}
	p.sync()
}

// sync sets every control from the stored values, except one the user
// is operating.
func (p *prefsView) sync() {
	if p.u.c == nil {
		return
	}
	store := p.u.c.Settings()
	for key, c := range p.controls {
		v := store.Get(key)
		switch {
		case c.check != nil:
			c.check.SetChecked(v == "yes")
		case c.sel != nil && !c.sel.Opened():
			i := 0
			for j, o := range c.options {
				if o.Value == v {
					i = j
					break
				}
			}
			c.sel.SetSelected(i)
		case c.slide != nil && !c.slide.Dragging():
			f, _ := strconv.ParseFloat(v, 64)
			c.slide.SetValue(float32((f - c.def.Min) / (c.def.Max - c.def.Min)))
		}
	}
}

func (p *prefsView) close() { p.u.settingsOpen = false }

func (p *prefsView) with(fn func(Controller)) {
	if p.u.c != nil {
		fn(p.u.c)
	}
}

// content builds the dialog body: header, section tabs, the section's
// settings and the footer.
func (p *prefsView) content() giode.Element {
	p.sync()

	header := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		giode.Box(giode.Styles{FlexGrow: 1, Width: -1},
			giode.Text("Settings", giode.Styles{Color: fgColor, FontSize: 18, FontWeight: giode.FontWeightMedium})),
		p.closeBtn,
	)
	tabs := make([]giode.Element, len(p.tabs))
	for i, t := range p.tabs {
		tabs[i] = t
	}
	tabRow := giode.Row(giode.Styles{Gap: 4, Wrap: giode.WrapNoWrap}, tabs...)

	section := settings.Sections[p.section]
	var rows []giode.Element
	for i := range settings.Defs {
		d := &settings.Defs[i]
		if d.Section == section {
			rows = append(rows, p.row(p.controls[d.Key]))
		}
	}
	if section == "Advanced" {
		rows = append(rows, formRow(
			prefLabel("Other mpv options", "Any mpv option, in mpv.conf. Applied on restart."),
			p.confBtn, 0))
	}

	footer := giode.Row(giode.Styles{Width: -1, Align: giode.AlignCenter, Wrap: giode.WrapNoWrap},
		p.resetBtn,
		giode.Box(giode.Styles{Width: -1}),
		p.doneBtn,
	)

	divider := giode.Box(giode.Styles{Width: -1, Height: 1, Background: menuSeparator})
	return giode.Box(giode.Styles{
		Width:   prefsWidth,
		Padding: giode.Inset{Top: 16, Right: 20, Bottom: 20, Left: 24},
	}, vstack(14,
		header,
		tabRow,
		divider,
		vstack(prefsRowGap, rows...),
		divider,
		footer,
	))
}

// row lays out one setting: its label, and its control on the right.
func (p *prefsView) row(c *prefControl) giode.Element {
	label := c.def.Label
	hint := ""
	if c.def.Restart {
		hint = "Applied on restart."
	}
	switch {
	case c.check != nil:
		return formRow(prefLabel(label, hint), c.check, 0)
	case c.sel != nil:
		return formRow(prefLabel(label, hint), c.sel, selectWidth)
	default:
		v := p.u.c.Settings().Get(c.def.Key)
		f, _ := strconv.ParseFloat(v, 64)
		return formRow(prefLabel(label, hint), rangeControl(c.slide, formatRange(c.def, f)+c.def.Unit), rangeSlider+12+rangeValue)
	}
}

// prefLabel is a setting's name, with an optional hint below it.
func prefLabel(label, hint string) giode.Element {
	text := giode.Text(label, giode.Styles{Color: fgColor, FontSize: prefsFontSize})
	if hint == "" {
		return text
	}
	return giode.Stack(giode.Styles{Gap: 2},
		text,
		giode.Text(hint, giode.Styles{Color: mutedColor, FontSize: 12}))
}

// rangeControl is a slider with its value to the right.
func rangeControl(s *slider.Slider, value string) giode.Element {
	return elements.Raw(func(gtx layout.Context) layout.Dimensions {
		sw := gtx.Dp(rangeSlider)
		vw := gtx.Dp(rangeValue)
		sgtx := gtx
		sgtx.Constraints = layout.Exact(image.Pt(sw, gtx.Dp(24)))
		sdims := place(sgtx, image.Point{}, s)

		vgtx := gtx
		vgtx.Constraints = layout.Constraints{Max: image.Pt(vw, gtx.Constraints.Max.Y)}
		call, vdims := measured(vgtx, giode.Text(value, giode.Styles{Color: mutedColor, FontSize: 13}))
		pos := image.Pt(gtx.Constraints.Max.X-vdims.Size.X, (sdims.Size.Y-vdims.Size.Y)/2)
		place(gtx, pos, elements.Raw(func(gtx layout.Context) layout.Dimensions {
			call.Add(gtx.Ops)
			return vdims
		}))
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, sdims.Size.Y)}
	})
}

// rangeValueAt maps slider position v in [0, 1] to d's range, snapped
// to its step.
func rangeValueAt(d *settings.Def, v float32) float64 {
	f := d.Min + math.Round(float64(v)*(d.Max-d.Min)/d.Step)*d.Step
	return math.Min(d.Max, math.Max(d.Min, f))
}

// formatRange formats a Range value with as many decimals as its step.
func formatRange(d *settings.Def, f float64) string {
	decimals := 0
	for s := d.Step; s < 1 && decimals < 3; s *= 10 {
		decimals++
	}
	return strconv.FormatFloat(f, 'f', decimals, 64)
}

// tabButton selects a settings section.
type tabButton struct {
	label string
	index int
	view  *prefsView
	click widget.Clickable
}

func (t *tabButton) Layout(gtx layout.Context) layout.Dimensions {
	if t.click.Clicked(gtx) {
		t.view.section = t.index
	}
	return t.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		st := giode.Styles{
			Padding:      giode.SymmetricInset(12, 7),
			BorderRadius: 6,
		}
		color := mutedColor
		switch {
		case t.view.section == t.index:
			st.Background = tabSelected
			color = fgColor
		case t.click.Hovered():
			st.Background = fieldBg
			color = fgColor
		}
		if t.click.Hovered() {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return giode.Box(st, giode.Text(t.label, giode.Styles{Color: properties.Color(color), FontSize: 13})).Layout(gtx)
	})
}
