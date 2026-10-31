package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strings"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// These profiles describe bounded wire observations, not authenticated peers,
// completed VPNs, routing-table updates, or decoded encrypted payloads.
type controlReader struct {
	w      []byte
	at     int
	fields []tlsCertificateField
	info   map[string]any
	err    error
}

func (r *controlReader) take(name string, n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.w)-r.at {
		r.err = fmt.Errorf("network-control: truncated %s", name)
		return nil
	}
	a := r.at
	r.at += n
	b := r.w[a:r.at]
	r.fields = append(r.fields, tlsCertificateField{Name: name, Type: "raw", Start: a, End: r.at})
	r.info[name] = bytes.Clone(b)
	return b
}
func (r *controlReader) num(name string, n int, little bool) uint64 {
	b := r.take(name, n)
	if r.err != nil {
		return 0
	}
	var v uint64
	if little {
		for i := len(b) - 1; i >= 0; i-- {
			v = v<<8 | uint64(b[i])
		}
	} else {
		for _, c := range b {
			v = v<<8 | uint64(c)
		}
	}
	f := &r.fields[len(r.fields)-1]
	f.Type = fmt.Sprintf("uint%d", 8*n)
	if little {
		f.Endian = "little"
	}
	r.info[name] = v
	return v
}
func (r *controlReader) need(ok bool, msg string) {
	if !ok && r.err == nil {
		r.err = fmt.Errorf("network-control: %s", msg)
	}
}
func (r *controlReader) rest(name string) []byte { return r.take(name, len(r.w)-r.at) }
func (r *controlReader) child(name string, end int, fn func(*controlReader)) map[string]any {
	if r.err != nil {
		return nil
	}
	if end < r.at || end > len(r.w) {
		r.need(false, "invalid child boundary")
		return nil
	}
	a := r.at
	c := &controlReader{w: r.w[:end], at: a, info: map[string]any{}}
	fn(c)
	c.need(c.at == end, "child has trailing bytes")
	if c.err != nil {
		r.err = c.err
		return nil
	}
	r.at = end
	r.fields = append(r.fields, tlsCertificateField{Name: name, Start: a, End: end, Children: c.fields})
	return c.info
}

func DecodeNetworkControl(w []byte, profile string) (map[string]any, error) {
	_, m, e := decodeNetworkControl(w, profile)
	return m, e
}
func parseNetworkControl(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeNetworkControl(w, profile) }, "network-control-"+profile)
}
func decodeNetworkControl(w []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	r := &controlReader{w: w, info: map[string]any{"Profile": profile, "Authentication Verified": false, "Peer Identity Verified": false, "Observation Scope": "bounded wire message"}}
	r.need(len(w) > 0 && len(w) <= 65507, "message outside 1..65507 byte profile")
	if r.err != nil {
		return nil, nil, r.err
	}
	switch profile {
	case "ike", "ike-natt":
		decodeControlIKE(r, profile == "ike-natt")
	case "wireguard":
		decodeControlWireGuard(r)
	case "rip":
		decodeControlRIP(r)
	case "nbt-dg":
		decodeControlNBTDG(r)
	case "onc-rpc":
		decodeControlRPC(r)
	case "l2tp":
		decodeControlL2TP(r)
	case "openvpn":
		decodeControlOpenVPN(r)
	case "ipmi":
		decodeControlIPMI(r)
	case "telnet":
		decodeControlTelnet(r)
	case "nbss":
		decodeControlNBSS(r)
	default:
		r.need(false, "unsupported control profile")
	}
	r.need(r.at == len(w), "trailing message bytes")
	if r.err != nil {
		return nil, nil, r.err
	}
	ranges := map[string]any{}
	for _, f := range r.fields {
		if f.Type != "" {
			ranges[f.Name] = []any{uint64(f.Start), uint64(f.End)}
		}
	}
	r.info["Wire Field Ranges"] = ranges
	return r.fields, r.info, nil
}

