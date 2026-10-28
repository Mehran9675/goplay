package cast

import (
	"bufio"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DLNA/UPnP: media renderers answer SSDP searches with the URL of an XML
// description listing their services; playback is driven with SOAP
// calls to the AVTransport service, volume with RenderingControl.

const (
	ssdpAddr        = "239.255.255.250:1900"
	rendererType    = "urn:schemas-upnp-org:device:MediaRenderer:1"
	avTransport     = "urn:schemas-upnp-org:service:AVTransport:1"
	renderingContrl = "urn:schemas-upnp-org:service:RenderingControl:1"
)

func discoverDLNA(ctx context.Context, report func(Device)) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	dst, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	search := []byte("M-SEARCH * HTTP/1.1\r\nHOST: " + ssdpAddr + "\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + rendererType + "\r\n\r\n")
	go func() {
		t := time.NewTicker(4 * time.Second)
		defer t.Stop()
		for {
			conn.WriteTo(search, dst)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	seen := map[string]bool{}
	var mu sync.Mutex
	buf := make([]byte, 4096)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(buf[:n])), nil)
		if err != nil {
			continue
		}
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		mu.Lock()
		dup := loc == "" || seen[loc]
		seen[loc] = true
		mu.Unlock()
		if dup {
			continue
		}
		go func() {
			d, err := describeRenderer(ctx, loc)
			if err == nil {
				report(d)
			}
		}()
	}
}

type upnpDescription struct {
	URLBase string `xml:"URLBase"`
	Device  struct {
		FriendlyName string `xml:"friendlyName"`
		ModelName    string `xml:"modelName"`
		Manufacturer string `xml:"manufacturer"`
		UDN          string `xml:"UDN"`
		Services     []struct {
			Type       string `xml:"serviceType"`
			ControlURL string `xml:"controlURL"`
		} `xml:"serviceList>service"`
	} `xml:"device"`
}

// describeRenderer fetches a renderer's description and finds its
// control URLs.
func describeRenderer(ctx context.Context, location string) (*DLNARenderer, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var desc upnpDescription
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&desc); err != nil {
		return nil, err
	}
	base, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	if desc.URLBase != "" {
		if b, err := url.Parse(desc.URLBase); err == nil {
			base = b
		}
	}
	r := &DLNARenderer{
		name:  desc.Device.FriendlyName,
		model: strings.TrimSpace(desc.Device.Manufacturer + " " + desc.Device.ModelName),
		id:    "dlna:" + desc.Device.UDN,
		host:  base.Host,
	}
	for _, s := range desc.Device.Services {
		ref, err := url.Parse(s.ControlURL)
		if err != nil {
			continue
		}
		u := base.ResolveReference(ref).String()
		switch {
		case strings.HasPrefix(s.Type, "urn:schemas-upnp-org:service:AVTransport:"):
			r.transportURL, r.transportType = u, s.Type
		case strings.HasPrefix(s.Type, "urn:schemas-upnp-org:service:RenderingControl:"):
			r.renderingURL, r.renderingType = u, s.Type
		}
	}
	if r.transportURL == "" {
		return nil, fmt.Errorf("cast: %s has no AVTransport service", r.name)
	}
	if r.name == "" {
		r.name = base.Hostname()
	}
	return r, nil
}

// DLNARenderer is a UPnP media renderer.
type DLNARenderer struct {
	id, name, model, host       string
	transportURL, transportType string
	renderingURL, renderingType string
	mu                          sync.Mutex
	duration                    float64
}

func (d *DLNARenderer) ID() string    { return d.id }
func (d *DLNARenderer) Name() string  { return d.name }
func (d *DLNARenderer) Kind() string  { return "DLNA" }
func (d *DLNARenderer) Model() string { return d.model }

// Addr is the device's network address, for the media server.
func (d *DLNARenderer) Addr() string { return d.host }

func (d *DLNARenderer) Load(ctx context.Context, m Media) error {
	d.mu.Lock()
	d.duration = m.Duration
	d.mu.Unlock()
	meta := didl(m)
	if _, err := d.soap(ctx, d.transportURL, d.transportType, "SetAVTransportURI",
		"<InstanceID>0</InstanceID><CurrentURI>"+html.EscapeString(m.URL)+"</CurrentURI><CurrentURIMetaData>"+html.EscapeString(meta)+"</CurrentURIMetaData>"); err != nil {
		return err
	}
	if err := d.Play(); err != nil {
		return err
	}
	if m.Start > 1 {
		// Many renderers only accept a seek once playing.
		go func() {
			time.Sleep(2 * time.Second)
			d.Seek(m.Start)
		}()
	}
	return nil
}

