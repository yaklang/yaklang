package pcaputil

import (
	"container/list"
	"fmt"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type routingDatagramState struct {
	identity       string
	discriminators [2]uint64
	seenPeer       [2]bool
	poll           [2]uint64
	pollAmbiguous  [2]bool
	pollTime       [2]time.Time
	last           time.Time
}

func routingNumber(m map[string]any, key string) uint64 { v, _ := m[key].(uint64); return v }
func routingHost(endpoint string) string {
	host, _, err := net.SplitHostPort(endpoint)
	if err == nil {
		return host
	}
	return endpoint
}

// Advertisements have no reliable packet sequence. We retain only observed
// identity and pending Poll evidence, never a synthetic operational state.
func (a *binParser) routingConversation(e *ProtocolEvent, m map[string]any, protocol string, explicit bool) bool {
	a.udpMu.Lock()
	defer a.udpMu.Unlock()
	store := a.udpSessions
	if store == nil {
		store = &binUDPStore{entries: map[binUDPKey]*list.Element{}}
		a.udpSessions = store
	}
	x, y := e.Source, e.Destination
	dir := 0
	var ids [2]uint64
	identity := ""
	if protocol == "bfd" {
		x, y = routingHost(x), routingHost(y)
		ids = [2]uint64{routingNumber(m, "My Discriminator"), routingNumber(m, "Your Discriminator")}
		if x > y {
			x, y = y, x
			ids[0], ids[1] = ids[1], ids[0]
			dir = 1
		}
		if m["Multipoint"] == true {
			m["Association"] = "unavailable: multipoint receiver not selected"
			return true
		}
	} else {
		identity = fmt.Sprint(m["Identifier"])
		if protocol == "glbp" {
			identity = fmt.Sprint(m["Owner ID"])
		}
	}
	key := binUDPKey{fmt.Sprintf("routing/%s/%s/%d", protocol, x, ids[0]), fmt.Sprintf("%s/%d", y, ids[1]), e.Domain}
	if protocol != "bfd" {
		key.a = fmt.Sprintf("routing/%s/%s/%d", protocol, x, routingNumber(m, "Protocol Version"))
		if protocol == "glbp" {
			key.a = fmt.Sprintf("routing/glbp/%s/%d", x, routingNumber(m, "Group"))
		}
	}
	el := store.entries[key]
	if el != nil && e.Timestamp.Sub(el.Value.(*binUDPEntry).touched) > 2*time.Minute {
		old := el.Value.(*binUDPEntry)
		old.flow.closeSession()
		delete(store.entries, old.key)
		store.lru.Remove(el)
		el = nil
	}
	if protocol == "bfd" && el == nil {
		// A captured Down bootstrap with YourDiscriminator=0 can acquire the
		// peer discriminator only from an unambiguous reciprocal observation.
		var candidates []*list.Element
		for _, candidate := range store.entries {
			v := candidate.Value.(*binUDPEntry)
			r := v.flow.routing
			if r == nil || v.key.domain != e.Domain || v.flow.protocol != "bfd" || v.flow.endpoints != [2]string{x, y} {
				continue
			}
			if e.Timestamp.Sub(v.touched) > 2*time.Minute {
				continue
			}
			if ids[0] != 0 && r.discriminators[0] != 0 && ids[0] != r.discriminators[0] || ids[1] != 0 && r.discriminators[1] != 0 && ids[1] != r.discriminators[1] {
				continue
			}
			shared := ids[0] != 0 && ids[0] == r.discriminators[0] || ids[1] != 0 && ids[1] == r.discriminators[1]
			if shared {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) == 1 {
			el = candidates[0]
		}
	}
	if el != nil {
		v := el.Value.(*binUDPEntry)
		r := v.flow.routing
		if protocol == "glbp" && !stream_parser.RoutingControlStrong(m) && r.identity != identity {
			m["Association"] = "unavailable: no matching observed GLBP owner identity"
			return explicit
		}
		if protocol != "bfd" && stream_parser.RoutingControlStrong(m) && r.identity != "" && r.identity != identity {
			v.flow.closeSession()
			delete(store.entries, v.key)
			store.lru.Remove(el)
			el = nil
		}
		if protocol == "bfd" && r.seenPeer[0] && r.seenPeer[1] && routingNumber(m, "Your Discriminator") == 0 && !e.Timestamp.Before(r.last) {
			v.flow.closeSession()
			delete(store.entries, v.key)
			store.lru.Remove(el)
			el = nil
		}
	}
	if el == nil {
		if !stream_parser.RoutingControlStrong(m) {
			m["Association"] = "unavailable: no complete sender identity observed"
			return explicit
		}
		if len(store.entries) >= sessionCollectionLimit(a.budget.MaxCollectionElements) {
			m["Association"] = "unavailable: routing observation capacity"
			return true
		}
		f := &binFlow{a: a, id: a.flows.Add(1), protocol: protocol, domain: e.Domain, endpoints: [2]string{x, y}, routing: &routingDatagramState{identity: identity, discriminators: ids, last: e.Timestamp}}
		if f.reserveSession(2048) != nil {
			m["Association"] = "unavailable: routing observation byte budget"
			return true
		}
		el = store.lru.PushBack(&binUDPEntry{key, f, e.Timestamp})
		store.entries[key] = el
	}
	v := el.Value.(*binUDPEntry)
	f := v.flow
	r := f.routing
	if e.Timestamp.After(v.touched) {
		v.touched = e.Timestamp
	}
	store.lru.MoveToBack(el)
	e.FlowID, e.Direction = f.id, dir
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	if protocol != "bfd" {
		m["Association"] = "observed sender/interface; group fields belong only to this datagram"
		if !stream_parser.RoutingControlStrong(m) {
			m["Identity Evidence"] = "prior complete sender advertisement; no group fields inherited"
		}
		return true
	}
	if e.Timestamp.Before(r.last) {
		m["Association"] = "historical observation; pending Poll context unchanged"
		return true
	}
	r.last = e.Timestamp
	for i := range ids {
		if ids[i] != 0 {
			r.discriminators[i] = ids[i]
		}
	}
	if routingNumber(m, "Your Discriminator") != 0 {
		r.seenPeer[dir] = true
	}
	// Re-key a one-sided bootstrap after the reciprocal discriminator arrived.
	boundKey := binUDPKey{fmt.Sprintf("routing/bfd/%s/%d", x, r.discriminators[0]), fmt.Sprintf("%s/%d", y, r.discriminators[1]), e.Domain}
	if boundKey != v.key {
		if store.entries[boundKey] == nil {
			delete(store.entries, v.key)
			v.key = boundKey
			store.entries[boundKey] = el
		}
	}
	m["Association"] = "observed discriminator pair; advertised state is not verified link health"
	m["Both Peer Discriminators Observed"] = r.seenPeer[0] && r.seenPeer[1]
	if m["Final"] == true {
		if !r.pollAmbiguous[1-dir] && r.poll[1-dir] != 0 && !e.Timestamp.Before(r.pollTime[1-dir]) {
			e.ResponseTo = r.poll[1-dir]
			e.TransactionID = e.ResponseTo
		}
		r.poll[1-dir] = 0
		r.pollAmbiguous[1-dir] = false
	}
	if m["Poll"] == true {
		if r.poll[dir] != 0 || r.pollAmbiguous[dir] {
			r.poll[dir] = 0
			r.pollAmbiguous[dir] = true
			m["Poll Context"] = "ambiguous: repeated Poll without observed Final; no unique wire transaction ID"
		} else {
			r.poll[dir] = e.ID
			r.pollTime[dir] = e.Timestamp
		}
		e.TransactionID = r.poll[dir]
	}
	return true
}