func decodeControlNBSS(r *controlReader) {
	t := r.num("Type", 1, false)
	flags := r.num("Flags", 1, false)
	n := r.num("Length", 2, false) + ((flags & 1) << 16)
	r.need(flags&0xfe == 0 && n == uint64(len(r.w)-4), "NBSS exact length/flags")
	r.info["Packet Name"] = map[uint64]string{0x81: "Session Request", 0x82: "Positive Session Response", 0x83: "Negative Session Response", 0x84: "Retarget Session Response", 0x85: "Session Keepalive"}[t]
	switch t {
	case 0x81:
		controlNetBIOSName(r, "Called Name")
		controlNetBIOSName(r, "Calling Name")
	case 0x82, 0x85:
		r.need(n == 0, "NBSS response/keepalive body")
	case 0x83:
		r.num("Error Code", 1, false)
	case 0x84:
		r.take("Retarget Address", 4)
		r.num("Retarget Port", 2, false)
	default:
		r.need(false, "NBSS control type")
	}
}

func decodeControlTelnet(r *controlReader) {
	if r.w[0] != 255 {
		r.need(!bytes.Contains(r.w, []byte{255}), "Telnet data frame crosses IAC boundary")
		r.info["Packet Name"] = "Data"
		r.rest("Data")
		return
	}
	r.num("IAC", 1, false)
	cmd := r.num("Command", 1, false)
	r.need(cmd >= 236, "Telnet command")
	r.info["Packet Name"] = map[uint64]string{236: "EOF", 237: "SUSP", 238: "ABORT", 239: "EOR", 240: "SE", 241: "NOP", 242: "Data Mark", 243: "Break", 244: "Interrupt Process", 245: "Abort Output", 246: "Are You There", 247: "Erase Character", 248: "Erase Line", 249: "Go Ahead", 250: "Subnegotiation", 251: "WILL", 252: "WONT", 253: "DO", 254: "DONT", 255: "Escaped IAC"}[cmd]
	if cmd >= 251 && cmd <= 254 {
		r.num("Option", 1, false)
	}
	if cmd == 250 {
		r.num("Option", 1, false)
		start := r.at
		for r.err == nil && r.at < len(r.w) {
			if r.w[r.at] != 255 {
				r.at++
				continue
			}
			if r.at+1 >= len(r.w) {
				break
			}
			if r.w[r.at+1] == 255 {
				r.at += 2
				continue
			}
			if r.w[r.at+1] == 240 {
				end := r.at
				r.at = start
				r.take("Subnegotiation Data", end-start)
				r.num("End IAC", 1, false)
				r.num("End Command", 1, false)
				return
			}
			r.need(false, "Telnet unescaped IAC inside subnegotiation")
		}
		r.need(false, "Telnet subnegotiation missing IAC SE")
	}
}

func decodeControlWireGuard(r *controlReader) {
	t := r.num("Message Type", 4, true)
	r.need(t >= 1 && t <= 4, "WireGuard type or reserved bits")
	switch t {
	case 1:
		r.need(len(r.w) == 148, "WireGuard initiation must be 148 bytes")
		r.info["Packet Name"] = "Handshake Initiation"
		r.num("Sender Index", 4, true)
		r.take("Ephemeral Public Key", 32)
		r.take("Encrypted Static", 48)
		r.take("Encrypted Timestamp", 28)
		r.take("MAC1", 16)
		r.take("MAC2", 16)
	case 2:
		r.need(len(r.w) == 92, "WireGuard response must be 92 bytes")
		r.info["Packet Name"] = "Handshake Response"
		r.num("Sender Index", 4, true)
		r.num("Receiver Index", 4, true)
		r.take("Ephemeral Public Key", 32)
		r.take("Encrypted Empty", 16)
		r.take("MAC1", 16)
		r.take("MAC2", 16)
	case 3:
		r.need(len(r.w) == 64, "WireGuard cookie reply must be 64 bytes")
		r.info["Packet Name"] = "Cookie Reply"
		r.num("Receiver Index", 4, true)
		r.take("Nonce", 24)
		r.take("Encrypted Cookie", 32)
	case 4:
		// wireguard-go clips padding at the interface MTU. In particular a
		// 1420-byte inner packet yields 1452 wire bytes, not a multiple of 16.
		// Ciphertext padding cannot be checked without decrypting it.
		r.need(len(r.w) >= 32, "WireGuard transport size including authentication tag")
		r.info["Packet Name"] = "Transport Data"
		r.num("Receiver Index", 4, true)
		r.num("Counter", 8, true)
		r.rest("Encrypted Payload")
	}
	r.info["Content Visibility"] = "encrypted"
	r.info["Cryptographic Verification"] = "unavailable; no WireGuard receiver key supplied"
}

