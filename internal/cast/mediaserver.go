package cast

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
)

// DLNA media servers (a NAS, Plex, Jellyfin, Windows Media Player's
// sharing, a router with a USB disk) answer SSDP searches like renderers
// do, and list their library through the ContentDirectory service: a
// tree of containers (folders) and items, each item with HTTP URLs mpv
// plays directly.

const (
	serverType       = "urn:schemas-upnp-org:device:MediaServer:1"
	contentDirPrefix = "urn:schemas-upnp-org:service:ContentDirectory:"
	// browsePage is how many entries one Browse call asks for; servers
	// may return fewer.
	browsePage = 200
	// maxEntries bounds a folder's listing.
	maxEntries = 5000
)

// MediaServer is a DLNA/UPnP media server on the network.
type MediaServer struct {
	ID, Name, Model string
	browseURL       string
	browseType      string
}

// MediaEntry is a folder or a playable file on a media server.
type MediaEntry struct {
	ID, Title string
	// Folder is a container to browse into; Count is how many entries
	// it holds, or -1 when the server doesn't say.
	Folder bool
	Count  int
	// URL plays the file; Video and Audio say what it is.
	URL          string
	Video, Audio bool
	// Duration is in seconds, 0 when unknown.
	Duration float64
	// Artist and Album are set for music.
	Artist, Album string
}

// DiscoverMediaServers searches the network for media servers until ctx
// ends, calling found from its own goroutines with each one, once.
func DiscoverMediaServers(ctx context.Context, found func(*MediaServer)) {
	var (
		mu   sync.Mutex
		seen = map[string]bool{}
	)
	ssdpSearch(ctx, serverType, func(loc string) {
		s, err := describeServer(ctx, loc)
		if err != nil {
			return
		}
		mu.Lock()
		dup := seen[s.ID]
		seen[s.ID] = true
		mu.Unlock()
		if !dup {
			found(s)
		}
	})
}

func describeServer(ctx context.Context, location string) (*MediaServer, error) {
	desc, base, err := describe(ctx, location)
	if err != nil {
		return nil, err
	}
	s := &MediaServer{
		ID:    "upnp:" + desc.Device.UDN,
		Name:  desc.Device.FriendlyName,
		Model: strings.TrimSpace(desc.Device.Manufacturer + " " + desc.Device.ModelName),
	}
	for _, svc := range desc.Device.Services {
		if !strings.HasPrefix(svc.Type, contentDirPrefix) {
			continue
		}
		if ref, err := base.Parse(svc.ControlURL); err == nil {
			s.browseURL, s.browseType = ref.String(), svc.Type
		}
	}
	if s.browseURL == "" {
		return nil, fmt.Errorf("cast: %s has no ContentDirectory service", s.Name)
	}
	if s.Name == "" {
		s.Name = base.Hostname()
	}
	return s, nil
}

// RootID is the ID of a media server's top folder.
const RootID = "0"

// Browse lists the folder id (RootID for the top): its folders first,
// then the video and audio files, each in the server's order. Pictures
// and other files are left out.
func (s *MediaServer) Browse(ctx context.Context, id string) ([]MediaEntry, error) {
	var folders, files []MediaEntry
	for start := 0; start < maxEntries; {
		args := "<ObjectID>" + html.EscapeString(id) + "</ObjectID>" +
			"<BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter>" +
			"<StartingIndex>" + strconv.Itoa(start) + "</StartingIndex>" +
			"<RequestedCount>" + strconv.Itoa(browsePage) + "</RequestedCount><SortCriteria></SortCriteria>"
		resp, err := soapCall(ctx, s.Name, s.browseURL, s.browseType, "Browse", args)
		if err != nil {
			return nil, err
		}
		returned, _ := strconv.Atoi(tag(resp, "NumberReturned"))
		total, _ := strconv.Atoi(tag(resp, "TotalMatches"))
		entries, err := parseDIDL(tag(resp, "Result"))
		if err != nil {
			return nil, fmt.Errorf("cast: %s sent a listing GoPlay can't read: %w", s.Name, err)
		}
		for _, e := range entries {
			switch {
			case e.Folder:
				folders = append(folders, e)
			case e.URL != "" && (e.Video || e.Audio):
				files = append(files, e)
			}
		}
		start += returned
		// Some servers report TotalMatches 0 when they don't count.
		if returned == 0 || (total > 0 && start >= total) || (total == 0 && returned < browsePage) {
			break
		}
	}
	return append(folders, files...), nil
}

// didlLite is the DIDL-Lite listing in a Browse response.
type didlLite struct {
	Containers []struct {
		ID         string `xml:"id,attr"`
		ChildCount string `xml:"childCount,attr"`
		Title      string `xml:"title"`
	} `xml:"container"`
	Items []struct {
		ID     string `xml:"id,attr"`
		Title  string `xml:"title"`
		Class  string `xml:"class"`
		Artist string `xml:"artist"`
		Album  string `xml:"album"`
		Res    []struct {
			ProtocolInfo string `xml:"protocolInfo,attr"`
			Duration     string `xml:"duration,attr"`
			URL          string `xml:",chardata"`
		} `xml:"res"`
	} `xml:"item"`
}

// parseDIDL reads a DIDL-Lite document into entries, in its order:
// containers, then items.
func parseDIDL(doc string) ([]MediaEntry, error) {
	if strings.TrimSpace(doc) == "" {
		return nil, nil
	}
	var d didlLite
	if err := xml.Unmarshal([]byte(doc), &d); err != nil {
		return nil, err
	}
	var out []MediaEntry
	for _, c := range d.Containers {
		count := -1
		if n, err := strconv.Atoi(c.ChildCount); err == nil {
			count = n
		}
		out = append(out, MediaEntry{ID: c.ID, Title: strings.TrimSpace(c.Title), Folder: true, Count: count})
	}
	for _, it := range d.Items {
		e := MediaEntry{ID: it.ID, Title: strings.TrimSpace(it.Title), Artist: it.Artist, Album: it.Album}
		class := strings.ToLower(it.Class)
		e.Video = strings.HasPrefix(class, "object.item.videoitem")
		e.Audio = strings.HasPrefix(class, "object.item.audioitem")
		// The first resource is the original; later ones are usually
		// transcodes or thumbnails.
		for _, r := range it.Res {
			mime := protocolMIME(r.ProtocolInfo)
			if !e.Video && !e.Audio {
				e.Video = strings.HasPrefix(mime, "video/")
				e.Audio = strings.HasPrefix(mime, "audio/")
			}
			if (e.Video && strings.HasPrefix(mime, "video/")) || (e.Audio && strings.HasPrefix(mime, "audio/")) || mime == "" {
				e.URL = strings.TrimSpace(r.URL)
				e.Duration = parseHMS(r.Duration)
				break
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// protocolMIME is the MIME type in a res protocolInfo
// ("http-get:*:video/mp4:DLNA.ORG_PN=...").
func protocolMIME(info string) string {
	parts := strings.Split(info, ":")
	if len(parts) < 3 {
		return ""
	}
	return strings.ToLower(parts[2])
}
