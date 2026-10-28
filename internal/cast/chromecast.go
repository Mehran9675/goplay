package cast

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

// Google Cast: devices are found by mDNS (_googlecast._tcp) and spoken to
// over TLS on port 8009 with length-prefixed protobuf CastMessages whose
// payloads are JSON. goplay launches the Default Media Receiver, which
// plays a URL it fetches itself.

const (
	nsConnection = "urn:x-cast:com.google.cast.tp.connection"
	nsHeartbeat  = "urn:x-cast:com.google.cast.tp.heartbeat"
	nsReceiver   = "urn:x-cast:com.google.cast.receiver"
	nsMedia      = "urn:x-cast:com.google.cast.media"

	defaultReceiverApp = "CC1AD845"
	senderID           = "sender-goplay"
	receiverID         = "receiver-0"
)

func discoverChromecasts(ctx context.Context, report func(Device)) {
	browse(ctx, "_googlecast._tcp.local.", func(s service) {
		name := s.TXT["fn"]
		if name == "" {
			name = s.Instance
		}
		id := s.TXT["id"]
		if id == "" {
			id = s.Instance
		}
		report(&Chromecast{
			id:    "cast:" + id,
			name:  name,
			model: s.TXT["md"],
			addr:  net.JoinHostPort(s.Addr.String(), strconv.Itoa(s.Port)),
		})
	})
}

// Chromecast is a Google Cast device.
type Chromecast struct {
	id, name, model, addr string

	mu        sync.Mutex
	conn      net.Conn
	w         *bufio.Writer
	requestID int
	waiters   map[int]chan map[string]any
	transport string // the media app's transport ID
	session   string // the media app's session ID
	mediaID   int    // media session ID
	status    Status
	closed    chan struct{}
}

func (c *Chromecast) ID() string    { return c.id }
func (c *Chromecast) Name() string  { return c.name }
func (c *Chromecast) Kind() string  { return "Chromecast" }
func (c *Chromecast) Model() string { return c.model }

// Addr is the device's network address, for the media server.
func (c *Chromecast) Addr() string { return c.addr }

// connect opens the TLS connection and the receiver channel, once.
func (c *Chromecast) connect(ctx context.Context) error {
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}} // devices use self-signed certificates
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("cast: connect to %s: %w", c.name, err)
	}
	c.mu.Lock()
	c.conn = conn
	c.w = bufio.NewWriter(conn)
	c.waiters = map[int]chan map[string]any{}
	c.closed = make(chan struct{})
	c.status.Volume = -1
	c.mu.Unlock()
	go c.readLoop(conn)
	go c.heartbeat()
	return c.send(receiverID, nsConnection, map[string]any{"type": "CONNECT"})
}

func (c *Chromecast) heartbeat() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.closed:
			return
		case <-t.C:
			if c.send(receiverID, nsHeartbeat, map[string]any{"type": "PING"}) != nil {
				return
			}
		}
	}
}

func (c *Chromecast) readLoop(conn net.Conn) {
	r := bufio.NewReader(conn)
	for {
		var size uint32
		if err := binary.Read(r, binary.BigEndian, &size); err != nil {
			c.drop()
			return
		}
		buf := make([]byte, size)
		if _, err := io.ReadFull(r, buf); err != nil {
			c.drop()
			return
		}
		msg, err := decodeCastMessage(buf)
		if err != nil {
			continue
		}
		var payload map[string]any
		if json.Unmarshal([]byte(msg.payload), &payload) != nil {
			continue
		}
		c.handle(msg, payload)
	}
}

func (c *Chromecast) handle(msg castMessage, payload map[string]any) {
	typ, _ := payload["type"].(string)
	switch {
	case msg.namespace == nsHeartbeat && typ == "PING":
		c.send(msg.source, nsHeartbeat, map[string]any{"type": "PONG"})
		return
	case msg.namespace == nsConnection && typ == "CLOSE":
		c.mu.Lock()
		if msg.source == c.transport {
			c.transport, c.session, c.mediaID = "", "", 0
			c.status.State = StateIdle
		}
		c.mu.Unlock()
	case typ == "RECEIVER_STATUS":
		c.updateReceiver(payload)
	case typ == "MEDIA_STATUS":
		c.updateMedia(payload)
	}
	if id, ok := payload["requestId"].(float64); ok && id > 0 {
		c.mu.Lock()
		ch := c.waiters[int(id)]
		delete(c.waiters, int(id))
		c.mu.Unlock()
		if ch != nil {
			ch <- payload
		}
	}
}

