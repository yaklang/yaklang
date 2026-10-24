package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// RDPUDPWire is one complete selected-version carrier envelope. Normalized
// owns its bytes: MS-RDPEUDP2 swaps bytes 0 and 7 on the wire. Field offsets
// below refer to Normalized, never to a made-up contiguous capture offset.
type RDPUDPWire struct {
	Version, Flags, Window, OfferedVersion                    uint16
	SourceACK, InitialSequence, SourceSequence, CodedSequence uint32
	DataSequence, ChannelSequence                             uint16
	SYN, ACK, Data, FEC, Lossy, Dummy                         bool
	Normalized, Payload, CorrelationID, CookieHash            []byte
	Fields                                                    map[string]any
}

// DecodeRDPUDPWire shares carrier validation between canonical rules and the
// native receiver. Version selection is supplied by a captured SYN exchange;
// a two-byte flags prefix is never a wire-version or TLS identity heuristic.
func DecodeRDPUDPWire(w []byte, version uint16) (*RDPUDPWire, error) {
	if len(w) > 65507 {
		return nil, fmt.Errorf("rdpudp: UDP payload exceeds profile")
	}
	r := &RDPUDPWire{Version: version, Normalized: bytes.Clone(w), Fields: map[string]any{"Protocol": "RDPUDP", "Authentication Verified": false}}
	if version == 0x101 {
		if len(w) < 8 {
			return nil, fmt.Errorf("rdpudp2: short wrapped packet")
		}
		r.Normalized[0], r.Normalized[7] = w[7], w[0]
		return decodeRDPUDP2(r)
	}
	if version != 1 && version != 2 {
		return nil, fmt.Errorf("rdpudp: unsupported selected version")
	}
	if len(w) < 8 {
		return nil, fmt.Errorf("rdpudp: short FEC header")
	}
	r.SourceACK, r.Window, r.Flags = binary.BigEndian.Uint32(w), binary.BigEndian.Uint16(w[4:]), binary.BigEndian.Uint16(w[6:])
	r.SYN, r.ACK, r.Data, r.FEC, r.Lossy = r.Flags&1 != 0, r.Flags&4 != 0, r.Flags&8 != 0, r.Flags&16 != 0, r.Flags&0x200 != 0
	r.Fields["Protocol Version"], r.Fields["Flags"], r.Fields["Source ACK"], r.Fields["Receive Window"] = version, r.Flags, r.SourceACK, r.Window
	r.Fields["SYN"], r.Fields["ACK"], r.Fields["DATA"], r.Fields["FEC"], r.Fields["Lossy"] = r.SYN, r.ACK, r.Data, r.FEC, r.Lossy
	at := 8
	take := func(n int) ([]byte, error) {
		if n < 0 || n > len(w)-at {
			return nil, fmt.Errorf("rdpudp: optional field exceeds datagram")
		}
		v := w[at : at+n]
		at += n
		return v, nil
	}
	if r.Flags&^uint16(0x1f7f) != 0 {
		return nil, fmt.Errorf("rdpudp: unselected reserved flags")
	}
	if r.SYN {
		if r.Data || r.FEC || r.Flags&0x102 != 0 {
			return nil, fmt.Errorf("rdpudp: SYN combines FIN/data/FEC/AOA")
		}
		v, err := take(8)
		if err != nil {
			return nil, err
		}
		r.InitialSequence = binary.BigEndian.Uint32(v)
		up, down := binary.BigEndian.Uint16(v[4:]), binary.BigEndian.Uint16(v[6:])
		if r.Window == 0 || up < 1132 || up > 1232 || down < 1132 || down > 1232 {
			return nil, fmt.Errorf("rdpudp: SYN receive window/MTU outside MS-RDPEUDP profile")
		}
		r.Fields["Initial Sequence"], r.Fields["Upstream MTU"], r.Fields["Downstream MTU"] = r.InitialSequence, up, down
		if len(w) != int(min(up, down)) {
			return nil, fmt.Errorf("rdpudp: SYN is not padded to the selected minimum MTU")
		}
		if !r.ACK && r.SourceACK != 0xffffffff {
			return nil, fmt.Errorf("rdpudp: initial SYN ACK is not -1")
		}
	}
	if r.Flags&0x800 != 0 {
		if !r.SYN {
			return nil, fmt.Errorf("rdpudp: correlation ID outside SYN")
		}
		v, err := take(32)
		if err != nil {
			return nil, err
		}
		r.CorrelationID = bytes.Clone(v[:16])
		r.Fields["Correlation ID"] = bytes.Clone(v[:16])
		if !bytes.Equal(v[16:], make([]byte, 16)) {
			return nil, fmt.Errorf("rdpudp: correlation reserved bytes are nonzero")
		}
	}
	r.OfferedVersion = 1
	if r.Flags&0x1000 != 0 {
		if !r.SYN {
			return nil, fmt.Errorf("rdpudp: SYNEX outside SYN")
		}
		v, err := take(2)
		if err != nil {
			return nil, err
		}
		flags := binary.BigEndian.Uint16(v)
		r.Fields["SYNEX Flags"] = flags
		if flags&^uint16(1) != 0 {
			return nil, fmt.Errorf("rdpudp: unselected SYNEX flags")
		}
		if flags&1 != 0 {
			v, err = take(2)
			if err != nil {
				return nil, err
			}
			r.OfferedVersion = binary.BigEndian.Uint16(v)
			if r.OfferedVersion != 1 && r.OfferedVersion != 2 && r.OfferedVersion != 0x101 {
				return nil, fmt.Errorf("rdpudp: unselected SYNEX version")
			}
			if r.OfferedVersion == 0x101 {
				v, err = take(32)
				if err != nil {
					return nil, err
				}
				r.CookieHash = bytes.Clone(v)
				r.Fields["Cookie Hash"] = bytes.Clone(v)
			}
		}
		r.Fields["Offered Version"] = r.OfferedVersion
	}
	if r.ACK && !r.SYN {
		v, err := take(2)
		if err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint16(v))
		if n > 2048 {
			return nil, fmt.Errorf("rdpudp: ACK vector exceeds 2048 entries")
		}
		v, err = take(n)
		if err != nil {
			return nil, err
		}
		r.Fields["ACK Vector"] = bytes.Clone(v)
	}
	if r.FEC {
		v, err := take(10)
		if err != nil {
			return nil, err
		}
		r.Fields["FEC Coded Sequence"], r.Fields["FEC Source Start"], r.Fields["FEC Range"], r.Fields["FEC Index"] = binary.BigEndian.Uint32(v), binary.BigEndian.Uint32(v[4:]), v[8], v[9]
	}
	if r.Flags&0x100 != 0 {
		v, err := take(4)
		if err != nil {
			return nil, err
		}
		r.Fields["ACK of ACKs"] = binary.BigEndian.Uint32(v)
	}
	if r.Data && !r.FEC {
		v, err := take(8)
		if err != nil {
			return nil, err
		}
		r.CodedSequence, r.SourceSequence = binary.BigEndian.Uint32(v), binary.BigEndian.Uint32(v[4:])
		r.Fields["Coded Sequence"], r.Fields["Source Sequence"] = r.CodedSequence, r.SourceSequence
	}
	if r.SYN {
		if !bytes.Equal(w[at:], make([]byte, len(w)-at)) {
			return nil, fmt.Errorf("rdpudp: nonzero SYN padding")
		}
		r.Fields["Padding Bytes"] = len(w) - at
	} else if r.Data || r.FEC {
		r.Payload = r.Normalized[at:]
	} else if at != len(w) {
		return nil, fmt.Errorf("rdpudp: unexpected control suffix")
	}
	r.Fields["Header Bytes"], r.Fields["Payload Bytes"] = at, len(r.Payload)
	return r, nil
}