func decodeControlIKE(r *controlReader, natt bool) {
	if natt {
		r.need(bytes.Equal(r.take("Non-ESP Marker", 4), make([]byte, 4)), "NAT-T requires non-ESP marker")
	}
	start := r.at
	r.take("Initiator SPI", 8)
	r.take("Responder SPI", 8)
	next := r.num("Next Payload", 1, false)
	ver := r.num("Version", 1, false)
	exchange := r.num("Exchange Type", 1, false)
	flags := r.num("Flags", 1, false)
	r.num("Message ID", 4, false)
	n := r.num("Length", 4, false)
	r.need(n == uint64(len(r.w)-start) && n >= 28, "IKE exact declared length")
	r.need(ver == 0x10 || ver == 0x20, "IKE version")
	r.info["Packet Name"] = fmt.Sprintf("IKEv%d exchange %d", ver>>4, exchange)
	if ver == 0x20 {
		if name := map[uint64]string{34: "IKE_SA_INIT", 35: "IKE_AUTH", 36: "CREATE_CHILD_SA", 37: "INFORMATIONAL"}[exchange]; name != "" {
			r.info["Packet Name"] = name
		}
	}
	if ver == 0x20 {
		r.need(exchange >= 34 && exchange <= 44 && flags & ^uint64(0x38) == 0, "IKEv2 exchange/flags")
	} else {
		r.need(flags & ^uint64(7) == 0, "IKEv1 flags")
	}
	if ver == 0x10 && flags&1 != 0 {
		r.rest("Encrypted Payload")
		r.info["Content Visibility"] = "encrypted"
		return
	}
	var ps []map[string]any
	for next != 0 && r.err == nil {
		r.need(len(ps) < 256, "IKE payload count limit")
		if r.err != nil {
			break
		}
		start := r.at
		typ := next
		if len(r.w)-start < 4 {
			r.need(false, "IKE payload header truncated")
			break
		}
		next = uint64(r.w[start])
		length := int(binary.BigEndian.Uint16(r.w[start+2:]))
		r.need(length >= 4 && length <= len(r.w)-start, "IKE payload length")
		p := r.child(fmt.Sprintf("Payload %d", len(ps)+1), start+length, func(c *controlReader) {
			c.info["Payload Type"] = typ
			c.num("Next Payload", 1, false)
			c.num("Flags", 1, false)
			c.num("Length", 2, false)
			if ver == 0x20 {
				switch typ {
				case 33:
					decodeIKEv2Proposals(c)
					return
				case 34:
					c.num("DH Group", 2, false)
					c.num("Reserved", 2, false)
					c.need(c.at < len(c.w), "empty IKE key exchange")
					c.rest("Key Exchange Data")
					return
				case 40:
					c.need(len(c.w)-c.at >= 16 && len(c.w)-c.at <= 256, "IKE nonce length")
					c.rest("Nonce")
					return
				case 41:
					c.num("Protocol ID", 1, false)
					sn := int(c.num("SPI Size", 1, false))
					c.num("Notify Type", 2, false)
					c.take("SPI", sn)
					c.rest("Notification Data")
					return
				case 35, 36:
					c.num("ID Type", 1, false)
					c.take("Reserved", 3)
					c.rest("Identification Data")
					return
				case 46:
					c.need(c.at < len(c.w), "empty IKE SK payload")
					c.rest("Encrypted Payload")
					return
				case 53:
					frag := c.num("Fragment Number", 2, false)
					total := c.num("Total Fragments", 2, false)
					c.need(frag > 0 && frag <= total, "IKE encrypted fragment numbering")
					c.rest("Encrypted Fragment")
					return
				}
			}
			c.rest("Payload Data")
		})
		ps = append(ps, p)
		if ver == 0x20 && (typ == 46 || typ == 53) {
			next = 0
			r.info["Content Visibility"] = "encrypted"
		}
	}
	r.info["Payloads"] = ps
}

