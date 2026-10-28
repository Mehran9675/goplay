package cast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCastMessageRoundTrip(t *testing.T) {
	in := castMessage{source: "sender-0", dest: "receiver-0", namespace: nsMedia, payload: `{"type":"PING","x":"ünïcode"}`}
	out, err := decodeCastMessage(encodeCastMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("got %+v, want %+v", out, in)
	}
	if _, err := decodeCastMessage([]byte{0x32, 0x10, 'a'}); err == nil {
		t.Error("truncated message decoded")
	}
}

func TestSRTToVTT(t *testing.T) {
	var b strings.Builder
	SRTToVTT(&b, strings.NewReader("\xef\xbb\xbf1\n00:00:01,500 --> 00:00:04,000\nHello\n"))
	want := "WEBVTT\n\n1\n00:00:01.500 --> 00:00:04.000\nHello\n"
	if b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}

func TestHMS(t *testing.T) {
	if got := hms(3725.9); got != "1:02:05" {
		t.Errorf("hms = %q", got)
	}
	for in, want := range map[string]float64{"1:02:05": 3725, "0:00:01.500": 1.5, "NOT_IMPLEMENTED": 0, "": 0} {
		if got := parseHMS(in); got != want {
			t.Errorf("parseHMS(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNeedsConversion(t *testing.T) {
	for _, c := range []struct {
		path, v, a string
		want       bool
	}{
		{"a.mp4", "h264", "aac", false},
		{"a.mp4", "hevc", "aac", true},
		{"a.mkv", "h264", "aac", true},
		{"a.webm", "vp9", "opus", false},
		{"a.mp4", "h264", "ac3", true},
	} {
		if got := NeedsConversion(c.path, c.v, c.a); got != c.want {
			t.Errorf("NeedsConversion(%s, %s, %s) = %v", c.path, c.v, c.a, got)
		}
	}
}

func TestServer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	os.WriteFile(path, []byte("0123456789"), 0o644)
	sub := filepath.Join(dir, "clip.srt")
	os.WriteFile(sub, []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n"), 0o644)

	s, err := NewServer("127.0.0.1:9")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	media, ctype, subs := s.Share(Source{Path: path, Subtitles: sub})
	if ctype != "video/mp4" || !strings.HasSuffix(media, "/media.mp4") || subs == "" {
		t.Fatalf("share: %s %s %s", media, ctype, subs)
	}

	req, _ := http.NewRequest(http.MethodGet, media, nil)
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Errorf("range: %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("transferMode.dlna.org") != "Streaming" {
		t.Error("missing DLNA streaming header")
	}

	resp, err = http.Get(subs)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasPrefix(string(body), "WEBVTT") || !strings.Contains(string(body), "00:00:01.000") {
		t.Errorf("subtitles: %q", body)
	}

	// Another token, or a guessed path, finds nothing.
	bad := strings.Replace(media, "/media/", "/media/00", 1)
	if resp, err := http.Get(bad); err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("unknown token: %d", resp.StatusCode)
		}
	}
}

// fakeRenderer is a minimal UPnP renderer: a description and an
// AVTransport service recording the actions it gets.
type fakeRenderer struct {
	mu      sync.Mutex
	actions []string
	uri     string
}

func (f *fakeRenderer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/desc.xml":
		fmt.Fprint(w, `<?xml version="1.0"?><root xmlns="urn:schemas-upnp-org:device-1-0"><device>
<friendlyName>Living Room TV</friendlyName><manufacturer>ACME</manufacturer><modelName>X1</modelName><UDN>uuid:tv1</UDN>
<serviceList><service><serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType><controlURL>/avt</controlURL></service>
<service><serviceType>urn:schemas-upnp-org:service:RenderingControl:1</serviceType><controlURL>/rc</controlURL></service></serviceList>
</device></root>`)
	case "/avt", "/rc":
		action := r.Header.Get("SOAPACTION")
		action = strings.Trim(action[strings.Index(action, "#")+1:], `"`)
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.actions = append(f.actions, action)
		if action == "SetAVTransportURI" {
			f.uri = tag(string(body), "CurrentURI")
		}
		f.mu.Unlock()
		switch action {
		case "GetTransportInfo":
			fmt.Fprint(w, `<s:Envelope><s:Body><u:GetTransportInfoResponse><CurrentTransportState>PLAYING</CurrentTransportState></u:GetTransportInfoResponse></s:Body></s:Envelope>`)
		case "GetPositionInfo":
			fmt.Fprint(w, `<s:Envelope><s:Body><u:R><RelTime>0:00:42</RelTime><TrackDuration>0:10:00</TrackDuration></u:R></s:Body></s:Envelope>`)
		case "GetVolume":
			fmt.Fprint(w, `<s:Envelope><s:Body><u:R><CurrentVolume>30</CurrentVolume></u:R></s:Body></s:Envelope>`)
		default:
			fmt.Fprint(w, `<s:Envelope><s:Body/></s:Envelope>`)
		}
	default:
		http.NotFound(w, r)
	}
}

func TestDLNA(t *testing.T) {
	fake := &fakeRenderer{}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	d, err := describeRenderer(context.Background(), srv.URL+"/desc.xml")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name() != "Living Room TV" || d.Model() != "ACME X1" || d.ID() != "dlna:uuid:tv1" {
		t.Errorf("device = %q %q %q", d.Name(), d.Model(), d.ID())
	}
	m := Media{URL: "http://10.0.0.2:1234/media/x/media.mp4?a=1&b=2", Title: "A & B", ContentType: "video/mp4"}
	if err := d.Load(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if fake.uri != m.URL {
		t.Errorf("renderer got URI %q, want %q", fake.uri, m.URL)
	}
	st, err := d.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.State != StatePlaying || st.Position != 42 || st.Duration != 600 || st.Volume != 0.3 {
		t.Errorf("status = %+v", st)
	}
	d.Seek(90)
	d.SetVolume(0.5)
	fake.mu.Lock()
	got := strings.Join(fake.actions, ",")
	fake.mu.Unlock()
	for _, want := range []string{"SetAVTransportURI", "Play", "Seek", "SetVolume"} {
		if !strings.Contains(got, want) {
			t.Errorf("actions %s lack %s", got, want)
		}
	}
}