func decodeRDPUDP2(r *RDPUDPWire) (*RDPUDPWire, error) {
	w := r.Normalized
	typ, short := (w[0]>>1)&15, int(w[0]>>5)
	if w[0]&1 != 0 || typ != 0 && typ != 8 {
		return nil, fmt.Errorf("rdpudp2: unselected packet prefix/type")
	}
	if short < 7 {
		end := 1 + short
		if end < 3 || len(w) != 8 || !bytes.Equal(w[end:], make([]byte, len(w)-end)) {
			return nil, fmt.Errorf("rdpudp2: invalid short packet length/padding")
		}
		w = w[:end]
	}
	header := binary.LittleEndian.Uint16(w[1:3])
	r.Flags, r.Dummy = header&0xfff, typ == 8
	logWindow := header >> 12
	if r.Flags&^uint16(0x35d) != 0 || logWindow == 0 {
		return nil, fmt.Errorf("rdpudp2: unselected flags/window")
	}
	r.Data, r.ACK = r.Flags&4 != 0, r.Flags&1 != 0
	r.Fields["Protocol Version"], r.Fields["Flags"], r.Fields["Log Window"], r.Fields["Packet Type"], r.Fields["Short Packet Length"] = uint16(0x101), r.Flags, logWindow, typ, short
	r.Fields["DATA"], r.Fields["ACK"], r.Fields["Dummy"] = r.Data, r.ACK, r.Dummy
	at := 3
	take := func(n int) ([]byte, error) {
		if n < 0 || n > len(w)-at {
			return nil, fmt.Errorf("rdpudp2: optional field exceeds packet layout")
		}
		v := w[at : at+n]
		at += n
		return v, nil
	}
	if r.ACK {
		v, err := take(7)
		if err != nil {
			return nil, err
		}
		n := int(v[6] & 15)
		r.Fields["ACK Base Sequence"], r.Fields["ACK Timestamp"], r.Fields["ACK Send Time Gap"], r.Fields["Delayed ACK Count"] = binary.LittleEndian.Uint16(v), uint32(v[2])|uint32(v[3])<<8|uint32(v[4])<<16, v[5], n
		v, err = take(n)
		if err != nil {
			return nil, err
		}
		r.Fields["Delayed ACKs"] = bytes.Clone(v)
	}
	if r.Flags&0x40 != 0 {
		v, err := take(1)
		if err != nil {
			return nil, err
		}
		r.Fields["Overhead Size"] = v[0]
	}
	if r.Flags&0x100 != 0 {
		v, err := take(3)
		if err != nil {
			return nil, err
		}
		r.Fields["Maximum Delayed ACKs"], r.Fields["Delayed ACK Timeout"] = v[0], binary.LittleEndian.Uint16(v[1:])
	}
	if r.Flags&0x10 != 0 {
		v, err := take(2)
		if err != nil {
			return nil, err
		}
		r.Fields["ACK of ACKs"] = binary.LittleEndian.Uint16(v)
	}
	if r.Data {
		v, err := take(2)
		if err != nil {
			return nil, err
		}
		r.DataSequence = binary.LittleEndian.Uint16(v)
		r.Fields["Data Sequence"] = r.DataSequence
	}
	if r.Flags&8 != 0 {
		v, err := take(3)
		if err != nil {
			return nil, err
		}
		n := int(v[2] & 127)
		r.Fields["ACK Vector Base Sequence"] = binary.LittleEndian.Uint16(v)
		if v[2]&128 != 0 {
			v, err = take(4)
			if err != nil {
				return nil, err
			}
			r.Fields["ACK Vector Timestamp"] = uint32(v[0]) | uint32(v[1])<<8 | uint32(v[2])<<16
			r.Fields["ACK Vector Send Time Gap"] = v[3]
		}
		v, err = take(n)
		if err != nil {
			return nil, err
		}
		r.Fields["ACK Vector"] = bytes.Clone(v)
	}
	if r.Data && !r.Dummy {
		v, err := take(2)
		if err != nil {
			return nil, err
		}
		r.ChannelSequence = binary.LittleEndian.Uint16(v)
		r.Fields["Channel Sequence"] = r.ChannelSequence
		r.Payload = r.Normalized[at:len(w)]
	} else if r.Dummy {
		r.Fields["Dummy Padding Bytes"] = len(w) - at
	} else if at != len(w) {
		return nil, fmt.Errorf("rdpudp2: unexpected control suffix")
	}
	r.Fields["Header Bytes"], r.Fields["Payload Bytes"] = at, len(r.Payload)
	return r, nil
}