func decodeIKEv2Proposals(r *controlReader) {
	var proposals []map[string]any
	for r.at < len(r.w) && r.err == nil {
		start := r.at
		if len(r.w)-start < 8 {
			r.need(false, "IKE proposal header truncated")
			break
		}
		n := int(binary.BigEndian.Uint16(r.w[start+2:]))
		r.need(n >= 8 && n <= len(r.w)-start && len(proposals) < 64, "IKE proposal length/count")
		p := r.child(fmt.Sprintf("Proposal %d", len(proposals)+1), start+n, func(c *controlReader) {
			last := c.num("Last Substructure", 1, false)
			c.num("Reserved", 1, false)
			c.num("Proposal Length", 2, false)
			c.num("Proposal Number", 1, false)
			c.num("Protocol ID", 1, false)
			sn := int(c.num("SPI Size", 1, false))
			count := int(c.num("Transform Count", 1, false))
			c.take("SPI", sn)
			c.need(last == 0 || last == 2, "IKE proposal chain marker")
			var transforms []map[string]any
			for c.at < len(c.w) && c.err == nil {
				a := c.at
				if len(c.w)-a < 8 {
					c.need(false, "IKE transform truncated")
					break
				}
				size := int(binary.BigEndian.Uint16(c.w[a+2:]))
				c.need(size >= 8 && size <= len(c.w)-a && len(transforms) < 64, "IKE transform length/count")
				t := c.child(fmt.Sprintf("Transform %d", len(transforms)+1), a+size, func(d *controlReader) {
					marker := d.num("Last Substructure", 1, false)
					d.num("Reserved", 1, false)
					d.num("Transform Length", 2, false)
					d.num("Transform Type", 1, false)
					d.num("Reserved Type", 1, false)
					d.num("Transform ID", 2, false)
					d.need(marker == 0 || marker == 3, "IKE transform chain marker")
					var attributes []map[string]any
					for d.at < len(d.w) && d.err == nil {
						typ := d.num(fmt.Sprintf("Attribute %d Type", len(attributes)+1), 2, false)
						value := d.num(fmt.Sprintf("Attribute %d Value or Length", len(attributes)+1), 2, false)
						m := map[string]any{"Type": typ & 0x7fff, "Value": value}
						if typ&0x8000 == 0 {
							m["Value"] = d.take(fmt.Sprintf("Attribute %d Data", len(attributes)+1), int(value))
						}
						attributes = append(attributes, m)
					}
					d.info["Attributes"] = attributes
				})
				transforms = append(transforms, t)
				if t != nil {
					marker := t["Last Substructure"].(uint64)
					c.need((marker == 0) == (c.at == len(c.w)), "IKE transform chain termination")
				}
			}
			c.need(len(transforms) == count && count > 0, "IKE transform count mismatch")
			c.info["Transforms"] = transforms
		})
		proposals = append(proposals, p)
		if p != nil {
			last := p["Last Substructure"].(uint64)
			r.need((last == 0) == (r.at == len(r.w)), "IKE proposal chain termination")
		}
	}
	r.need(len(proposals) > 0, "empty IKE SA")
	r.info["Proposals"] = proposals
}