// didl is the DIDL-Lite metadata renderers want with a URI: the title,
// the resource's type, and subtitles in the forms TVs look for.
func didl(m Media) string {
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:sec="http://www.sec.co.kr/">`)
	b.WriteString(`<item id="0" parentID="-1" restricted="1"><dc:title>`)
	b.WriteString(html.EscapeString(m.Title))
	b.WriteString(`</dc:title><upnp:class>object.item.videoItem</upnp:class>`)
	fmt.Fprintf(&b, `<res protocolInfo="http-get:*:%s:DLNA.ORG_OP=01;DLNA.ORG_FLAGS=01700000000000000000000000000000">%s</res>`,
		html.EscapeString(m.ContentType), html.EscapeString(m.URL))
	if m.Subtitles != "" {
		fmt.Fprintf(&b, `<res protocolInfo="http-get:*:text/vtt:*">%s</res>`, html.EscapeString(m.Subtitles))
		fmt.Fprintf(&b, `<sec:CaptionInfoEx sec:type="vtt">%s</sec:CaptionInfoEx>`, html.EscapeString(m.Subtitles))
	}
	b.WriteString(`</item></DIDL-Lite>`)
	return b.String()
}

func (d *DLNARenderer) Play() error {
	_, err := d.soap(context.Background(), d.transportURL, d.transportType, "Play", "<InstanceID>0</InstanceID><Speed>1</Speed>")
	return err
}

func (d *DLNARenderer) Pause() error {
	_, err := d.soap(context.Background(), d.transportURL, d.transportType, "Pause", "<InstanceID>0</InstanceID>")
	return err
}

func (d *DLNARenderer) Seek(s float64) error {
	_, err := d.soap(context.Background(), d.transportURL, d.transportType, "Seek",
		"<InstanceID>0</InstanceID><Unit>REL_TIME</Unit><Target>"+hms(s)+"</Target>")
	return err
}

func (d *DLNARenderer) Stop() error {
	_, err := d.soap(context.Background(), d.transportURL, d.transportType, "Stop", "<InstanceID>0</InstanceID>")
	return err
}

func (d *DLNARenderer) SetVolume(level float64) error {
	if d.renderingURL == "" {
		return ErrUnsupported
	}
	v := int(max(0, min(1, level)) * 100)
	_, err := d.soap(context.Background(), d.renderingURL, d.renderingType, "SetVolume",
		"<InstanceID>0</InstanceID><Channel>Master</Channel><DesiredVolume>"+strconv.Itoa(v)+"</DesiredVolume>")
	return err
}

func (d *DLNARenderer) Status(ctx context.Context) (Status, error) {
	st := Status{Volume: -1}
	info, err := d.soap(ctx, d.transportURL, d.transportType, "GetTransportInfo", "<InstanceID>0</InstanceID>")
	if err != nil {
		return st, err
	}
	switch tag(info, "CurrentTransportState") {
	case "PLAYING":
		st.State = StatePlaying
	case "PAUSED_PLAYBACK", "PAUSED_RECORDING":
		st.State = StatePaused
	case "TRANSITIONING":
		st.State = StateBuffering
	case "STOPPED":
		st.State = StateStopped
	default:
		st.State = StateIdle
	}
	if pos, err := d.soap(ctx, d.transportURL, d.transportType, "GetPositionInfo", "<InstanceID>0</InstanceID>"); err == nil {
		st.Position = parseHMS(tag(pos, "RelTime"))
		st.Duration = parseHMS(tag(pos, "TrackDuration"))
	}
	if st.Duration == 0 {
		d.mu.Lock()
		st.Duration = d.duration
		d.mu.Unlock()
	}
	if d.renderingURL != "" {
		if vol, err := d.soap(ctx, d.renderingURL, d.renderingType, "GetVolume",
			"<InstanceID>0</InstanceID><Channel>Master</Channel>"); err == nil {
			if v, err := strconv.Atoi(tag(vol, "CurrentVolume")); err == nil {
				st.Volume = float64(v) / 100
			}
		}
	}
	return st, nil
}

func (d *DLNARenderer) Close() error { return nil }

// soap calls action on a UPnP service and returns the response body.
func (d *DLNARenderer) soap(ctx context.Context, controlURL, service, action, args string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><u:` + action + ` xmlns:u="` + service + `">` + args + `</u:` + action + `></s:Body></s:Envelope>`
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+service+"#"+action+`"`)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cast: %s: %w", d.name, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		msg := tag(string(data), "errorDescription")
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("cast: %s refused %s: %s", d.name, action, msg)
	}
	return string(data), nil
}

// tag returns the text of the first <name> element in s, ignoring
// namespaces. SOAP responses are small and flat enough for this.
func tag(s, name string) string {
	for _, open := range []string{"<" + name + ">", ":" + name + ">"} {
		i := strings.Index(s, open)
		if i < 0 {
			continue
		}
		rest := s[i+len(open):]
		j := strings.Index(rest, "</")
		if j < 0 {
			return ""
		}
		return html.UnescapeString(strings.TrimSpace(rest[:j]))
	}
	return ""
}

// hms formats seconds as H:MM:SS for UPnP.
func hms(s float64) string {
	t := int(max(0, s))
	return fmt.Sprintf("%d:%02d:%02d", t/3600, t/60%60, t%60)
}

// parseHMS reads UPnP's H:MM:SS(.fff); "NOT_IMPLEMENTED" and the like
// give 0.
func parseHMS(s string) float64 {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	var total float64
	for _, p := range parts {
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0
		}
		total = total*60 + v
	}
	return total
}