func (c *Chromecast) updateReceiver(p map[string]any) {
	st, _ := p["status"].(map[string]any)
	c.mu.Lock()
	defer c.mu.Unlock()
	if vol, ok := st["volume"].(map[string]any); ok {
		if l, ok := vol["level"].(float64); ok {
			c.status.Volume = l
		}
		c.status.Muted, _ = vol["muted"].(bool)
	}
	apps, _ := st["applications"].([]any)
	for _, a := range apps {
		app, _ := a.(map[string]any)
		if app["appId"] == defaultReceiverApp {
			c.transport, _ = app["transportId"].(string)
			c.session, _ = app["sessionId"].(string)
		}
	}
}

func (c *Chromecast) updateMedia(p map[string]any) {
	list, _ := p["status"].([]any)
	if len(list) == 0 {
		return
	}
	st, _ := list[0].(map[string]any)
	c.mu.Lock()
	defer c.mu.Unlock()
	if id, ok := st["mediaSessionId"].(float64); ok {
		c.mediaID = int(id)
	}
	if t, ok := st["currentTime"].(float64); ok {
		c.status.Position = t
	}
	if m, ok := st["media"].(map[string]any); ok {
		if d, ok := m["duration"].(float64); ok {
			c.status.Duration = d
		}
	}
	switch st["playerState"] {
	case "PLAYING":
		c.status.State = StatePlaying
	case "PAUSED":
		c.status.State = StatePaused
	case "BUFFERING":
		c.status.State = StateBuffering
	case "IDLE":
		c.status.State = StateIdle
		if st["idleReason"] == "FINISHED" || st["idleReason"] == "CANCELLED" {
			c.status.State = StateStopped
		}
	}
}

func (c *Chromecast) drop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return
	}
	c.conn.Close()
	c.conn = nil
	close(c.closed)
	for id, ch := range c.waiters {
		close(ch)
		delete(c.waiters, id)
	}
	c.transport, c.session, c.mediaID = "", "", 0
	c.status.State = StateIdle
}

func (c *Chromecast) send(dest, ns string, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame := encodeCastMessage(castMessage{source: senderID, dest: dest, namespace: ns, payload: string(data)})
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("cast: not connected")
	}
	c.conn.SetWriteDeadline(time.Now().Add(requestTimeout))
	binary.Write(c.w, binary.BigEndian, uint32(len(frame)))
	c.w.Write(frame)
	return c.w.Flush()
}

// request sends a payload with a fresh requestId and waits for the reply.
func (c *Chromecast) request(ctx context.Context, dest, ns string, payload map[string]any) (map[string]any, error) {
	c.mu.Lock()
	c.requestID++
	id := c.requestID
	ch := make(chan map[string]any, 1)
	if c.waiters == nil {
		c.mu.Unlock()
		return nil, errors.New("cast: not connected")
	}
	c.waiters[id] = ch
	c.mu.Unlock()
	payload["requestId"] = id
	if err := c.send(dest, ns, payload); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	select {
	case reply, ok := <-ch:
		if !ok {
			return nil, errors.New("cast: connection lost")
		}
		if t, _ := reply["type"].(string); t == "LOAD_FAILED" || t == "LAUNCH_ERROR" || t == "INVALID_REQUEST" {
			reason, _ := reply["reason"].(string)
			if reason == "" {
				reason = t
			}
			return reply, fmt.Errorf("cast: %s refused: %s", c.name, reason)
		}
		return reply, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.waiters, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("cast: %s did not answer", c.name)
	}
}