func decodeControlRIP(r *controlReader) {
	cmd := r.num("Command", 1, false)
	v := r.num("Version", 1, false)
	zero := r.num("Reserved", 2, false)
	r.need((cmd == 1 || cmd == 2) && (v == 1 || v == 2) && zero == 0, "RIP request/response header")
	r.need(len(r.w) >= 24 && (len(r.w)-4)%20 == 0 && (len(r.w)-4)/20 <= 25, "RIP exact route-entry length")
	r.info["Packet Name"] = map[uint64]string{1: "Request", 2: "Response"}[cmd]
	var entries []map[string]any
	for r.at < len(r.w) && r.err == nil {
		index := len(entries)
		e := r.child(fmt.Sprintf("Entry %d", index+1), r.at+20, func(c *controlReader) {
			af := c.num("Address Family", 2, false)
			tag := c.num("Route Tag", 2, false)
			if af == 65535 {
				c.need(index == 0 && v == 2 && tag == 2, "only simple-password RIP authentication profile supported")
				c.take("Authentication Data", 16)
				c.info["Authentication Verified"] = false
				return
			}
			ip := c.take("Address", 4)
			mask := c.take("Subnet Mask", 4)
			hop := c.take("Next Hop", 4)
			metric := c.num("Metric", 4, false)
			c.need((af == 2 || af == 0 && cmd == 1 && metric == 16) && metric >= 1 && metric <= 16, "RIP address family or metric")
			if v == 1 {
				c.need(tag == 0 && bytes.Equal(mask, make([]byte, 4)) && bytes.Equal(hop, make([]byte, 4)), "RIPv1 reserved route fields")
			}
			c.info["Address Text"], c.info["Mask Text"], c.info["Next Hop Text"] = net.IP(ip).String(), net.IP(mask).String(), net.IP(hop).String()
		})
		entries = append(entries, e)
	}
	r.info["Entries"] = entries
	if len(r.fields) >= 3 {
		children := append([]tlsCertificateField(nil), r.fields[3:]...)
		for i := range children {
			children[i].Name = "Entry"
		}
		r.fields = append(r.fields[:3], tlsCertificateField{Name: "Entries", Start: 4, End: len(r.w), List: true, Children: children})
	}
	r.info["Route Installation Verified"] = false
}

func controlNetBIOSName(r *controlReader, name string) {
	start := r.at
	n := r.num(name+" Label Length", 1, false)
	r.need(n == 32, "NetBIOS first-level encoded label length")
	b := r.take(name+" Encoded Label", 32)
	decoded := make([]byte, 16)
	for i := 0; i < len(b); i += 2 {
		if b[i] < 'A' || b[i] > 'P' || b[i+1] < 'A' || b[i+1] > 'P' {
			r.need(false, "NetBIOS encoded name nibble")
			return
		}
		decoded[i/2] = (b[i]-'A')<<4 | (b[i+1] - 'A')
	}
	labels := 0
	for r.err == nil {
		size := int(r.num(name+" Scope Length", 1, false))
		if size == 0 {
			break
		}
		labels++
		r.need(size <= 63 && r.at-start+size <= 255 && labels <= 16, "NetBIOS scope length")
		r.take(fmt.Sprintf("%s Scope %d", name, labels), size)
	}
	r.info[name] = strings.TrimRight(string(decoded[:15]), " ")
	r.info[name+" Suffix"] = decoded[15]
}
func decodeControlNBTDG(r *controlReader) {
	t := r.num("Message Type", 1, false)
	flags := r.num("Flags", 1, false)
	r.num("Datagram ID", 2, false)
	ip := r.take("Source IP", 4)
	r.num("Source Port", 2, false)
	r.need(t >= 0x10 && t <= 0x16 && flags&0xf0 == 0, "NetBIOS datagram type/flags")
	r.info["Source IP Text"] = net.IP(ip).String()
	r.info["Packet Name"] = map[uint64]string{0x10: "Direct Unique Datagram", 0x11: "Direct Group Datagram", 0x12: "Broadcast Datagram", 0x13: "Datagram Error", 0x14: "Query Request", 0x15: "Positive Query Response", 0x16: "Negative Query Response"}[t]
	if t == 0x13 {
		r.num("Error Code", 1, false)
		return
	}
	if t >= 0x14 {
		controlNetBIOSName(r, "Destination Name")
		return
	}
	n := r.num("Datagram Length", 2, false)
	off := r.num("Packet Offset", 2, false)
	r.need(n >= uint64(len(r.w)-14) && off <= n, "NetBIOS datagram length/offset")
	first, more := flags&2 != 0, flags&1 != 0
	if !more && first {
		r.need(n == uint64(len(r.w)-14) && off == 0, "NetBIOS complete datagram length")
	}
	if first {
		r.need(off == 0, "NetBIOS first fragment offset")
		controlNetBIOSName(r, "Source Name")
		controlNetBIOSName(r, "Destination Name")
	}
	r.rest("User Data")
	r.info["Fragmented"] = more || !first
	r.info["Reassembly Performed"] = false
}

