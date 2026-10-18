package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

const bacnetRoutedReadProfile = "bacnet-ip-routed-read-property"

type bacnetRouteAddress struct {
	network uint16
	address []byte
}
type bacnetRoutes struct {
	source, destination *bacnetRouteAddress
	hop                 uint8
	header              []byte
	offset              int
}

// Selected BACnet1.4.1 NPDU address layout: destination network/length/address,
// source network/length/address, then destination hop count. A source-only
// message has no encoded hop count. Network-message and reserved controls are
// not interpreted as application data. MAC bytes are literal observations,
// not authenticated MS/TP or Ethernet device identities.
func decodeBACnetRoutes(w []byte) (*bacnetRoutes, error) {
	if len(w) < 6 || w[4] != 1 {
		return nil, bacnetReadError(ErrMalformedMessage, "short routed NPDU")
	}
	control := w[5]
	if control&0xd0 != 0 {
		return nil, bacnetReadError(ErrUnsupportedFeature, "network message or reserved NPDU control")
	}
	n := &bacnetRoutes{}
	at := 6
	address := func(source bool) (*bacnetRouteAddress, error) {
		if len(w)-at < 3 {
			return nil, bacnetReadError(ErrMalformedMessage, "short routing network/address length")
		}
		net := binary.BigEndian.Uint16(w[at : at+2])
		count := int(w[at+2])
		at += 3
		if count > len(w)-at {
			return nil, bacnetReadError(ErrMalformedMessage, "routing address exceeds NPDU")
		}
		if net == 0 || source && (net == 65535 || count == 0) {
			return nil, bacnetReadError(ErrMalformedMessage, "invalid source/destination routing network")
		}
		if !source && (net == 65535 || count == 0) {
			return nil, bacnetReadError(ErrUnsupportedFeature, "broadcast routing destination has no unique ReadProperty peer")
		}
		if count > 6 {
			return nil, bacnetReadError(ErrUnsupportedFeature, "route addresses longer than selected six-byte profile")
		}
		out := &bacnetRouteAddress{network: net, address: bytes.Clone(w[at : at+count])}
		at += count
		return out, nil
	}
	var err error
	if control&0x20 != 0 {
		n.destination, err = address(false)
		if err != nil {
			return nil, err
		}
	}
	if control&8 != 0 {
		n.source, err = address(true)
		if err != nil {
			return nil, err
		}
	}
	if n.destination != nil {
		if at >= len(w) {
			return nil, bacnetReadError(ErrMalformedMessage, "missing routed destination hop count")
		}
		n.hop = w[at]
		at++
	}
	n.offset = at
	n.header = bytes.Clone(w[4:at])
	return n, nil
}
func (n *bacnetRoutes) fields(f map[string]any) {
	f["Source Route Present"], f["Destination Route Present"], f["Hop Count Present"] = n.source != nil, n.destination != nil, n.destination != nil
	f["NPDU Header"], f["APDU Offset"] = bytes.Clone(n.header), n.offset
	for key, v := range map[string]*bacnetRouteAddress{"Source Route": n.source, "Destination Route": n.destination} {
		if v != nil {
			f[key] = map[string]any{"Network": v.network, "Address": bytes.Clone(v.address)}
		}
	}
	if n.destination != nil {
		f["Hop Count"] = n.hop
	}
}
func bacnetLogicalAddress(endpoint string, address *bacnetRouteAddress) string {
	if address == nil {
		return "udp/" + endpoint
	}
	return fmt.Sprintf("network/%d/%s", address.network, hex.EncodeToString(address.address))
}

// Confirmed services share the originating endpoint's invoke namespace.
// A complete unsupported service header can retire that one identity without
// inventing service/body semantics or discarding unrelated pending requests.
// Unreadable, selected-service and segmented headers take the refusal path.
func bacnetForeignInvokeHint(w []byte, routed bool) *bacnetReadMessage {
	if len(w) < 6 || w[0] != 0x81 || (w[1] != 10 && w[1] != 11) || int(binary.BigEndian.Uint16(w[2:4])) != len(w) || w[4] != 1 {
		return nil
	}
	offset := 6
	var routes *bacnetRoutes
	if routed {
		var err error
		routes, err = decodeBACnetRoutes(w)
		if err != nil {
			return nil
		}
		offset = routes.offset
	} else if w[5]&0xf8 != 0 {
		return nil
	}
	a := w[offset:]
	if len(a) < 3 {
		return nil
	}
	m := &bacnetReadMessage{namespaceOnly: true, routes: routes}
	service := byte(12)
	switch a[0] {
	case 0, 2:
		if len(a) < 4 || a[1]&0x80 != 0 || a[1]&15 > 5 {
			return nil
		}
		m.invoke, m.request, service = a[2], true, a[3]
	case 0x20:
		if len(a) != 3 {
			return nil
		}
		m.invoke, service = a[1], a[2]
	case 0x30, 0x50:
		m.invoke, service = a[1], a[2]
	default:
		return nil
	}
	if service == 12 {
		return nil
	}
	return m
}
