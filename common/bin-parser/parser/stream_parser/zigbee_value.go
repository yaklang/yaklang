package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// Wire layouts: CSA 05-3474-23, sections 2.2.5, 3.3, 3.4, 3.6.8,
// 4.4.11 and 4.5.1. IEEE 802.15.4-2003/2006 MAC layout is independently
// cross-checked against Scapy 6f158c0c0ca4c1b6f1c8a0e9aa4b0905e0086b91,
// layers/dot15d4.py. No decryption, replay state or application schema is used.
// Spans are byte offsets in the ORIGINAL complete capture record.
type zigbeeField struct {
	Name, Type string
	Start, End int
	Children   []zigbeeField
	List       bool
}

type zigbeeDecoder struct {
	wire []byte
	pos  int
	err  error
	info map[string]any
}

func (d *zigbeeDecoder) field(dst *[]zigbeeField, name, typ string, size int) []byte {
	if d.err != nil {
		return nil
	}
	if size < 0 || size > len(d.wire)-d.pos {
		d.err = fmt.Errorf("zigbee: truncated %s at byte %d", name, d.pos)
		return nil
	}
	start := d.pos
	d.pos += size
	*dst = append(*dst, zigbeeField{Name: name, Type: typ, Start: start, End: d.pos})
	return d.wire[start:d.pos]
}