// launch starts the Default Media Receiver, if it isn't running.
func (c *Chromecast) launch(ctx context.Context) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	running := c.transport != ""
	c.mu.Unlock()
	if !running {
		if _, err := c.request(ctx, receiverID, nsReceiver, map[string]any{"type": "LAUNCH", "appId": defaultReceiverApp}); err != nil {
			return err
		}
		// The launch reply is a RECEIVER_STATUS that already carries the
		// app; wait briefly if it didn't.
		for i := 0; i < 20; i++ {
			c.mu.Lock()
			running = c.transport != ""
			c.mu.Unlock()
			if running {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if !running {
			return fmt.Errorf("cast: %s did not start the media player", c.name)
		}
	}
	c.mu.Lock()
	transport := c.transport
	c.mu.Unlock()
	return c.send(transport, nsConnection, map[string]any{"type": "CONNECT"})
}

func (c *Chromecast) Load(ctx context.Context, m Media) error {
	if err := c.launch(ctx); err != nil {
		return err
	}
	media := map[string]any{
		"contentId":   m.URL,
		"contentType": m.ContentType,
		"streamType":  "BUFFERED",
		"metadata":    map[string]any{"metadataType": 0, "title": m.Title},
	}
	if m.Duration > 0 {
		media["duration"] = m.Duration
	}
	load := map[string]any{"type": "LOAD", "media": media, "autoplay": true, "currentTime": m.Start}
	if m.Subtitles != "" {
		media["tracks"] = []any{map[string]any{
			"trackId": 1, "type": "TEXT", "subtype": "SUBTITLES", "name": "Subtitles",
			"trackContentId": m.Subtitles, "trackContentType": "text/vtt", "language": m.SubtitleLang,
		}}
		media["textTrackStyle"] = map[string]any{"backgroundColor": "#00000000", "edgeType": "OUTLINE", "edgeColor": "#000000FF"}
		load["activeTrackIds"] = []int{1}
	}
	c.mu.Lock()
	transport, session := c.transport, c.session
	c.status.State = StateLoading
	c.mu.Unlock()
	load["sessionId"] = session
	_, err := c.request(ctx, transport, nsMedia, load)
	return err
}

func (c *Chromecast) mediaCommand(typ string, extra map[string]any) error {
	c.mu.Lock()
	transport, id := c.transport, c.mediaID
	c.mu.Unlock()
	if transport == "" || id == 0 {
		return errors.New("cast: nothing is playing")
	}
	p := map[string]any{"type": typ, "mediaSessionId": id}
	for k, v := range extra {
		p[k] = v
	}
	_, err := c.request(context.Background(), transport, nsMedia, p)
	return err
}

func (c *Chromecast) Play() error  { return c.mediaCommand("PLAY", nil) }
func (c *Chromecast) Pause() error { return c.mediaCommand("PAUSE", nil) }
func (c *Chromecast) Seek(s float64) error {
	return c.mediaCommand("SEEK", map[string]any{"currentTime": s})
}

// Stop ends playback and closes the media app, returning the TV to its
// home screen.
func (c *Chromecast) Stop() error {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session == "" {
		return nil
	}
	_, err := c.request(context.Background(), receiverID, nsReceiver, map[string]any{"type": "STOP", "sessionId": session})
	return err
}

func (c *Chromecast) SetVolume(level float64) error {
	if err := c.connect(context.Background()); err != nil {
		return err
	}
	_, err := c.request(context.Background(), receiverID, nsReceiver, map[string]any{
		"type": "SET_VOLUME", "volume": map[string]any{"level": max(0, min(1, level))},
	})
	return err
}

func (c *Chromecast) Status(ctx context.Context) (Status, error) {
	c.mu.Lock()
	transport, id := c.transport, c.mediaID
	c.mu.Unlock()
	if transport != "" {
		p := map[string]any{"type": "GET_STATUS"}
		if id != 0 {
			p["mediaSessionId"] = id
		}
		if _, err := c.request(ctx, transport, nsMedia, p); err != nil {
			return c.snapshot(), err
		}
	}
	return c.snapshot(), nil
}

func (c *Chromecast) snapshot() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *Chromecast) Close() error {
	c.drop()
	return nil
}

// CastMessage protobuf encoding. Only the fields goplay uses:
// 1 protocol_version (varint), 2 source_id, 3 destination_id,
// 4 namespace, 5 payload_type (varint, 0 = string), 6 payload_utf8.

type castMessage struct {
	source, dest, namespace, payload string
}

func encodeCastMessage(m castMessage) []byte {
	var b []byte
	b = appendVarintField(b, 1, 0)
	b = appendStringField(b, 2, m.source)
	b = appendStringField(b, 3, m.dest)
	b = appendStringField(b, 4, m.namespace)
	b = appendVarintField(b, 5, 0)
	b = appendStringField(b, 6, m.payload)
	return b
}

func appendVarintField(b []byte, field int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3|0))
	return binary.AppendUvarint(b, v)
}

func appendStringField(b []byte, field int, s string) []byte {
	b = binary.AppendUvarint(b, uint64(field<<3|2))
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}

func decodeCastMessage(b []byte) (castMessage, error) {
	var m castMessage
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return m, errors.New("cast: bad message")
		}
		b = b[n:]
		field, wire := int(key>>3), key&7
		switch wire {
		case 0:
			_, n := binary.Uvarint(b)
			if n <= 0 {
				return m, errors.New("cast: bad varint")
			}
			b = b[n:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return m, errors.New("cast: bad length")
			}
			v := string(b[n : n+int(l)])
			b = b[n+int(l):]
			switch field {
			case 2:
				m.source = v
			case 3:
				m.dest = v
			case 4:
				m.namespace = v
			case 6:
				m.payload = v
			}
		default:
			return m, errors.New("cast: unexpected wire type")
		}
	}
	return m, nil
}