func controlRPCAuth(r *controlReader, name string) {
	r.num(name+" Flavor", 4, false)
	n := int(r.num(name+" Length", 4, false))
	r.need(n <= 400, "RPC opaque_auth exceeds 400 bytes")
	r.take(name+" Body", n)
	pad := (4 - n%4) % 4
	r.take(name+" XDR Padding", pad)
}
func decodeControlRPC(r *controlReader) {
	r.num("XID", 4, false)
	t := r.num("Message Type", 4, false)
	r.need(t <= 1, "RPC message type")
	if t == 0 {
		r.info["Packet Name"] = "CALL"
		r.need(r.num("RPC Version", 4, false) == 2, "RPC version")
		r.num("Program", 4, false)
		r.num("Program Version", 4, false)
		r.num("Procedure", 4, false)
		controlRPCAuth(r, "Credentials")
		controlRPCAuth(r, "Verifier")
		r.rest("Procedure Arguments")
		return
	}
	r.info["Packet Name"] = "REPLY"
	stat := r.num("Reply Status", 4, false)
	r.need(stat <= 1, "RPC reply status")
	if stat == 0 {
		controlRPCAuth(r, "Verifier")
		accept := r.num("Accept Status", 4, false)
		r.need(accept <= 5, "RPC accepted status")
		if accept == 2 {
			r.num("Low Version", 4, false)
			r.num("High Version", 4, false)
		}
		if accept == 0 {
			r.rest("Procedure Result")
		}
	} else {
		reject := r.num("Reject Status", 4, false)
		r.need(reject <= 1, "RPC rejected status")
		if reject == 0 {
			r.num("Low Version", 4, false)
			r.num("High Version", 4, false)
		} else {
			r.num("Authentication Status", 4, false)
		}
	}
	r.info["Request Association"] = "unavailable; stateless datagram"
}

func decodeControlL2TP(r *controlReader) {
	flags := r.num("Flags", 2, false)
	r.need(flags&15 == 2 && flags & ^uint64(0xcb0f) == 0, "L2TPv2 flags/version")
	control := flags&0x8000 != 0
	if control {
		r.need(flags&0xc800 == 0xc800 && flags&0x300 == 0, "L2TP control requires T/L/S without offset/priority")
	}
	if flags&0x4000 != 0 {
		r.need(r.num("Length", 2, false) == uint64(len(r.w)), "L2TP exact length")
	}
	r.num("Tunnel ID", 2, false)
	r.num("Session ID", 2, false)
	if flags&0x800 != 0 {
		r.num("Ns", 2, false)
		r.num("Nr", 2, false)
	}
	if flags&0x200 != 0 {
		off := int(r.num("Offset Size", 2, false))
		r.take("Offset Padding", off)
	}
	if !control {
		r.info["Packet Name"] = "Data"
		r.rest("PPP Payload")
		return
	}
	r.info["Packet Name"] = "Zero-Length Body Acknowledgment"
	var avps []map[string]any
	for r.at < len(r.w) && r.err == nil {
		start := r.at
		if len(r.w)-start < 6 {
			r.need(false, "L2TP AVP header truncated")
			break
		}
		fl := binary.BigEndian.Uint16(r.w[start:])
		n := int(fl & 1023)
		r.need(fl&0x3c00 == 0 && n >= 6 && n <= len(r.w)-start && len(avps) < 256, "L2TP AVP flags/length")
		a := r.child(fmt.Sprintf("AVP %d", len(avps)+1), start+n, func(c *controlReader) {
			c.num("Flags and Length", 2, false)
			vendor := c.num("Vendor ID", 2, false)
			typ := c.num("Attribute Type", 2, false)
			if fl&0x4000 != 0 {
				c.rest("Hidden Value")
				return
			}
			if vendor == 0 {
				switch typ {
				case 0, 2, 9, 10, 14, 19, 23, 25, 26, 29, 31:
					c.num("Value", 2, false)
				case 3, 4, 5, 6, 15, 24, 32, 35, 36:
					c.num("Value", 4, false)
				default:
					c.rest("Value")
				}
			} else {
				c.rest("Value")
			}
		})
		avps = append(avps, a)
		if len(avps) == 1 && a != nil {
			r.need(a["Vendor ID"] == uint64(0) && a["Attribute Type"] == uint64(0) && fl&0xc000 == 0x8000, "L2TP first AVP must be clear mandatory Message Type")
			if v, ok := a["Value"].(uint64); ok {
				r.info["Message Type"] = v
				r.info["Packet Name"] = fmt.Sprintf("Control message %d", v)
				if name := map[uint64]string{1: "SCCRQ", 2: "SCCRP", 3: "SCCCN", 4: "StopCCN", 6: "HELLO", 7: "OCRQ", 8: "OCRP", 9: "OCCN", 10: "ICRQ", 11: "ICRP", 12: "ICCN", 14: "CDN", 15: "WEN", 16: "SLI"}[v]; name != "" {
					r.info["Packet Name"] = name
				}
			}
		}
	}
	r.info["AVPs"] = avps
}

