package app

import (
	"strconv"

	"github.com/mehran9675/giode/components/iconbutton"
	"github.com/mehran9675/giode/components/segmented"
	"github.com/mehran9675/giode/components/virtuallist"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/goplay/internal/player"
)

const (
	panelPlaylist = iota
	panelChapters
)

// sidePanel slides in from the right with the playlist and chapters.
type sidePanel struct {
	u        *ui
	tabs     *segmented.Control
	playlist *virtuallist.List
	chapters *virtuallist.List
	close    *iconbutton.Button
	add      *iconbutton.Button
	shuffle  *iconbutton.Button
	clear    *iconbutton.Button
}

func newSidePanel(u *ui) *sidePanel {
	a := u.a
	p := &sidePanel{u: u}
	p.tabs = segmented.New(tr("Playlist"), tr("Chapters")).OnChange(func(i int) { u.panelTab = i })
	p.tabs.Styles(styles.Styles{Width: -1, Background: colSurface2, BorderColor: colSurface3, Color: colText, FontSize: 13})
	st := &a.player.State
	p.playlist = virtuallist.New(func() int { return len(st.Playlist) }, 52, gapped(p.playlistRow),
		listStyles()).
		OnActivate(func(i int) { a.player.PlaylistPlay(i) }).
		ClickFocusesRow(false)
	p.chapters = virtuallist.New(func() int { return len(st.Chapters) }, 44, gapped(p.chapterRow),
		listStyles()).
		OnActivate(func(i int) { a.player.SetChapter(i) }).
		ClickFocusesRow(false)
	p.close = ghostButton("close", 18, tr("Close"), func() { u.panelOpen = false })
	p.add = ghostButton("add", 20, tr("Add files"), a.addFilesDialog)
	p.shuffle = ghostButton("shuffle", 20, tr("Shuffle"), func() { a.actions.Run("shuffle") })
	p.clear = ghostButton("delete", 20, tr("Clear the rest"), func() { a.player.PlaylistClear() })
	return p
}

func (p *sidePanel) opened() {
	p.tabs.SetSelected(p.u.panelTab)
	p.scrollToCurrent()
}

func (p *sidePanel) scrollToCurrent() {
	st := &p.u.a.player.State
	if st.PlaylistPos >= 0 {
		p.playlist.ScrollToIndex(st.PlaylistPos, virtuallist.AlignNearest)
	}
	if st.Chapter >= 0 {
		p.chapters.ScrollToIndex(st.Chapter, virtuallist.AlignNearest)
	}
}

func (p *sidePanel) content() elements.Element {
	u := p.u
	st := &u.a.player.State
	p.tabs.SetSelected(u.panelTab)

	header := elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Gap: 8, Wrap: properties.WrapNoWrap},
		elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, p.tabs), p.close)

	var body elements.Element
	var footer elements.Element
	switch u.panelTab {
	case panelChapters:
		if len(st.Chapters) == 0 {
			body = emptyNote(tr("This file has no chapters."))
		} else {
			body = elements.Box(styles.Styles{FlexGrow: 1, Width: -1, Height: -1}, p.chapters)
		}
	default:
		if len(st.Playlist) == 0 {
			body = emptyNote(tr("Nothing queued. Open a folder, or add files."))
		} else {
			body = elements.Box(styles.Styles{FlexGrow: 1, Width: -1, Height: -1}, p.playlist)
		}
		count := tr("{count} items", "count", len(st.Playlist))
		footer = elements.Row(styles.Styles{Width: -1, Align: properties.AlignCenter, Gap: 4, Wrap: properties.WrapNoWrap},
			elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Text(count, styles.Styles{Color: colMuted, FontSize: 12})),
			p.add, p.shuffle, p.clear)
	}
	children := []elements.Element{header, body}
	if footer != nil {
		children = append(children, footer)
	}
	return elements.Box(styles.Styles{Width: -1, Height: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 12,
		Padding: properties.UniformInset(14)}, children...)
}