func (d *zigbeeDecoder) u8(dst *[]zigbeeField, name string) byte {
	b := d.field(dst, name, "uint8", 1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (d *zigbeeDecoder) u16(dst *[]zigbeeField, name string) uint16 {
	b := d.field(dst, name, "uint16", 2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (d *zigbeeDecoder) raw(dst *[]zigbeeField, name string) {
	d.field(dst, name, "raw", len(d.wire)-d.pos)
}

func (d *zigbeeDecoder) group(dst *[]zigbeeField, name string, list bool, run func(*[]zigbeeField)) {
	if d.err != nil {
		return
	}
	f := zigbeeField{Name: name, Start: d.pos, List: list}
	run(&f.Children)
	f.End = d.pos
	*dst = append(*dst, f)
}

func (d *zigbeeDecoder) fail(reason string) {
	if d.err == nil {
		d.err = fmt.Errorf("zigbee: %s", reason)
	}
}
func (d *zigbeeDecoder) end() {
	if d.pos != len(d.wire) {
		d.fail("unexpected trailing command bytes")
	}
}

func zigbeeCRC(wire []byte) uint16 {
	var crc uint16
	for _, b := range wire {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0x8408
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func decodeZigbeeFrame(wire []byte, hasFCS bool) ([]zigbeeField, map[string]any, error) {
	maximum, minimum := 125, 3
	if hasFCS {
		maximum, minimum = 127, 5
	}
	if len(wire) < minimum || len(wire) > maximum {
		return nil, nil, fmt.Errorf("zigbee: frame boundary must be %d..%d bytes", minimum, maximum)
	}
	end := len(wire)
	if hasFCS {
		end -= 2
		if zigbeeCRC(wire[:end]) != binary.LittleEndian.Uint16(wire[end:]) {
			return nil, nil, fmt.Errorf("zigbee: incorrect frame check sequence")
		}
	}
	d := &zigbeeDecoder{wire: wire[:end], info: map[string]any{
		"Profile": "Zigbee bounded MAC Beacon NWK APS fields", "FCS Present": hasFCS, "FCS Validated": hasFCS,
		"Payload Decrypted": false, "Payload Authenticated": false, "Application Semantics Decoded": false,
		"Fragment Reassembly Performed": false, "Protection Level Inferred": false, "Uninterpreted MAC Tail": false,
		"Beacon Present": false, "NWK Present": false, "APS Present": false,
	}}
	var fields []zigbeeField
	fc := d.u16(&fields, "MAC Frame Control")
	kind, version, dest, src := fc&7, (fc>>12)&3, (fc>>10)&3, (fc>>14)&3
	d.info["MAC Frame Type"] = uint8(kind)
	if kind > 3 || version > 1 || dest == 1 || src == 1 || fc&0x0380 != 0 {
		d.fail("unsupported MAC frame version, type or reserved flags")
	}
	if fc&8 != 0 {
		d.fail("MAC auxiliary protection profile is not supported")
	}
	if fc&0x40 != 0 && (src == 0 || dest == 0) {
		d.fail("MAC PAN compression requires both addresses")
	}
	d.u8(&fields, "MAC Sequence Number")
	if dest != 0 {
		d.u16(&fields, "MAC Destination PAN")
		if dest == 2 {
			d.u16(&fields, "MAC Destination Short")
		} else {
			d.field(&fields, "MAC Destination Extended", "raw", 8)
		}
	}
	if src != 0 {
		if fc&0x40 == 0 {
			d.u16(&fields, "MAC Source PAN")
		}
		if src == 2 {
			d.u16(&fields, "MAC Source Short")
		} else {
			d.field(&fields, "MAC Source Extended", "raw", 8)
		}
	}
	switch kind {
	case 0:
		if dest != 0 || src == 0 {
			d.fail("invalid MAC beacon addressing")
		}
		d.group(&fields, "MAC Beacon", false, d.macBeacon)
		d.group(&fields, "Zigbee Beacon", false, d.beacon)
	case 1:
		d.group(&fields, "Zigbee NWK", false, d.nwk)
	case 2:
		if dest != 0 || src != 0 || fc&0x68 != 0 {
			d.fail("invalid legacy MAC acknowledgment flags")
		}
		// Some original no-FCS records contain two extra bytes. They are neither
		// silently stripped nor advertised as a valid FCS or Zigbee payload.
		if d.pos < end {
			d.info["Uninterpreted MAC Tail"] = true
			d.raw(&fields, "Uninterpreted MAC Acknowledgment Tail")
		}
	case 3:
		d.group(&fields, "MAC Command", false, func(f *[]zigbeeField) { d.macCommand(f, version) })
	}
	if d.err != nil {
		return nil, nil, d.err
	}
	d.end()
	if d.err != nil {
		return nil, nil, d.err
	}
	if hasFCS {
		fields = append(fields, zigbeeField{Name: "Frame Check Sequence", Type: "uint16", Start: end, End: end + 2})
	}
	return fields, d.info, nil
}

func (d *zigbeeDecoder) macBeacon(f *[]zigbeeField) {
	d.u16(f, "Superframe Specification")
	gts := d.u8(f, "GTS Specification")
	if gts&0x78 != 0 {
		d.fail("reserved GTS specification flags")
	}
	if gts&7 != 0 {
		d.u8(f, "GTS Directions")
		d.group(f, "GTS Descriptors", true, func(list *[]zigbeeField) {
			for i := byte(0); i < gts&7; i++ {
				d.group(list, "GTS Descriptor", false, func(e *[]zigbeeField) { d.u16(e, "GTS Address"); d.u8(e, "GTS Slot and Length") })
			}
		})
	}
	pending := d.u8(f, "Pending Address Specification")
	if pending&0x88 != 0 {
		d.fail("reserved pending address flags")
	}
	if pending&7 != 0 {
		d.group(f, "Pending Short Addresses", true, func(list *[]zigbeeField) {
			for i := byte(0); i < pending&7; i++ {
				d.u16(list, "Pending Short Address")
			}
		})
	}
	if pending&0x70 != 0 {
		d.group(f, "Pending Extended Addresses", true, func(list *[]zigbeeField) {
			for i := byte(0); i < (pending>>4)&7; i++ {
				d.field(list, "Pending Extended Address", "raw", 8)
			}
		})
	}
}

func (d *zigbeeDecoder) beacon(f *[]zigbeeField) {
	d.info["Beacon Present"] = true
	if d.u8(f, "Beacon Protocol ID") != 0 {
		d.fail("unsupported beacon protocol identifier")
	}
	profile := d.u8(f, "Beacon Stack Profile and Version")
	if profile>>4 != 2 {
		d.fail("unsupported beacon protocol version")
	}
	capacity := d.u8(f, "Beacon Capacity and Depth")
	if capacity&3 != 0 {
		d.fail("reserved beacon capacity flags")
	}
	d.field(f, "Beacon Extended PAN ID", "raw", 8)
	// uint32 with a 24-bit wire span keeps the little-endian numeric value.
	d.field(f, "Beacon Transmit Offset", "uint32", 3)
	d.u8(f, "Beacon Update ID")
	if d.pos < len(d.wire) {
		d.raw(f, "Uninterpreted Beacon Appendix")
		d.info["Beacon Appendix Decoded"] = false
	}
}

func (d *zigbeeDecoder) macCommand(f *[]zigbeeField, version uint16) {
	id := d.u8(f, "MAC Command ID")
	switch id {
	case 1:
		d.u8(f, "MAC Capability Information")
	case 2:
		d.u16(f, "Assigned Short Address")
		d.u8(f, "Association Status")
	case 3:
		d.u8(f, "Disassociation Reason")
	case 4, 5, 6, 7:
	case 8:
		d.u16(f, "Realignment PAN")
		d.u16(f, "Coordinator Short Address")
		d.u8(f, "Logical Channel")
		d.u16(f, "Device Short Address")
		if version == 1 && d.pos < len(d.wire) {
			d.u8(f, "Channel Page")
		}
	case 9:
		d.u8(f, "GTS Characteristics")
	default:
		d.raw(f, "Uninterpreted MAC Command Data")
		d.info["MAC Command Decoded"] = false
		return
	}
	d.info["MAC Command Decoded"] = true
	d.end()
}

func (d *zigbeeDecoder) nwk(f *[]zigbeeField) {
	d.info["NWK Present"] = true
	fc := d.u16(f, "NWK Frame Control")
	typ := (fc & 3)
	if (fc>>2)&15 != 2 || typ > 1 || fc&0xc100 != 0 || (fc>>6)&3 > 1 || (typ == 1 && fc&0xc0 != 0) {
		d.fail("unsupported NWK version, type, multicast or reserved flags")
	}
	d.u16(f, "NWK Destination")
	d.u16(f, "NWK Source")
	d.u8(f, "NWK Radius")
	d.u8(f, "NWK Sequence Number")
	if fc&0x0800 != 0 {
		d.field(f, "NWK Destination Extended", "raw", 8)
	}
	if fc&0x1000 != 0 {
		d.field(f, "NWK Source Extended", "raw", 8)
	}
	if fc&0x0400 != 0 {
		count := d.u8(f, "Relay Count")
		index := d.u8(f, "Relay Index")
		if count == 0 || (index >= count && index != 255) {
			d.fail("invalid source route count or index")
		}
		d.group(f, "Relays", true, func(list *[]zigbeeField) {
			for i := byte(0); i < count; i++ {
				d.u16(list, "Relay Address")
			}
		})
	}
	if fc&0x0200 != 0 {
		d.group(f, "NWK Auxiliary Header", false, d.protected)
		return
	}
	if typ == 0 {
		d.group(f, "Zigbee APS", false, d.aps)
	} else {
		d.group(f, "NWK Command", false, d.nwkCommand)
	}
}

func (d *zigbeeDecoder) protected(f *[]zigbeeField) {
	control := d.u8(f, "Auxiliary Control")
	if control&0x80 != 0 {
		d.fail("reserved auxiliary control flag")
	}
	d.field(f, "Auxiliary Frame Counter", "uint32", 4)
	if control&0x20 != 0 {
		d.field(f, "Auxiliary Source Extended", "raw", 8)
	}
	if (control>>3)&3 == 1 {
		d.u8(f, "Auxiliary Key Sequence Number")
	}
	// Zero on the wire does not establish the network's configured protection
	// level. Keep payload+MIC together instead of inventing a MIC boundary.
	level := control & 7
	if level == 0 {
		d.raw(f, "Protected Payload and MIC")
		return
	}
	mic := []int{0, 4, 8, 16, 0, 4, 8, 16}[level]
	d.field(f, "Protected Payload", "raw", len(d.wire)-d.pos-mic)
	if mic != 0 {
		d.field(f, "Message Integrity Code", "raw", mic)
	}
}

func (d *zigbeeDecoder) nwkCommand(f *[]zigbeeField) {
	id := d.u8(f, "NWK Command ID")
	switch id {
	case 1:
		options := d.u8(f, "Route Request Options")
		d.u8(f, "Route Request ID")
		d.u16(f, "Route Destination")
		d.u8(f, "Path Cost")
		if options&0x20 != 0 {
			d.field(f, "Route Destination Extended", "raw", 8)
		}
	case 2:
		options := d.u8(f, "Route Reply Options")
		d.u8(f, "Route Request ID")
		d.u16(f, "Route Originator")
		d.u16(f, "Route Responder")
		d.u8(f, "Path Cost")
		if options&0x10 != 0 {
			d.field(f, "Route Originator Extended", "raw", 8)
		}
		if options&0x20 != 0 {
			d.field(f, "Route Responder Extended", "raw", 8)
		}
	case 3:
		d.u8(f, "Network Status")
		d.u16(f, "Status Destination")
	case 4:
		d.u8(f, "Leave Options")
	case 5:
		count := d.u8(f, "Route Record Count")
		d.group(f, "Route Record Addresses", true, func(list *[]zigbeeField) {
			for i := byte(0); i < count; i++ {
				d.u16(list, "Route Record Address")
			}
		})
	case 6:
		d.u8(f, "Rejoin Capability")
	case 7:
		d.u16(f, "Rejoin Address")
		d.u8(f, "Rejoin Status")
	case 8:
		options := d.u8(f, "Link Status Options")
		d.group(f, "Link Status Entries", true, func(list *[]zigbeeField) {
			for i := byte(0); i < options&31; i++ {
				d.group(list, "Link Status Entry", false, func(e *[]zigbeeField) { d.u16(e, "Neighbor Address"); d.u8(e, "Link Costs") })
			}
		})
	case 11:
		d.u8(f, "Requested Timeout")
		d.u8(f, "End Device Configuration")
	case 12:
		d.u8(f, "Timeout Status")
		d.u8(f, "Parent Information")
	default:
		d.raw(f, "Uninterpreted NWK Command Data")
		d.info["NWK Command Decoded"] = false
		return
	}
	d.info["NWK Command Decoded"] = true
	d.end()
}

func (d *zigbeeDecoder) aps(f *[]zigbeeField) {
	d.info["APS Present"] = true
	fc := d.u8(f, "APS Frame Control")
	typ, mode := fc&3, (fc>>2)&3
	if typ == 3 || mode == 1 || (typ != 2 && fc&0x10 != 0) || (typ == 1 && (mode == 3 || fc&0x80 != 0)) || (typ == 2 && (mode != 0 || fc&0x40 != 0)) || (mode >= 2 && fc&0x40 != 0) {
		d.fail("unsupported APS type, delivery mode or flags")
	}
	if typ == 0 || (typ == 2 && fc&0x10 == 0) {
		if mode == 3 {
			d.u16(f, "APS Group Address")
		} else {
			d.u8(f, "APS Destination Endpoint")
		}
		d.u16(f, "APS Cluster ID")
		d.u16(f, "APS Profile ID")
		d.u8(f, "APS Source Endpoint")
	}
	d.u8(f, "APS Counter")
	fragment := byte(0)
	if fc&0x80 != 0 {
		control := d.u8(f, "APS Extended Frame Control")
		fragment = control & 3
		if control&0xfc != 0 || fragment == 3 {
			d.fail("reserved APS fragmentation control")
		}
		if fragment != 0 {
			d.u8(f, "APS Block Number")
			if typ == 2 {
				d.u8(f, "APS Acknowledgment Bitfield")
			}
		}
	}
	if fc&0x20 != 0 {
		d.group(f, "APS Auxiliary Header", false, d.protected)
		return
	}
	if fragment != 0 {
		d.info["APS Fragment Present"] = true
		if typ == 2 {
			d.end()
		} else {
			d.raw(f, "Unreassembled APS Fragment")
		}
		return
	}
	switch typ {
	case 0:
		d.raw(f, "APS Application Data")
	case 1:
		d.group(f, "APS Command", false, d.apsCommand)
	case 2:
		d.end()
	}
}

func (d *zigbeeDecoder) apsCommand(f *[]zigbeeField) {
	id := d.u8(f, "APS Command ID")
	switch id {
	case 1, 2, 3, 4:
		d.field(f, "Command Initiator", "raw", 8)
		d.field(f, "Command Responder", "raw", 8)
		d.field(f, "Command Material", "raw", 16)
	case 5:
		kind := d.u8(f, "Transport Key Type")
		if kind > 5 {
			d.raw(f, "Uninterpreted Transport Key Data")
			d.info["APS Command Decoded"] = false
			return
		}
		d.field(f, "Transport Key Material", "raw", 16)
		if kind == 1 || kind == 5 {
			d.u8(f, "Transport Key Sequence Number")
		}
		if kind == 2 || kind == 3 {
			d.field(f, "Transport Partner Extended", "raw", 8)
			flag := d.u8(f, "Transport Initiator Flag")
			if flag > 1 {
				d.fail("invalid transport initiator flag")
			}
		} else {
			d.field(f, "Transport Destination Extended", "raw", 8)
			d.field(f, "Transport Source Extended", "raw", 8)
		}
	case 6:
		d.field(f, "Updated Device Extended", "raw", 8)
		d.u16(f, "Updated Device Short")
		d.u8(f, "Device Update Status")
	case 7:
		d.field(f, "Removed Device Extended", "raw", 8)
	case 8:
		kind := d.u8(f, "Requested Key Type")
		if kind == 2 {
			d.field(f, "Requested Partner Extended", "raw", 8)
		} else if kind != 4 {
			d.fail("unsupported requested key type")
		}
	case 9:
		d.u8(f, "Switch Key Sequence Number")
	case 15:
		d.u8(f, "Verified Key Type")
		d.field(f, "Verified Source Extended", "raw", 8)
		d.field(f, "Verification Material", "raw", 16)
	case 16:
		d.u8(f, "Confirmation Status")
		d.u8(f, "Confirmed Key Type")
		d.field(f, "Confirmed Destination Extended", "raw", 8)
	default:
		d.raw(f, "Uninterpreted APS Command Data")
		d.info["APS Command Decoded"] = false
		return
	}
	d.info["APS Command Decoded"] = true
	// R23 adds optional TLVs to several older commands. Do not reinterpret them
	// as application data or silently discard them in this legacy fixed profile.
	if d.pos < len(d.wire) {
		d.fail("unsupported APS command appendix or trailing bytes")
	}
}