func decodeControlOpenVPN(r *controlReader) {
	header := r.num("Opcode and Key ID", 1, false)
	op := header >> 3
	r.info["Opcode"] = op
	r.info["Key ID"] = header & 7
	r.need(op >= 1 && op <= 11, "OpenVPN opcode")
	r.info["Packet Name"] = map[uint64]string{1: "Hard Reset Client v1", 2: "Hard Reset Server v1", 3: "Soft Reset", 4: "Control", 5: "Acknowledgment", 6: "Data v1", 7: "Hard Reset Client v2", 8: "Hard Reset Server v2", 9: "Data v2", 10: "Hard Reset Client v3", 11: "Control WKC"}[op]
	if op == 6 || op == 9 {
		if op == 9 {
			r.take("Peer ID", 3)
		}
		r.need(r.at < len(r.w), "empty OpenVPN data")
		r.rest("Encrypted Payload")
		r.info["Content Visibility"] = "encrypted"
		return
	}
	r.take("Session ID", 8)
	// Authenticated/wrapped control adds fields whose lengths depend on config.
	// This explicit profile is the unauthenticated clear control envelope only.
	if op >= 10 {
		r.rest("Wrapped Control Data")
		r.info["Content Visibility"] = "encrypted"
		return
	}
	count := int(r.num("ACK Count", 1, false))
	r.need(count <= 64, "OpenVPN ACK count")
	for i := 0; i < count && r.err == nil; i++ {
		r.num(fmt.Sprintf("ACK Packet ID %d", i+1), 4, false)
	}
	if count > 0 {
		r.take("Remote Session ID", 8)
	}
	if op != 5 {
		r.num("Packet ID", 4, false)
		r.rest("Control Fragment")
	}
	r.info["Control Protection"] = "clear envelope only; tls-auth/tls-crypt configuration unavailable"
}

