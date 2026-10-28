package cast

import (
	"context"
	"net"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// A minimal mDNS (multicast DNS) browser: enough to find services such
// as _googlecast._tcp on the local network. Queries go out from an
// ephemeral port, so responders answer directly to it (RFC 6762 §6.7),
// and no multicast group membership or port 5353 is needed.

var mdnsAddr = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// service is one instance found by browse.
type service struct {
	Instance string
	Host     string
	Port     int
	Addr     net.IP
	TXT      map[string]string
}

// browse queries for service (e.g. "_googlecast._tcp.local.") every few
// seconds until ctx ends, reporting each instance once its address is
// known.
func browse(ctx context.Context, svc string, found func(service)) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return err
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	query, err := mdnsQuery(svc)
	if err != nil {
		return err
	}
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			conn.WriteToUDP(query, mdnsAddr)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()

	type partial struct {
		host string
		port int
		txt  map[string]string
	}
	instances := map[string]*partial{}
	addrs := map[string]net.IP{}
	reported := map[string]bool{}
	buf := make([]byte, 9000)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			return ctx.Err()
		}
		var p dnsmessage.Parser
		if _, err := p.Start(buf[:n]); err != nil {
			continue
		}
		p.SkipAllQuestions()
		var records []dnsmessage.Resource
		for _, section := range []func() ([]dnsmessage.Resource, error){p.AllAnswers, p.AllAuthorities, p.AllAdditionals} {
			rs, err := section()
			if err != nil {
				break
			}
			records = append(records, rs...)
		}
		for _, r := range records {
			name := strings.ToLower(r.Header.Name.String())
			switch b := r.Body.(type) {
			case *dnsmessage.PTRResource:
				if strings.EqualFold(name, svc) {
					inst := strings.ToLower(b.PTR.String())
					if instances[inst] == nil {
						instances[inst] = &partial{}
					}
				}
			case *dnsmessage.SRVResource:
				in := instances[name]
				if in == nil {
					in = &partial{}
					instances[name] = in
				}
				in.host, in.port = strings.ToLower(b.Target.String()), int(b.Port)
			case *dnsmessage.TXTResource:
				in := instances[name]
				if in == nil {
					in = &partial{}
					instances[name] = in
				}
				in.txt = map[string]string{}
				for _, kv := range b.TXT {
					k, v, _ := strings.Cut(kv, "=")
					in.txt[strings.ToLower(k)] = v
				}
			case *dnsmessage.AResource:
				addrs[name] = net.IP(b.A[:])
			}
		}
		for inst, in := range instances {
			if reported[inst] || in.port == 0 || !strings.HasSuffix(inst, svc) {
				continue
			}
			ip := addrs[in.host]
			if ip == nil {
				// Some devices leave the A record out: the answer came
				// from the device itself.
				ip = src.IP
			}
			reported[inst] = true
			found(service{Instance: inst, Host: in.host, Port: in.port, Addr: ip, TXT: in.txt})
		}
	}
}

// mdnsQuery builds a PTR query for svc.
func mdnsQuery(svc string) ([]byte, error) {
	name, err := dnsmessage.NewName(svc)
	if err != nil {
		return nil, err
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{})
	b.EnableCompression()
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	if err := b.Question(dnsmessage.Question{Name: name, Type: dnsmessage.TypePTR, Class: dnsmessage.ClassINET}); err != nil {
		return nil, err
	}
	return b.Finish()
}