func emptyNote(s string) elements.Element {
	return elements.Box(styles.Styles{FlexGrow: 1, Width: -1, Height: -1, Justify: properties.JustifyCenter, Align: properties.AlignCenter},
		elements.Text(s, styles.Styles{Color: colFaint, FontSize: 13, TextAlign: properties.TextAlignCenter}))
}

// playlistRow is one playlist entry; rows are looked up by key (index
// and file), so their hover state survives scrolling.
func (p *sidePanel) playlistRow(i int) elements.Element {
	a := p.u.a
	st := &a.player.State
	if i >= len(st.Playlist) {
		return elements.Box(styles.Styles{})
	}
	e := st.Playlist[i]
	current := i == st.PlaylistPos
	remove := elements.Keyed("playlist-remove", e.Filename+"#"+strconv.Itoa(i), func() *iconbutton.Button {
		return ghostButton("close", 16, tr("Remove"), nil)
	})
	remove.OnClick(func() { elements.AfterLayout(func() { a.player.PlaylistRemove(i) }) })
	return elements.Keyed("playlist-row", e.Filename+"#"+strconv.Itoa(i), func() *elements.Pressable {
		return elements.NewPressable(nil)
	}).Content(func(s elements.PressState) elements.Element {
		bg := ""
		switch {
		case current:
			bg = colSurface3
		case s.Hovered:
			bg = colSurface2
		}
		var num elements.Element = elements.Text(strconv.Itoa(i+1), styles.Styles{Color: colFaint, FontSize: 12})
		if current {
			num = ic("play", 14, colHighlight)
		}
		titleColor := colText
		if !current && !s.Hovered {
			titleColor = "#d4d4da"
		}
		children := []elements.Element{
			elements.Box(styles.Styles{Width: 24, Justify: properties.JustifyCenter}, num),
			elements.Box(styles.Styles{FlexGrow: 1, Width: -1},
				elements.Text(e.Name(), styles.Styles{Color: titleColor, FontSize: 13, MaxLines: 2, TextOverflow: properties.TextOverflowEllipsis})),
		}
		if s.Hovered {
			children = append(children, remove)
		}
		return elements.Row(styles.Styles{Width: -1, Height: -1, Align: properties.AlignCenter, Gap: 8, Wrap: properties.WrapNoWrap,
			Padding: properties.SymmetricInset(8, 4), Background: bg, BorderRadius: 8}, children...)
	}).Label(e.Name()).OnClick(func(elements.ClickEvent) { a.player.PlaylistPlay(i) })
}

func (p *sidePanel) chapterRow(i int) elements.Element {
	a := p.u.a
	st := &a.player.State
	if i >= len(st.Chapters) {
		return elements.Box(styles.Styles{})
	}
	c := st.Chapters[i]
	current := i == st.Chapter
	title := c.Title
	if title == "" {
		title = tr("Chapter {number}", "number", i+1)
	}
	return elements.Keyed("chapter-row", strconv.Itoa(i)+title, func() *elements.Pressable {
		return elements.NewPressable(nil)
	}).Content(func(s elements.PressState) elements.Element {
		bg := ""
		switch {
		case current:
			bg = colSurface3
		case s.Hovered:
			bg = colSurface2
		}
		return elements.Row(styles.Styles{Width: -1, Height: -1, Align: properties.AlignCenter, Gap: 12, Wrap: properties.WrapNoWrap,
			Padding: properties.SymmetricInset(10, 4), Background: bg, BorderRadius: 8},
			elements.Box(styles.Styles{Width: 56}, elements.Text(player.FormatTime(c.Time), styles.Styles{Color: colMuted, FontSize: 12})),
			elements.Box(styles.Styles{FlexGrow: 1, Width: -1}, elements.Text(title, styles.Styles{Color: colText, FontSize: 13, MaxLines: 1, TextOverflow: properties.TextOverflowEllipsis})),
		)
	}).Label(title).OnClick(func(elements.ClickEvent) { a.player.SetChapter(i) })
}
