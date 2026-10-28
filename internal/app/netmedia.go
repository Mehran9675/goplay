package app

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"

	"github.com/mehran9675/giode/components/button"
	"github.com/mehran9675/giode/components/dialog"
	"github.com/mehran9675/giode/components/spinner"
	"github.com/mehran9675/giode/components/virtuallist"
	"github.com/mehran9675/giode/elements"
	"github.com/mehran9675/giode/styles"
	"github.com/mehran9675/giode/styles/properties"

	"github.com/mehran9675/goplay/internal/cast"
	"github.com/mehran9675/goplay/internal/player"
)

// netMedia browses the media servers on the network (DLNA/UPnP: a NAS,
// Plex, Jellyfin, Windows' media sharing) and plays what they share.
type netMedia struct {
	u    *ui
	open bool
	dlg  *dialog.Dialog
	list *virtuallist.List

	scanMu     sync.Mutex
	scanCancel context.CancelFunc
	scanning   bool
	servers    []*cast.MediaServer

	// server is the one being browsed, path the folders from its top
	// down to the one listed, entries that folder's contents.
	server  *cast.MediaServer
	path    []netFolder
	entries []cast.MediaEntry
	loading bool
	err     string
	// gen tells a listing that arrives late from one still wanted.
	gen int

	back, rescan, playAll, closeBtn *button.Button
}

type netFolder struct{ id, title string }

func newNetMedia(u *ui) *netMedia {
	n := &netMedia{u: u}
	n.list = virtuallist.New(func() int { return len(n.entries) }, 52, gapped(n.row), listStyles()).
		OnActivate(n.activate).
		ClickFocusesRow(false)
	n.back = secondaryButton("Back", n.up)
	n.rescan = secondaryButton("Search Again", n.scan)
	n.playAll = primaryButton("Play All", func() { n.play(0) })
	n.closeBtn = secondaryButton("Close", func() { n.open = false })
	n.dlg = dialog.New(&n.open, n.content, dialogStyles(600))
	return n
}

// show opens the window, looking for servers when none is being
// browsed.
func (n *netMedia) show() {
	n.open = true
	if n.server == nil {
		n.scan()
	}
}

func (n *netMedia) layout(gtx layout.Context) {
	n.dlg.Layout(gtx)
	if !n.open && n.scanning {
		n.stopScan()
	}
}

// scan looks for media servers for a while; they arrive through Post.
func (n *netMedia) scan() {
	a := n.u.a
	n.scanMu.Lock()
	if n.scanCancel != nil {
		n.scanCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	n.scanCancel = cancel
	n.scanMu.Unlock()
	n.scanning = true
	n.servers = nil
	n.server, n.path, n.entries, n.err = nil, nil, nil, ""
	go func() {
		cast.DiscoverMediaServers(ctx, func(s *cast.MediaServer) {
			a.Post(func() {
				n.servers = append(n.servers, s)
				sortServers(n.servers)
			})
		})
		a.Post(func() { n.scanning = false })
	}()
}

func (n *netMedia) stopScan() {
	n.scanMu.Lock()
	if n.scanCancel != nil {
		n.scanCancel()
		n.scanCancel = nil
	}
	n.scanMu.Unlock()
	n.scanning = false
}

func sortServers(ss []*cast.MediaServer) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && strings.ToLower(ss[j].Name) < strings.ToLower(ss[j-1].Name); j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// openServer starts browsing s at its top folder.
func (n *netMedia) openServer(s *cast.MediaServer) {
	n.stopScan()
	n.server = s
	n.path = []netFolder{{id: cast.RootID, title: s.Name}}
	n.load()
}

// load lists the folder at the end of the path.
func (n *netMedia) load() {
	a := n.u.a
	n.gen++
	gen, s, id := n.gen, n.server, n.path[len(n.path)-1].id
	n.loading, n.err, n.entries = true, "", nil
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		entries, err := s.Browse(ctx, id)
		a.Post(func() {
			if gen != n.gen {
				return
			}
			n.loading = false
			if err != nil {
				n.err = "Couldn't open this folder: " + err.Error()
				return
			}
			n.entries = entries
			n.list.ScrollToIndex(0, virtuallist.AlignStart)
		})
	}()
}

// up goes to the parent folder, or back to the list of servers.
func (n *netMedia) up() {
	if len(n.path) > 1 {
		n.path = n.path[:len(n.path)-1]
		n.load()
		return
	}
	n.gen++
	n.server, n.path, n.entries, n.err, n.loading = nil, nil, nil, "", false
	if len(n.servers) == 0 {
		n.scan()
	}
}

// activate opens a folder, or plays the folder's files from entry i.
func (n *netMedia) activate(i int) {
	if i < 0 || i >= len(n.entries) {
		return
	}
	if e := n.entries[i]; e.Folder {
		n.path = append(n.path, netFolder{id: e.ID, title: e.Title})
		n.load()
		return
	}
	n.play(i)
}

// play plays the listed files from entry i on, as a playlist.
func (n *netMedia) play(from int) {
	var urls []string
	for _, e := range n.entries[from:] {
		if !e.Folder {
			urls = append(urls, e.URL)
		}
	}
	if len(urls) == 0 {
		return
	}
	n.open = false
	n.u.a.openItems(urls, false)
}

func (n *netMedia) files() int {
	c := 0
	for _, e := range n.entries {
		if !e.Folder {
			c++
		}
	}
	return c
}

