package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

// DHCP leases are observations, not proof of address ownership. Broadcast
// discovery can receive several offers; correlations remain available until TTL.
func decodeDHCP4(w []byte, limit int) (map[string]any, error) {
	if len(w) < 240 || binary.BigEndian.Uint32(w[236:240]) != dhcpCookie || w[0] < 1 || w[0] > 2 || w[2] == 0 || w[2] > 16 {
		return nil, fmt.Errorf("DHCP header")
	}
	opts := map[byte][]byte{}
	count := 0
	parse := func(b []byte) error {
		for i := 0; i < len(b); {
			code := b[i]
			i++
			if code == 255 {
				return nil
			}
			if code == 0 {
				continue
			}
			if i >= len(b) || int(b[i]) > len(b)-i-1 {
				return fmt.Errorf("DHCP option length")
			}
			n := int(b[i])
			i++
			count++
			if count > limit {
				return protocolError(ErrResourceExceeded, "DHCP option count")
			}
			opts[code] = append(opts[code], b[i:i+n]...)
			i += n
		}
		return fmt.Errorf("DHCP options missing END")
	}
	if err := parse(w[240:]); err != nil {
		return nil, err
	}
	overload := byte(0)
	if b := opts[52]; len(b) > 0 {
		if len(b) != 1 || b[0] > 3 || b[0] == 0 {
			return nil, fmt.Errorf("DHCP overload")
		}
		overload = b[0]
	}
	if overload&1 != 0 {
		if err := parse(w[108:236]); err != nil {
			return nil, err
		}
	}
	if overload&2 != 0 {
		if err := parse(w[44:108]); err != nil {
			return nil, err
		}
	}
	if len(opts[53]) != 1 {
		return nil, fmt.Errorf("DHCP message type")
	}
	typ := opts[53][0]
	out := map[string]any{"Message Type": typ, "Packet Name": dhcpMsgName(typ), "Xid": binary.BigEndian.Uint32(w[4:8]), "CHADDR": bytes.Clone(w[28 : 28+int(w[2])]), "Client IP": net.IP(w[12:16]).String(), "Offered IP": net.IP(w[16:20]).String(), "Relay IP": net.IP(w[24:28]).String(), "Observation": "unverified-lease", "Options Overload": overload}
	for _, opt := range []struct {
		code byte
		name string
	}{{50, "Requested IP"}, {54, "Server Identifier"}, {1, "Subnet Mask"}} {
		if b, ok := opts[opt.code]; ok {
			if len(b) != 4 {
				return nil, fmt.Errorf("DHCP IPv4 option length")
			}
			out[opt.name] = net.IP(b).String()
		}
	}
	for _, opt := range []struct {
		code byte
		name string
	}{{51, "Lease Seconds"}, {58, "Renewal Seconds"}, {59, "Rebinding Seconds"}} {
		if b, ok := opts[opt.code]; ok {
			if len(b) != 4 {
				return nil, fmt.Errorf("DHCP lifetime option length")
			}
			out[opt.name] = binary.BigEndian.Uint32(b)
		}
	}
	for _, opt := range []struct {
		code byte
		name string
	}{{3, "Routers"}, {6, "DNS Servers"}} {
		if b, ok := opts[opt.code]; ok {
			if len(b)%4 != 0 {
				return nil, fmt.Errorf("DHCP address list length")
			}
			var addrs []string
			for i := 0; i < len(b); i += 4 {
				addrs = append(addrs, net.IP(b[i:i+4]).String())
			}
			out[opt.name] = addrs
		}
	}
	for _, opt := range []struct {
		code byte
		name string
	}{{12, "Hostname"}, {15, "Domain"}, {61, "Client Identifier"}} {
		if b, ok := opts[opt.code]; ok {
			out[opt.name] = bytes.Clone(b)
		}
	}
	out["Option Count"] = count
	return out, nil
}

func (a *binParser) dhcpObservation(e *ProtocolEvent) {
	info := e.Session
	if info == nil {
		return
	}
	typ, ok := info["Message Type"].(byte)
	if !ok {
		return
	}
	identity := fmt.Sprintf("%x", info["CHADDR"])
	key := fmt.Sprintf("dhcp/%v/%v/%s/%v", e.Domain, info["Relay IP"], identity, info["Xid"])
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.dns
	if s.pending == nil {
		s.pending = map[string]dnsPending{}
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	for k, v := range s.pending {
		if s.clock.Sub(v.ts) > 30*time.Second {
			delete(s.pending, k)
			a.buffered.Add(-v.cost)
		}
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if typ == 1 || typ == 3 || typ == 8 {
		if p, ok := s.pending[key]; ok {
			e.TransactionID = p.id
		} else if len(s.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(int64(len(key)+128)) {
			s.pending[key] = dnsPending{id: e.ID, ts: e.Timestamp, cost: int64(len(key) + 128)}
			e.TransactionID = e.ID
		}
	} else if typ == 2 || typ == 5 || typ == 6 {
		if p, ok := s.pending[key]; ok {
			e.ResponseTo = p.id
			e.TransactionID = p.id
			info["Association"] = "observed-client-xid"
			if typ == 5 || typ == 6 {
				e.Completeness = "transaction"
			}
		} else {
			info["Association"] = "missing-request"
		}
	}
}

func (a *binParser) dhcp6Observation(e *ProtocolEvent) {
	info := e.Session
	relay := ""
	for depth := 0; depth < 8; depth++ {
		typ, _ := info["Message Type"].(byte)
		if typ != 12 && typ != 13 {
			break
		}
		relay += fmt.Sprint(info["Link Address"], "/", info["Peer Address"], "/")
		var next map[string]any
		for _, o := range info["Options"].([]map[string]any) {
			if m, ok := o["Relay Message"].(map[string]any); ok {
				next = m
				break
			}
		}
		if next == nil {
			return
		}
		info = next
	}
	var client []byte
	for _, o := range info["Options"].([]map[string]any) {
		if o["Code"] == uint16(1) {
			client, _ = o["DUID"].([]byte)
		}
	}
	if len(client) == 0 {
		e.Session["Association"] = "missing-client-duid"
		return
	}
	key := fmt.Sprintf("dhcp6/%v/%s/%x/%v", e.Domain, relay, client, info["Transaction ID"])
	typ, _ := info["Message Type"].(byte)
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	s := &a.dns
	if s.pending == nil {
		s.pending = map[string]dnsPending{}
	}
	if e.Timestamp.After(s.clock) {
		s.clock = e.Timestamp
	}
	for k, v := range s.pending {
		if s.clock.Sub(v.ts) > 30*time.Second {
			delete(s.pending, k)
			a.buffered.Add(-v.cost)
		}
	}
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if typ == 2 || typ == 7 {
		if p, ok := s.pending[key]; ok {
			e.ResponseTo, e.TransactionID = p.id, p.id
			e.Session["Association"] = "observed-client-xid"
			e.Completeness = "transaction"
		} else {
			e.Session["Association"] = "missing-request"
		}
	} else if typ != 10 {
		if p, ok := s.pending[key]; ok {
			e.TransactionID = p.id
		} else if len(s.pending) < a.budget.MaxCollectionElements && a.reserveEvidence(int64(len(key)+128)) {
			s.pending[key] = dnsPending{id: e.ID, ts: e.Timestamp, cost: int64(len(key) + 128)}
			e.TransactionID = e.ID
		}
	}
}
