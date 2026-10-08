package pcaputil

import (
	"bytes"
	"encoding/binary"
	"net"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// This Ethernet profile requires an explicit End TLV. It reports observed
// neighbor fields, not authenticated identity, topology state or PROFINET RT.
func decodeLLDP(w []byte, maxElements int) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) { return nil, protocolError(ErrMalformedMessage, "LLDP "+why) }
	var tlvs []map[string]any
	fields := map[string]any{"Observation": "unverified-neighbor"}
	for at := 0; at < len(w); {
		if len(tlvs) >= maxElements {
			return nil, protocolError(ErrResourceExceeded, "LLDP TLV collection budget")
		}
		if len(w)-at < 2 {
			return bad("TLV header is incomplete")
		}
		h := binary.BigEndian.Uint16(w[at:])
		typ, n := int(h>>9), int(h&511)
		end := at + 2 + n
		if n > len(w)-at-2 {
			return bad("TLV length exceeds captured bytes")
		}
		if len(tlvs) < 3 && typ != len(tlvs)+1 {
			return bad("mandatory Chassis/Port/TTL TLVs are out of order")
		}
		if len(tlvs) >= 3 && typ >= 1 && typ <= 3 {
			return bad("mandatory TLV is repeated")
		}
		v := w[at+2 : end]
		t := map[string]any{"Type": typ, "Length": n, "Offset": at, "Value": bytes.Clone(v)}
		switch typ {
		case 0:
			if n != 0 {
				return bad("End TLV has a value")
			}
			tlvs = append(tlvs, t)
			fields["TLVs"] = tlvs
			fields["PDU Length"] = end
			fields["Ethernet Trailer"] = bytes.Clone(w[end:])
			return fields, nil
		case 1, 2:
			if n < 2 || n > 256 || v[0] < 1 || v[0] > 7 {
				return bad("identity subtype/length is invalid")
			}
			id := map[string]any{"Subtype": v[0], "ID": bytes.Clone(v[1:])}
			mac, ip := byte(4), byte(5)
			name := "Chassis ID"
			if typ == 2 {
				mac, ip = 3, 4
				name = "Port ID"
			}
			if v[0] == mac {
				if n != 7 {
					return bad("MAC identity length is invalid")
				}
				id["MAC"] = net.HardwareAddr(v[1:]).String()
			} else if v[0] == ip {
				if n < 3 {
					return bad("network identity has no address")
				}
				id["Address Family"] = v[1]
				if v[1] == 1 || v[1] == 2 {
					length := 6
					if v[1] == 2 {
						length = 18
					}
					if n != length {
						return bad("network identity address length is invalid")
					}
					id["IP"] = net.IP(v[2:]).String()
				}
			}
			t["Identity"] = id
			fields[name] = cloneSession(id)
		case 3:
			if n != 2 {
				return bad("TTL must be two octets")
			}
			ttl := binary.BigEndian.Uint16(v)
			t["TTL Seconds"] = ttl
			fields["TTL Seconds"] = ttl
			fields["Shutdown Announcement"] = ttl == 0
		case 4, 5, 6:
			if n > 255 {
				return bad("text information exceeds 255 octets")
			}
			names := map[int]string{4: "Port Description", 5: "System Name", 6: "System Description"}
			t[names[typ]] = bytes.Clone(v)
		case 7:
			if n != 4 {
				return bad("capabilities must be four octets")
			}
			supported, enabled := binary.BigEndian.Uint16(v), binary.BigEndian.Uint16(v[2:])
			if enabled & ^supported != 0 {
				return bad("enabled capabilities are not a subset of supported capabilities")
			}
			t["Supported Capabilities"], t["Enabled Capabilities"] = supported, enabled
		case 8:
			if n < 8 {
				return bad("management address header is incomplete")
			}
			size := int(v[0])
			if size < 1 || size > 31 || size+7 > n {
				return bad("management address length is invalid")
			}
			family := v[1]
			addr := v[2 : 1+size]
			iface := v[1+size]
			oidn := int(v[6+size])
			if iface < 1 || iface > 3 || oidn > 128 || size+7+oidn != n {
				return bad("management interface/OID boundary is invalid")
			}
			m := map[string]any{"Address Family": family, "Address": bytes.Clone(addr), "Interface Subtype": iface, "Interface Number": binary.BigEndian.Uint32(v[2+size:]), "OID": bytes.Clone(v[7+size:])}
			if family == 1 || family == 2 {
				length := 4
				if family == 2 {
					length = 16
				}
				if len(addr) != length {
					return bad("management IP address length is invalid")
				}
				m["IP"] = net.IP(addr).String()
			}
			t["Management Address"] = m
		case 127:
			if n < 4 {
				return bad("organizational OUI/subtype is incomplete")
			}
			oui := uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2])
			sub := v[3]
			t["OUI"], t["Subtype"], t["Organization Data"] = oui, sub, bytes.Clone(v[4:])
			if oui == 0x000ecf {
				switch sub {
				case 2:
					if n != 8 {
						return bad("PNO Port Status length is invalid")
					}
					t["PNO Port Status"] = map[string]any{"Class 2": binary.BigEndian.Uint16(v[4:]), "Class 3": binary.BigEndian.Uint16(v[6:])}
				case 5:
					if n != 10 {
						return bad("PNO Chassis MAC length is invalid")
					}
					t["PNO Chassis MAC"] = net.HardwareAddr(v[4:]).String()
				}
			} else if oui == 0x00120f && sub == 1 {
				if n != 9 {
					return bad("IEEE802.3 MAC/PHY length is invalid")
				}
				t["IEEE802.3 MAC/PHY"] = map[string]any{"Auto Negotiation": v[4], "Advertised PMD Capabilities": binary.BigEndian.Uint16(v[5:]), "Operational MAU Type": binary.BigEndian.Uint16(v[7:])}
			}
		}
		tlvs = append(tlvs, t)
		at = end
	}
	return nil, protocolError(ErrUnsupportedFeature, "LLDP Ethernet profile requires an explicit End TLV")
}

