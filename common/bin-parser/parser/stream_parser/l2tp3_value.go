package stream_parser

import (
	"bytes"
	"fmt"
	"net"
	"strconv"
	"unicode/utf8"
)

// L2TPv3DataContext is an explicitly supplied or observed receiver proposal.
// The sender uses the receiving peer's cookie/sublayer requirements. Cookie
// equality is an association check, never cryptographic authentication.
type L2TPv3DataContext struct {
	Cookie                           []byte
	Sublayer, Pseudowire, Sequencing uint16
}

func L2TPv3MessageName(typ uint64) string {
	if name := map[uint64]string{1: "SCCRQ", 2: "SCCRP", 3: "SCCCN", 4: "StopCCN", 6: "HELLO", 7: "OCRQ", 8: "OCRP", 9: "OCCN", 10: "ICRQ", 11: "ICRP", 12: "ICCN", 14: "CDN", 15: "WEN", 16: "SLI", 20: "ACK"}[typ]; name != "" {
		return name
	}
	return "Unknown"
}

func l2tp3Charge(r *controlReader, budget *int) bool {
	if *budget <= 0 {
		r.err = ErrObservedWireBudget
		return false
	}
	*budget--
	return true
}

// DecodeL2TPv3Wire validates a complete IP/UDP v3 wire envelope. The IP control
// length excludes its four-byte zero Session ID. Data messages intentionally
// retain the opaque remainder until a separate receiver context is supplied.
func DecodeL2TPv3Wire(w []byte, overUDP bool, budget int) (map[string]any, error) {
	if budget <= 0 {
		return nil, ErrObservedWireBudget
	}
	if len(w) < 4 || len(w) > 65535 {
		return nil, fmt.Errorf("l2tp3: bounded complete datagram/header required")
	}
	r := &controlReader{w: w, info: map[string]any{"Version": uint64(3), "Authentication Verified": false, "Payload Decoded": false}}
	r.info["Observation Scope"] = "observed wire parameters; no authenticated peer or installed pseudowire claim"
	control := false
	start := 0
	if overUDP {
		flags := r.num("Flags", 2, false)
		r.need(flags&15 == 3, "L2TPv3 UDP version")
		control = flags&0x8000 != 0
		if !control {
			r.num("Reserved", 2, false)
			r.num("Session ID", 4, false)
		}
	} else {
		control = r.num("Session ID", 4, false) == 0
		start = 4
		if control {
			r.num("Flags", 2, false)
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["Control"] = control
	if !control {
		r.need(r.info["Session ID"] != uint64(0), "L2TPv3 data session ID is zero")
		if r.err != nil {
			return nil, r.err
		}
		r.info["Packet Name"] = "Data"
		r.info["Envelope Verified"] = true
		r.info["Payload Offset"] = uint64(r.at)
		r.info["Wire Completeness"] = "session envelope; cookie, sublayer and pseudowire require receiver context"
		r.rest("Opaque Session Payload")
		r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
		l2tp3FlagRanges(r.info)
		return r.info, nil
	}
	flags := r.info["Flags"].(uint64)
	r.need(flags&15 == 3 && flags&0xc800 == 0xc800, "L2TPv3 control requires version3 and T/L/S")
	length := r.num("Length", 2, false)
	r.need(length >= 12 && length == uint64(len(w)-start), "L2TPv3 exact control length (excludes IP zero Session ID)")
	r.num("Control Connection ID", 4, false)
	r.num("Ns", 2, false)
	r.num("Nr", 2, false)
	if r.err != nil {
		return nil, r.err
	}
	r.info["Packet Name"] = "ZLB acknowledgment"
	r.info["Zero Length Body"] = r.at == len(w)
	var avps []map[string]any
	seen := map[uint64]bool{}
	for r.at < len(w) && r.err == nil {
		if !l2tp3Charge(r, &budget) {
			break
		}
		c := &controlReader{w: w, at: r.at, info: map[string]any{}}
		fl := c.num("Flags and Length", 2, false)
		n := int(fl & 0x3ff)
		c.need(n >= 6 && n <= len(w)-r.at, "L2TPv3 AVP boundary")
		if c.err != nil {
			return nil, c.err
		}
		end := r.at + n
		c.w = w[:end]
		vendor := c.num("Vendor ID", 2, false)
		typ := c.num("Attribute Type", 2, false)
		c.info["AVP Length"], c.info["Mandatory"], c.info["Hidden"] = uint64(n), fl&0x8000 != 0, fl&0x4000 != 0
		c.info["Reserved AVP Flags"] = fl & 0x3c00 // ignored on receipt, not a signature
		if len(avps) == 0 {
			c.need(typ == 0 && fl&0x4000 == 0 && n == 8, "L2TP first AVP must be a clear eight-byte Message Type")
		}
		if c.err != nil {
			return nil, c.err
		}
		if fl&0x4000 != 0 {
			c.rest("Hidden Value")
			c.info["Value Decoded"] = false
		} else if vendor != 0 {
			c.rest("Vendor Value")
			c.info["Value Decoded"] = false
		} else {
			decodeL2TPv3AVP(c, typ, &budget)
		}
		c.need(c.at == end, "L2TPv3 AVP trailing bytes")
		if c.err != nil {
			return nil, c.err
		}
		if vendor == 0 {
			if seen[typ] && typ != 36 && typ != 59 {
				r.info["Ambiguous Parameters"] = true
			}
			seen[typ] = true
		}
		if vendor == 0 && typ == 59 { // RFC3931 permits two adjacent digests during an algorithm transition.
			count := 0
			for _, a := range avps {
				if a["Vendor ID"] == uint64(0) && a["Attribute Type"] == uint64(59) {
					count++
				}
			}
			c.need(len(avps) == 1 || len(avps) == 2 && count == 1, "L2TP message digests must immediately follow Message Type")
			if c.err != nil {
				return nil, c.err
			}
		}
		if c.info["Value Decoded"] == false && fl&0x8000 != 0 {
			r.info["Unparsed Mandatory Parameters"] = true
		}
		if c.info["Value Decoded"] == false {
			r.info["Unparsed Parameters"] = true
		}
		if len(avps) == 0 {
			if vendor == 0 {
				mt := c.info["Message Type"].(uint64)
				r.info["Message Type"], r.info["Packet Name"] = mt, L2TPv3MessageName(mt)
				if L2TPv3MessageName(mt) == "Unknown" {
					r.info["Unparsed Message Type"] = true
				}
				r.need(L2TPv3MessageName(mt) == "Unknown" || fl&0x8000 != 0, "base Message Type must be mandatory")
			} else {
				r.info["Packet Name"] = "Vendor control message"
				r.info["Unparsed Message Type"] = true
			}
		} else if vendor == 0 && typ == 0 {
			return nil, fmt.Errorf("l2tp3: duplicate Message Type")
		}
		c.info["Wire Field Ranges"] = routingFieldRanges(c.fields)
		ranges := c.info["Wire Field Ranges"].(map[string]any)
		for _, key := range []string{"AVP Length", "Mandatory", "Hidden", "Reserved AVP Flags"} {
			ranges[key] = ranges["Flags and Length"]
		}
		if typ == 71 && vendor == 0 {
			ranges["Active"], ranges["New Circuit"] = ranges["Circuit Status"], ranges["Circuit Status"]
		}
		avps = append(avps, c.info)
		r.at = end
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["AVPs"] = avps
	if len(avps) > 0 && r.info["Unparsed Message Type"] != true {
		if err := l2tp3Mandatory(r.info, avps); err != nil {
			return nil, err
		}
	}
	r.info["Envelope Verified"] = true
	r.info["Wire Completeness"] = "complete control message"
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	l2tp3FlagRanges(r.info)
	return r.info, nil
}

func l2tp3FlagRanges(m map[string]any) {
	if _, ok := m["Flags"]; ok {
		ranges := m["Wire Field Ranges"].(map[string]any)
		ranges["Version"], ranges["Control"] = ranges["Flags"], ranges["Flags"]
	}
}

func l2tp3Text(r *controlReader, key string) {
	b := r.rest(key)
	if r.err != nil {
		return
	}
	if utf8.Valid(b) {
		r.info[key] = string(b)
	} else {
		r.info[key] = strconv.QuoteToASCII(string(b))
		r.info["Text Encoding"] = "invalid UTF-8; lossless escaped display"
	}
}

func decodeL2TPv3AVP(r *controlReader, typ uint64, budget *int) {
	r.info["Value Decoded"] = true
	switch typ {
	case 0:
		r.num("Message Type", 2, false)
	case 1:
		r.num("Result Code", 2, false)
		if r.at < len(r.w) {
			r.num("Error Code", 2, false)
			l2tp3Text(r, "Error Message")
		}
	case 2:
		r.num("Protocol Version", 1, false)
		r.num("Protocol Revision", 1, false)
	case 3:
		r.num("Framing Capabilities", 4, false)
	case 4:
		r.num("Bearer Capabilities", 4, false)
	case 5:
		r.num("Tie Breaker", 8, false) // Same AVP serves control and session exchanges.
	case 6:
		r.num("Firmware Revision", 2, false)
	case 7:
		l2tp3Text(r, "Host Name")
	case 8:
		l2tp3Text(r, "Vendor Name")
	case 10:
		r.need(r.num("Receive Window", 2, false) > 0, "L2TP receive window is zero")
	case 15:
		r.num("Serial Number", 4, false)
	case 18:
		r.num("Bearer Type", 4, false)
	case 19:
		r.num("Framing Type", 4, false)
	case 24:
		r.num("Tx Connect Speed", 4, false)
	case 25:
		r.num("Physical Channel ID", 4, false)
	case 36:
		r.rest("Random Vector")
	case 35:
		r.num("ACCM Reserved", 2, false)
		r.num("Send ACCM", 4, false)
		r.num("Receive ACCM", 4, false)
	case 38:
		r.num("Rx Connect Speed", 4, false)
	case 58:
		r.num("Extended Vendor ID", 4, false)
	case 59:
		alg := r.num("Digest Algorithm", 1, false)
		digest := r.rest("Message Digest")
		if alg == 0 {
			r.need(len(digest) == 16, "L2TP HMAC-MD5 digest width")
		} else if alg == 1 {
			r.need(len(digest) == 20, "L2TP HMAC-SHA1 digest width")
		} else {
			r.info["Value Decoded"] = false
		}
		r.info["Digest Verified"] = false
	case 60:
		routingAddress(r, "Router ID", 4)
	case 61:
		r.need(r.num("Assigned Control Connection ID", 4, false) != 0, "L2TP assigned control ID is zero")
	case 62:
		r.need(len(r.w)-r.at > 0 && (len(r.w)-r.at)%2 == 0, "L2TP pseudowire capability list width")
		var pws []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !l2tp3Charge(r, budget) {
				break
			}
			c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
			c.num("Pseudowire Type", 2, false)
			pws = append(pws, map[string]any{"Pseudowire Type": c.info["Pseudowire Type"], "Wire Field Ranges": routingFieldRanges(c.fields)})
			r.at = c.at
		}
		r.info["Pseudowire Capabilities"] = pws
	case 63:
		r.need(r.num("Local Session ID", 4, false) != 0, "L2TP local session ID is zero")
	case 64:
		r.num("Remote Session ID", 4, false)
	case 65:
		n := len(r.w) - r.at
		r.need(n == 0 || n == 4 || n == 8, "L2TP cookie length must be zero, four or eight")
		r.rest("Assigned Cookie")
	case 66:
		r.rest("Remote End ID")
	case 68:
		r.num("Pseudowire Type", 2, false)
	case 69:
		r.num("Sublayer Type", 2, false)
	case 70:
		r.num("Data Sequencing", 2, false)
	case 71:
		v := r.num("Circuit Status", 2, false)
		r.info["Active"], r.info["New Circuit"] = v&1 != 0, v&2 != 0
	case 72:
		l2tp3Text(r, "Preferred Language")
	case 73:
		r.rest("Authentication Nonce")
	case 74:
		r.num("Tx Connect Speed", 8, false)
	case 75:
		r.num("Rx Connect Speed", 8, false)
	default:
		r.rest("Unknown Value")
		r.info["Value Decoded"] = false
	}
}

func l2tp3Mandatory(m map[string]any, avps []map[string]any) error {
	typ, _ := m["Message Type"].(uint64)
	var required []uint64
	switch typ {
	case 1, 2:
		required = []uint64{7, 60, 61, 62}
	case 4:
		required = []uint64{1}
	case 7, 10:
		required = []uint64{63, 64, 15, 68, 66, 71}
	case 8, 11:
		required = []uint64{63, 64, 71}
	case 9, 12:
		required = []uint64{63, 64}
	case 14:
		required = []uint64{1, 63, 64}
	case 15, 16:
		required = []uint64{63, 64}
	}
	for _, want := range required {
		found := false
		for _, a := range avps {
			if a["Vendor ID"] == uint64(0) && a["Attribute Type"] == want {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("l2tp3: %s missing required AVP %d", L2TPv3MessageName(typ), want)
		}
	}
	return nil
}

// DecodeL2TPv3Data requires explicit receiver requirements. It never searches
// the opaque payload for a plausible Ethernet/IP header to guess cookie size.
func DecodeL2TPv3Data(w []byte, overUDP bool, context L2TPv3DataContext, budget int) (map[string]any, error) {
	m, err := DecodeL2TPv3Wire(w, overUDP, budget)
	if err != nil {
		return nil, err
	}
	if m["Control"] == true {
		return nil, fmt.Errorf("l2tp3: data decoder received control message")
	}
	if len(context.Cookie) != 0 && len(context.Cookie) != 4 && len(context.Cookie) != 8 {
		return nil, fmt.Errorf("l2tp3: invalid supplied cookie width")
	}
	if context.Sublayer > 1 || context.Sequencing > 2 {
		return m, fmt.Errorf("%w: unselected L2TP sublayer/sequencing", ErrObservedWireContext)
	}
	r := &controlReader{w: w, at: int(m["Payload Offset"].(uint64)), info: map[string]any{}}
	cookie := r.take("Cookie", len(context.Cookie))
	defer func() {
		ranges := m["Wire Field Ranges"].(map[string]any)
		for k, v := range routingFieldRanges(r.fields) {
			ranges[k] = v
		}
		if _, ok := m["Default Sublayer"]; ok {
			for _, key := range []string{"Sequence Present", "Sequence Number", "Sublayer Reserved"} {
				ranges[key] = ranges["Default Sublayer"]
			}
		}
	}()
	if r.err != nil {
		return m, r.err
	}
	m["Cookie"] = r.info["Cookie"]
	if !bytes.Equal(cookie, context.Cookie) {
		return m, fmt.Errorf("l2tp3: cookie does not match receiver proposal")
	}
	m["Cookie Matches Proposal"] = true
	m["Cookie Length"], m["Sublayer Type"], m["Pseudowire Type"], m["Data Sequencing"] = uint64(len(context.Cookie)), uint64(context.Sublayer), uint64(context.Pseudowire), uint64(context.Sequencing)
	if context.Sublayer == 1 {
		v := r.num("Default Sublayer", 4, false)
		if r.err == nil {
			m["Default Sublayer"] = v
		}
		m["Sequence Present"], m["Sequence Number"] = v&0x40000000 != 0, v&0xffffff
		m["Sublayer Reserved"] = v & 0xbf000000
		m["Sequence Meaningful"] = v&0x40000000 != 0 && context.Sequencing != 0
	}
	if r.err != nil {
		return m, r.err
	}
	if context.Sequencing != 0 && context.Sublayer != 1 {
		return m, fmt.Errorf("%w: sequencing requires selected default sublayer", ErrObservedWireContext)
	}
	m["Payload Offset"] = uint64(r.at)
	if context.Pseudowire != 4 && context.Pseudowire != 5 {
		return m, fmt.Errorf("%w: unselected L2TP pseudowire type %d", ErrObservedWireContext, context.Pseudowire)
	}
	ethernet := r.rest("Ethernet Bytes")
	if len(ethernet) < 14 {
		return m, fmt.Errorf("l2tp3: incomplete Ethernet pseudowire frame")
	}
	etherType := uint64(ethernet[12])<<8 | uint64(ethernet[13])
	// A VLAN tag includes both TCI and the next EtherType, even when no
	// sequencing policy is selected. A short tag is not an Ethernet header.
	at := 12
	for etherType == 0x8100 || etherType == 0x88a8 {
		if budget <= 0 {
			return m, ErrObservedWireBudget
		}
		budget--
		if at+6 > len(ethernet) {
			return m, fmt.Errorf("l2tp3: truncated Ethernet VLAN tag")
		}
		at += 4
		etherType = uint64(ethernet[at])<<8 | uint64(ethernet[at+1])
	}
	sequenced := m["Sequence Present"] == true
	if context.Sequencing == 2 && !sequenced {
		return m, fmt.Errorf("l2tp3: receiver requires every frame to be sequenced")
	}
	if context.Sequencing == 1 {
		isIP := etherType == 0x800 || etherType == 0x86dd
		if sequenced == isIP {
			return m, fmt.Errorf("l2tp3: non-IP sequencing requirement violated")
		}
	}
	m["Ethernet Bytes"], m["Ethernet Source"], m["Ethernet Destination"] = r.info["Ethernet Bytes"], net.HardwareAddr(ethernet[6:12]).String(), net.HardwareAddr(ethernet[:6]).String()
	m["Payload Decoded"], m["Wire Completeness"] = true, "complete selected pseudowire frame"
	delete(m, "Opaque Session Payload")
	return m, nil
}