func (n *netMedia) content() elements.Element {
	if n.server == nil {
		return n.serversContent()
	}
	crumbs := make([]string, len(n.path))
	for i, f := range n.path {
		crumbs[i] = f.title
	}
	rows := []elements.Element{heading(n.path[len(n.path)-1].title)}
	if len(crumbs) > 1 {
		rows = append(rows, note(strings.Join(crumbs, "  ›  ")))
	}
	switch {
	case n.loading:
		rows = append(rows, busy("Opening…"))
	case n.err != "":
		rows = append(rows, note(n.err))
	case len(n.entries) == 0:
		rows = append(rows, note("There's no video or music in this folder."))
	default:
		h := min(len(n.entries), 8) * 52
		rows = append(rows, elements.Box(styles.Styles{Width: -1, Height: properties.Height(h)}, n.list))
	}
	btns := []elements.Element{n.back}
	btns = append(btns, elements.Box(styles.Styles{FlexGrow: 1, Width: -1}))
	if !n.loading && n.files() > 0 {
		btns = append(btns, n.playAll)
	}
	btns = append(btns, n.closeBtn)
	rows = append(rows, elements.Row(styles.Styles{Width: -1, Gap: 8, Align: properties.AlignCenter, Wrap: properties.WrapNoWrap,
		Margin: properties.Inset{Top: 6}}, btns...))
	return column(10, rows...)
}

func (n *netMedia) serversContent() elements.Element {
	rows := []elements.Element{
		heading("Media on your network"),
		note("Videos and music shared by computers, NAS drives and media servers (DLNA/UPnP) on this network."),
	}
	for _, s := range n.servers {
		rows = append(rows, n.serverRow(s))
	}
	if n.scanning {
		rows = append(rows, busy("Looking for media servers…"))
	} else if len(n.servers) == 0 {
		rows = append(rows, note("No media servers found. Make sure sharing is on and they're on the same network as this computer."))
	}
	rows = append(rows, buttonsRow(n.rescan, n.closeBtn))
	return column(10, rows...)
}

func busy(text string) elements.Element {
	return elements.Row(styles.Styles{Gap: 10, Align: properties.AlignCenter, Padding: properties.SymmetricInset(4, 6)},
		spinner.Spinner(styles.Styles{Width: 18, Height: 18, Color: colMuted}), note(text))
}

func (n *netMedia) serverRow(s *cast.MediaServer) elements.Element {
	return elements.Keyed("net-server", s.ID, func() *elements.Pressable {
		return elements.NewPressable(func(ps elements.PressState) elements.Element {
			bg := colSurface2
			if ps.Hovered {
				bg = colSurface3
			}
			children := []elements.Element{elements.Text(s.Name, styles.Styles{Color: colText, FontSize: 14, FontWeight: properties.FontWeightMedium})}
			if s.Model != "" {
				children = append(children, elements.Text(s.Model, styles.Styles{Color: colFaint, FontSize: 12}))
			}
			return elements.Row(styles.Styles{Width: -1, Gap: 12, Align: properties.AlignCenter, Padding: properties.SymmetricInset(12, 10),
				Background: bg, BorderRadius: 10, Wrap: properties.WrapNoWrap},
				ic("folder", 22, colText),
				elements.Box(styles.Styles{FlexGrow: 1, Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 2}, children...))
		}).Label(s.Name).OnClick(func(elements.ClickEvent) { n.openServer(s) })
	})
}

// row is entry i of the folder: a folder to open or a file to play.
func (n *netMedia) row(i int) elements.Element {
	if i >= len(n.entries) {
		return elements.Box(styles.Styles{})
	}
	e := n.entries[i]
	return elements.Keyed("net-entry", n.server.ID+"\x00"+e.ID, func() *elements.Pressable {
		return elements.NewPressable(func(ps elements.PressState) elements.Element {
			var bg properties.Color
			if ps.Hovered {
				bg = colSurface2
			}
			icon, sub := "file", ""
			switch {
			case e.Folder:
				icon = "folder"
				if e.Count >= 0 {
					sub = strconv.Itoa(e.Count) + " items"
				}
			case e.Audio && !e.Video:
				icon = "audio"
				sub = strings.Join(nonEmpty(e.Artist, e.Album), " · ")
			}
			if !e.Folder && e.Duration > 0 {
				sub = strings.Join(nonEmpty(sub, player.FormatTime(e.Duration)), " · ")
			}
			text := []elements.Element{elements.Text(e.Title, styles.Styles{Color: colText, FontSize: 13, MaxLines: 1, TextOverflow: properties.TextOverflowEllipsis})}
			if sub != "" {
				text = append(text, elements.Text(sub, styles.Styles{Color: colFaint, FontSize: 11, MaxLines: 1}))
			}
			row := []elements.Element{ic(icon, 20, colMuted),
				elements.Box(styles.Styles{FlexGrow: 1, Width: -1, FlexDirection: properties.FlexDirectionColumn, Gap: 2}, text...)}
			if e.Folder {
				row = append(row, ic("chevron", 18, colFaint))
			}
			return elements.Row(styles.Styles{Width: -1, Height: 52, Gap: 12, Align: properties.AlignCenter, Padding: properties.SymmetricInset(10, 0),
				Background: bg, BorderRadius: 8, Wrap: properties.WrapNoWrap}, row...)
		}).Label(e.Title).OnClick(func(elements.ClickEvent) {
			// The list may have changed since this row was built.
			if idx := n.indexOf(e.ID); idx >= 0 {
				n.activate(idx)
			}
		})
	})
}

func (n *netMedia) indexOf(id string) int {
	for i := range n.entries {
		if n.entries[i].ID == id {
			return i
		}
	}
	return -1
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}