func (a *binParser) decodeLLDPEthernet(eth *layers.Ethernet, w []byte, evidence captureEvidence, ci gopacket.CaptureInfo) {
	e := &ProtocolEvent{Timestamp: ci.Timestamp, Transport: "ethernet", Protocol: "lldp", Profile: "lldp-ended-discovery", Admission: "ether-type-and-complete-tlv-profile", Source: eth.SrcMAC.String(), Destination: eth.DstMAC.String(), Domain: evidence.Ref.Domain, Length: len(w), SourceBytes: ByteSource{Kind: "captured", PacketRefs: []PacketReference{evidence.Ref}}, Completeness: "message"}
	if ci.CaptureLength < ci.Length {
		e.Status, e.sessionError = classifySessionError(protocolError(ErrNeedMore, "LLDP capture does not contain the complete Ethernet frame"))
		e.Error, e.Completeness = e.sessionError.Error(), e.Status
		a.incomplete.Add(1)
		a.emit(e)
		return
	}
	if len(w) > min(a.budget.MaxFrameBytes, a.budget.MaxMessageBytes) {
		e.Status, e.sessionError = classifySessionError(protocolError(ErrResourceExceeded, "LLDP frame exceeds parser byte budget"))
		e.Error = e.sessionError.Error()
		e.Completeness = "limited"
		a.limited.Add(uint64(len(w)))
		a.emit(e)
		return
	}
	e.Raw = bytes.Clone(w)
	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(w)))
	fields, err := decodeLLDP(w, a.budget.MaxCollectionElements)
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
		e.Completeness = e.Status
		switch e.Status {
		case "limited":
			a.limited.Add(uint64(len(w)))
		case "incomplete":
			a.incomplete.Add(1)
		case "context-required":
			a.contextRequired.Add(1)
		default:
			a.malformed.Add(1)
		}
		a.emit(e)
		return
	}
	e.Session, e.semanticFields = fields, cloneSession(fields)
	e.Structured = map[string]any{"fields": cloneSession(fields)}
	e.Status = "decoded"
	if a.config.Deferred {
		e.Status = "deferred"
		a.deferred.Add(1)
	} else {
		a.decoded.Add(1)
	}
	a.emit(e)
}
