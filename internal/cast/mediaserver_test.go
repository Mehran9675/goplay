package cast

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testDIDL = `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">
<container id="1$4" parentID="0" childCount="12" restricted="1"><dc:title>Movies</dc:title><upnp:class>object.container.storageFolder</upnp:class></container>
<item id="1$5" parentID="0" restricted="1"><dc:title>Holiday &amp; Beach</dc:title><upnp:class>object.item.videoItem.movie</upnp:class>
<res protocolInfo="http-get:*:image/jpeg:*">http://nas/thumb.jpg</res>
<res protocolInfo="http-get:*:video/x-matroska:*" duration="1:02:03.000">http://nas/holiday.mkv</res></item>
<item id="1$6" parentID="0" restricted="1"><dc:title>Song</dc:title><upnp:class>object.item.audioItem.musicTrack</upnp:class><upnp:artist>Band</upnp:artist>
<res protocolInfo="http-get:*:audio/flac:*" duration="0:03:30">http://nas/song.flac</res></item>
<item id="1$7" parentID="0" restricted="1"><dc:title>Photo</dc:title><upnp:class>object.item.imageItem.photo</upnp:class>
<res protocolInfo="http-get:*:image/jpeg:*">http://nas/photo.jpg</res></item>
</DIDL-Lite>`

func TestParseDIDL(t *testing.T) {
	es, err := parseDIDL(testDIDL)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 4 {
		t.Fatalf("got %d entries, want 4", len(es))
	}
	if f := es[0]; !f.Folder || f.Title != "Movies" || f.Count != 12 || f.ID != "1$4" {
		t.Errorf("folder = %+v", f)
	}
	if v := es[1]; !v.Video || v.URL != "http://nas/holiday.mkv" || v.Duration != 3723 || v.Title != "Holiday & Beach" {
		t.Errorf("video = %+v", v)
	}
	if a := es[2]; !a.Audio || a.URL != "http://nas/song.flac" || a.Artist != "Band" || a.Duration != 210 {
		t.Errorf("audio = %+v", a)
	}
	if p := es[3]; p.Video || p.Audio {
		t.Errorf("photo counted as media: %+v", p)
	}
}

// TestBrowse runs Browse against a fake ContentDirectory that pages its
// answer, and checks pictures are left out and folders come first.
func TestBrowse(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(r.Header.Get("SOAPACTION"), "#Browse") || !strings.Contains(string(body), "<ObjectID>0</ObjectID>") {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		calls++
		var page string
		var n int
		if strings.Contains(string(body), "<StartingIndex>0</StartingIndex>") {
			page, n = testDIDL[:strings.Index(testDIDL, "<item")]+"</DIDL-Lite>", 1
		} else {
			page, n = testDIDL[:strings.Index(testDIDL, "<container")]+testDIDL[strings.Index(testDIDL, "<item"):], 3
		}
		fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
			`<u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><Result>%s</Result>`+
			`<NumberReturned>%d</NumberReturned><TotalMatches>4</TotalMatches><UpdateID>1</UpdateID></u:BrowseResponse></s:Body></s:Envelope>`,
			html.EscapeString(page), n)
	}))
	defer srv.Close()
	s := &MediaServer{Name: "Test", browseURL: srv.URL, browseType: contentDirPrefix + "1"}
	es, err := s.Browse(context.Background(), RootID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("Browse made %d calls, want 2 pages", calls)
	}
	var titles []string
	for _, e := range es {
		titles = append(titles, e.Title)
	}
	if got := strings.Join(titles, ", "); got != "Movies, Holiday & Beach, Song" {
		t.Errorf("entries = %s", got)
	}
}