func decodeControlIPMI(r *controlReader) {
	r.need(r.num("RMCP Version", 1, false) == 6, "RMCP version")
	r.need(r.num("RMCP Reserved", 1, false) == 0, "RMCP reserved")
	r.num("RMCP Sequence", 1, false)
	class := r.num("RMCP Class", 1, false)
	r.need(class == 7 || class == 6, "IPMI/ASF RMCP message class")
	if class == 6 {
		r.num("IANA Enterprise", 4, false)
		r.num("ASF Message Type", 1, false)
		r.num("Message Tag", 1, false)
		r.num("ASF Reserved", 1, false)
		n := r.num("ASF Data Length", 1, false)
		r.need(n == uint64(len(r.w)-r.at), "ASF exact data length")
		r.rest("ASF Data")
		r.info["Packet Name"] = "ASF Presence Message"
		return
	}
	auth := r.num("Authentication Type", 1, false)
	if auth == 6 {
		t := r.num("Payload Type and Flags", 1, false)
		r.num("Session ID", 4, true)
		r.num("Session Sequence", 4, true)
		n := r.num("Payload Length", 2, true)
		r.need(n <= uint64(len(r.w)-r.at), "RMCP+ truncated payload")
		r.info["Payload Type"] = t & 63
		r.info["Payload Encrypted"] = t&0x80 != 0
		r.info["Payload Authenticated Flag"] = t&0x40 != 0
		r.info["Packet Name"] = fmt.Sprintf("RMCP+ payload %d", t&63)
		if name := map[uint64]string{0: "IPMI Message", 0x10: "Open Session Request", 0x11: "Open Session Response", 0x12: "RAKP Message 1", 0x13: "RAKP Message 2", 0x14: "RAKP Message 3", 0x15: "RAKP Message 4"}[t&63]; name != "" {
			r.info["Packet Name"] = name
		}
		if t&0x40 != 0 {
			// The payload length excludes the integrity trailer. Its MAC and
			// pad split depend on the negotiated algorithm, which this bounded
			// wire profile does not know. Preserve it without inventing a MAC
			// result or confusing integrity-only protection with encryption.
			end := r.at + int(n)
			trailer := len(r.w) - end
			r.need(trailer >= 14 && trailer <= 37, "RMCP+ integrity trailer outside bounded profile")
			r.info["Payload"] = r.child("Payload", end, func(c *controlReader) {
				if t&0x80 != 0 {
					c.rest("Encrypted Payload")
				} else if t&63 == 0 {
					decodeControlIPMIBody(c)
				} else {
					c.rest("Session Payload")
				}
			})
			r.rest("Integrity Trailer")
			r.info["Integrity Verification"] = "unavailable: negotiated integrity algorithm and keys required"
			if t&0x80 != 0 {
				r.info["Content Visibility"] = "encrypted"
			} else {
				r.info["Content Visibility"] = "clear; integrity unverified"
			}
			return
		}
		r.need(n == uint64(len(r.w)-r.at), "RMCP+ exact unauthenticated payload length")
		if t&0x80 != 0 {
			r.rest("Protected Payload")
			r.info["Content Visibility"] = "encrypted"
			return
		}
		if t&63 == 0 {
			decodeControlIPMIBody(r)
			return
		}
		if t&63 == 0x10 {
			r.num("Message Tag", 1, false)
			r.num("Requested Privilege", 1, false)
			r.take("Reserved", 2)
			r.num("Remote Console Session ID", 4, true)
			var algs []map[string]any
			for i := 0; i < 3 && r.err == nil; i++ {
				a := r.child(fmt.Sprintf("Algorithm %d", i), r.at+8, func(c *controlReader) {
					typ := c.num("Payload Type", 1, false)
					c.take("Reserved", 2)
					c.need(c.num("Payload Length", 1, false) == 8, "RMCP+ algorithm payload length")
					c.num("Algorithm", 1, false)
					c.take("Reserved Tail", 3)
					c.need(typ == uint64(i), "RMCP+ algorithm payload order")
				})
				algs = append(algs, a)
			}
			r.info["Algorithms"] = algs
			return
		}
		r.rest("Session Payload")
		return
	}
	r.need(auth == 0 || auth == 1 || auth == 2 || auth == 4 || auth == 5, "IPMI authentication type")
	r.num("Session Sequence", 4, true)
	r.num("Session ID", 4, true)
	if auth != 0 {
		r.take("Authentication Code", 16)
	}
	n := r.num("Message Length", 1, false)
	r.need(n == uint64(len(r.w)-r.at), "IPMI exact message length")
	r.info["Packet Name"] = "IPMI Message"
	decodeControlIPMIBody(r)
}
func decodeControlIPMIBody(r *controlReader) {
	start := r.at
	r.num("Responder Address", 1, false)
	r.num("NetFn and LUN", 1, false)
	r.num("Header Checksum", 1, false)
	r.num("Requester Address", 1, false)
	r.num("Request Sequence and LUN", 1, false)
	r.num("Command", 1, false)
	r.need(len(r.w)-r.at >= 1, "IPMI message checksum missing")
	r.take("Command Data", max(0, len(r.w)-r.at-1))
	r.num("Data Checksum", 1, false)
	if r.err == nil {
		var a, b byte
		for _, c := range r.w[start : start+3] {
			a += c
		}
		for _, c := range r.w[start+3:] {
			b += c
		}
		r.need(a == 0 && b == 0, "IPMI message checksum")
		r.info["Checksum Verified"] = true
	}
}
