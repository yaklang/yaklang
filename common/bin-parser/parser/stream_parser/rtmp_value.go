package stream_parser

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"
)

// RTMPChunkContext belongs to one direction and one chunk stream, never a port.
// Received counts only the observed payload of its current logical message.
type RTMPChunkContext struct {
	Known                                             bool
	Timestamp, Delta, Length, StreamID, ExtendedValue uint32
	Type                                              byte
	Extended                                          bool
	Received                                          int
}
type RTMPChunkHeader struct {
	Context                                 RTMPChunkContext
	Format                                  byte
	CSID                                    uint32
	HeaderLength, PayloadLength, WireLength int
	Continuation                            bool
}

func rtmp24(w []byte) uint32 { return uint32(w[0])<<16 | uint32(w[1])<<8 | uint32(w[2]) }

// DecodeRTMPChunkHeader does not allocate the advertised message. A zero wire
// length means an incomplete header; a positive length may exceed len(w).
func DecodeRTMPChunkHeader(w []byte, prev RTMPChunkContext, chunk uint32, limit int) (h RTMPChunkHeader, err error) {
	if len(w) == 0 {
		return
	}
	h.Format = w[0] >> 6
	h.CSID = uint32(w[0] & 63)
	off := 1
	if h.CSID == 0 {
		if len(w) < 2 {
			return
		}
		h.CSID = 64 + uint32(w[1])
		off = 2
	} else if h.CSID == 1 {
		if len(w) < 3 {
			return
		}
		h.CSID = 64 + uint32(w[1]) + uint32(w[2])*256
		off = 3
	}
	if h.Format != 0 && !prev.Known {
		return h, fmt.Errorf("%w: RTMP compressed header before observed CSID header", ErrObservedWireContext)
	}
	n := []int{11, 7, 3, 0}[h.Format]
	if len(w) < off+n {
		return
	}
	c := prev
	h.Continuation = prev.Known && prev.Received < int(prev.Length)
	var ts uint32
	if h.Format < 3 {
		ts = rtmp24(w[off:])
		c.Extended = ts == 0xffffff
	}
	if h.Format < 2 {
		c.Length = rtmp24(w[off+3:])
		c.Type = w[off+6]
	}
	if h.Format == 0 {
		c.StreamID = binary.LittleEndian.Uint32(w[off+7:])
	}
	off += n
	if c.Extended {
		if len(w) < off+4 {
			return h, nil
		}
		ts = binary.BigEndian.Uint32(w[off:])
		off += 4
		if h.Format == 3 && ts != prev.ExtendedValue {
			return h, fmt.Errorf("RTMP continuation extended timestamp disagrees with inherited header")
		}
	}
	if h.Format == 3 {
		c = prev
		if !h.Continuation {
			c.Timestamp += c.Delta
			c.Received = 0
		}
	} else {
		if h.Continuation {
			if c.Length != prev.Length || c.Type != prev.Type || c.StreamID != prev.StreamID || (h.Format == 0 && ts != prev.Timestamp) || (h.Format != 0 && ts != prev.Delta) {
				return h, fmt.Errorf("RTMP header replaces an unfinished message without Abort")
			}
			c = prev
		} else {
			c.Received = 0
			c.ExtendedValue = ts
			if h.Format == 0 {
				c.Timestamp = ts
				c.Delta = ts
			} else {
				c.Timestamp = prev.Timestamp + ts
				c.Delta = ts
			}
		}
	}
	if chunk == 0 || chunk > 0x7fffffff {
		return h, fmt.Errorf("RTMP invalid directional chunk size")
	}
	if int(c.Length) > limit {
		return h, fmt.Errorf("%w: RTMP message length exceeds receiver budget", ErrObservedWireBudget)
	}
	c.Known = true
	h.Context = c
	h.HeaderLength = off
	h.PayloadLength = min(int(chunk), int(c.Length)-c.Received)
	if h.PayloadLength < 0 {
		return h, fmt.Errorf("RTMP received byte count exceeds message length")
	}
	h.WireLength = off + h.PayloadLength
	return
}

var rtmpDigestTail = []byte{0xf0, 0xee, 0xc2, 0x4a, 0x80, 0x68, 0xbe, 0xe8, 0x2e, 0x00, 0xd0, 0xd1, 0x02, 0x9e, 0x7e, 0x57, 0x6e, 0xec, 0x5d, 0x2d, 0x29, 0x80, 0x6f, 0xab, 0x93, 0xb8, 0xe6, 0x36, 0xcf, 0xeb, 0x31, 0xae}
var rtmpKeys = []string{"Genuine Adobe Flash Player 001", "Genuine Adobe Flash Media Server 001"}

func rtmpHMAC(key, w []byte) []byte { h := hmac.New(sha256.New, key); h.Write(w); return h.Sum(nil) }

// RTMPInitialHandshake validates the complete version-3 initial block. Complex
// digests use public Adobe constants: this is structure, not authentication.
func RTMPInitialHandshake(w []byte) (map[string]any, error) {
	if len(w) != 1537 || w[0] != 3 {
		return nil, fmt.Errorf("RTMP initial handshake requires version 3 and 1536 complete bytes")
	}
	b := w[1:]
	f := map[string]any{"Packet Name": "Initial Handshake", "Version": uint8(3), "Time": binary.BigEndian.Uint32(b), "Version Word": binary.BigEndian.Uint32(b[4:]), "Wire Field Ranges": map[string]any{"Version": []any{0, 1}, "Time": []any{1, 5}, "Version Word": []any{5, 9}}}
	if binary.BigEndian.Uint32(b[4:]) == 0 {
		f["Handshake Mode"] = "simple"
		return f, nil
	}
	for k, key := range rtmpKeys {
		for _, base := range []int{8, 772} {
			o := base + 4 + (int(b[base])+int(b[base+1])+int(b[base+2])+int(b[base+3]))%728
			mac := hmac.New(sha256.New, []byte(key))
			mac.Write(b[:o])
			mac.Write(b[o+32:])
			if hmac.Equal(mac.Sum(nil), b[o:o+32]) {
				f["Handshake Mode"] = "public digest"
				f["Digest Role"] = []string{"player", "server"}[k]
				f["Digest"] = append([]byte(nil), b[o:o+32]...)
				f["Digest Offset"] = o
				return f, nil
			}
		}
	}
	return nil, fmt.Errorf("RTMP nonzero version word without a valid public handshake digest")
}
func RTMPEchoHandshake(w, peer []byte, peerFields map[string]any) (map[string]any, error) {
	if len(w) != 1536 {
		return nil, fmt.Errorf("RTMP echo handshake requires 1536 bytes")
	}
	f := map[string]any{"Packet Name": "Echo Handshake", "Time": binary.BigEndian.Uint32(w), "Time2": binary.BigEndian.Uint32(w[4:]), "Echo Verification": "peer initial handshake unavailable"}
	if len(peer) != 1537 {
		return f, nil
	}
	if peerFields["Handshake Mode"] == "simple" {
		if !bytes.Equal(w[:4], peer[1:5]) || !bytes.Equal(w[8:], peer[9:]) {
			return f, fmt.Errorf("RTMP simple echo differs from observed peer handshake")
		}
		f["Echo Verification"] = "matches observed peer time and random bytes"
		return f, nil
	}
	digest, ok := peerFields["Digest"].([]byte)
	if !ok {
		return f, nil
	}
	role, _ := peerFields["Digest Role"].(string)
	key := rtmpKeys[1]
	if role == "server" {
		key = rtmpKeys[0]
	}
	full := append(append([]byte(nil), []byte(key)...), rtmpDigestTail...)
	if !hmac.Equal(rtmpHMAC(rtmpHMAC(full, digest), w[:1504]), w[1504:]) {
		return f, fmt.Errorf("RTMP public digest echo differs from observed peer challenge")
	}
	f["Echo Verification"] = "public digest matches observed peer challenge"
	return f, nil
}

type amf0Decoder struct {
	w                     []byte
	off, nodes, max, refs int
}

func (a *amf0Decoder) take(n int) ([]byte, error) {
	if n < 0 || n > len(a.w)-a.off {
		return nil, fmt.Errorf("AMF0 truncated value")
	}
	b := a.w[a.off : a.off+n]
	a.off += n
	return b, nil
}
func (a *amf0Decoder) str(n int) (string, error) {
	b, e := a.take(n)
	if e != nil {
		return "", e
	}
	if !utf8.Valid(b) {
		return "", fmt.Errorf("AMF0 invalid UTF-8 string")
	}
	return string(b), nil
}
func (a *amf0Decoder) short() (string, error) {
	b, e := a.take(2)
	if e != nil {
		return "", e
	}
	return a.str(int(binary.BigEndian.Uint16(b)))
}
func (a *amf0Decoder) object(depth int) ([]any, error) {
	var out []any
	for {
		if len(a.w)-a.off >= 3 && bytes.Equal(a.w[a.off:a.off+3], []byte{0, 0, 9}) {
			a.off += 3
			return out, nil
		}
		name, e := a.short()
		if e != nil {
			return nil, e
		}
		v, e := a.value(depth + 1)
		if e != nil {
			return nil, e
		}
		out = append(out, map[string]any{"Name": name, "Value": v})
	}
}
func (a *amf0Decoder) value(depth int) (any, error) {
	a.nodes++
	if a.nodes > a.max || depth > 32 {
		return nil, fmt.Errorf("%w: AMF0 collection/depth budget", ErrObservedWireBudget)
	}
	b, e := a.take(1)
	if e != nil {
		return nil, e
	}
	t := b[0]
	f := map[string]any{"Type": t}
	switch t {
	case 0:
		b, e = a.take(8)
		if e != nil {
			return nil, e
		}
		v := math.Float64frombits(binary.BigEndian.Uint64(b))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			f["Number Bits"] = binary.BigEndian.Uint64(b)
		} else {
			f["Value"] = v
		}
	case 1:
		b, e = a.take(1)
		if e != nil {
			return nil, e
		}
		if b[0] > 1 {
			return nil, fmt.Errorf("AMF0 boolean outside 0/1")
		}
		f["Value"] = b[0] == 1
	case 2:
		f["Value"], e = a.short()
	case 3:
		a.refs++
		f["Properties"], e = a.object(depth)
	case 5, 6, 13: // null, undefined and unsupported are actual AMF0 markers.
	case 7:
		b, e = a.take(2)
		if e != nil {
			return nil, e
		}
		v := int(binary.BigEndian.Uint16(b))
		if v >= a.refs {
			return nil, fmt.Errorf("AMF0 reference precedes its object")
		}
		f["Reference"] = v
	case 8:
		b, e = a.take(4)
		if e != nil {
			return nil, e
		}
		a.refs++
		f["Declared Count"] = binary.BigEndian.Uint32(b)
		f["Properties"], e = a.object(depth)
	case 10:
		b, e = a.take(4)
		if e != nil {
			return nil, e
		}
		n := binary.BigEndian.Uint32(b)
		if n > uint32(a.max-a.nodes) {
			return nil, fmt.Errorf("%w: AMF0 array budget", ErrObservedWireBudget)
		}
		a.refs++
		var values []any
		for i := uint32(0); i < n; i++ {
			v, er := a.value(depth + 1)
			if er != nil {
				return nil, er
			}
			values = append(values, v)
		}
		f["Values"] = values
	case 11:
		b, e = a.take(10)
		if e != nil {
			return nil, e
		}
		a.refs++
		v := math.Float64frombits(binary.BigEndian.Uint64(b))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("AMF0 nonfinite date")
		}
		f["Milliseconds"] = v
		f["Timezone"] = int16(binary.BigEndian.Uint16(b[8:]))
	case 12, 15:
		b, e = a.take(4)
		if e != nil {
			return nil, e
		}
		f["Value"], e = a.str(int(binary.BigEndian.Uint32(b)))
		if t == 15 {
			a.refs++
		}
	case 16:
		a.refs++
		f["Class"], e = a.short()
		if e == nil {
			f["Properties"], e = a.object(depth)
		}
	default:
		return nil, fmt.Errorf("%w: AMF0 marker %d requires another selected codec", ErrObservedWireContext, t)
	}
	return f, e
}
func DecodeAMF0(w []byte, max int) ([]any, error) {
	a := amf0Decoder{w: w, max: max}
	var out []any
	for a.off < len(w) {
		v, e := a.value(0)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

func RTMPMessageName(t byte) string {
	if s := map[byte]string{1: "Set Chunk Size", 2: "Abort", 3: "Acknowledgement", 4: "User Control", 5: "Window Acknowledgement Size", 6: "Set Peer Bandwidth", 8: "Audio", 9: "Video", 15: "AMF3 Data", 16: "AMF3 Shared Object", 17: "AMF3 Command", 18: "AMF0 Data", 19: "AMF0 Shared Object", 20: "AMF0 Command", 22: "Aggregate"}[t]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown Message Type %d", t)
}

// DecodeRTMPMessage applies only to a complete reassembled payload; transport
// framing and peer negotiation are deliberately supplied by the caller.
func DecodeRTMPMessage(w []byte, t byte, max int) (map[string]any, error) {
	f := map[string]any{"Packet Name": RTMPMessageName(t), "Message Type": t, "Message Length": len(w)}
	bad := func() (map[string]any, error) {
		return f, fmt.Errorf("RTMP %s malformed complete payload", RTMPMessageName(t))
	}
	switch t {
	case 1, 2, 3, 5:
		if len(w) != 4 {
			return bad()
		}
		v := binary.BigEndian.Uint32(w)
		key := map[byte]string{1: "Chunk Size", 2: "Abort CSID", 3: "Sequence", 5: "Acknowledgement Window"}[t]
		f[key] = v
		if t == 1 && (v == 0 || v > 0x7fffffff) || t == 2 && (v < 2 || v > 65599) {
			return bad()
		}
	case 6:
		if len(w) != 5 || w[4] > 2 {
			return bad()
		}
		f["Bandwidth"] = binary.BigEndian.Uint32(w)
		f["Limit Type"] = w[4]
	case 4:
		if len(w) < 2 {
			return bad()
		}
		ev := binary.BigEndian.Uint16(w)
		f["Event Type"] = ev
		n := map[uint16]int{0: 6, 1: 6, 2: 6, 3: 10, 4: 6, 6: 6, 7: 6}[ev]
		if n == 0 {
			f["Event Data"] = append([]byte(nil), w[2:]...)
			break
		}
		if len(w) != n {
			return bad()
		}
		f["Event Value"] = binary.BigEndian.Uint32(w[2:])
		if n == 10 {
			f["Buffer Length"] = binary.BigEndian.Uint32(w[6:])
		}
	case 17, 18, 20, 15:
		data := w
		if t == 17 || t == 15 {
			if len(data) == 0 || data[0] != 0 {
				return f, fmt.Errorf("%w: AMF3 message has no observed AMF0 wrapper", ErrObservedWireContext)
			}
			data = data[1:]
			f["AMF0 Wrapper"] = true
		}
		values, e := DecodeAMF0(data, max)
		if e != nil {
			return f, e
		}
		if len(values) == 0 {
			return bad()
		}
		f["AMF0 Values"] = values
		if t == 17 || t == 20 {
			if len(values) < 3 {
				return bad()
			}
			cmd := values[0].(map[string]any)
			txn := values[1].(map[string]any)
			if cmd["Type"] != byte(2) || txn["Type"] != byte(0) {
				return bad()
			}
			f["Command"] = cmd["Value"]
			if v, ok := txn["Value"].(float64); ok {
				f["Transaction Number"] = v
			}
		}
	case 8:
		if len(w) < 1 {
			return bad()
		}
		f["Sound Format"] = w[0] >> 4
		f["Sound Rate"] = w[0] >> 2 & 3
		f["Sound Size"] = w[0] >> 1 & 1
		f["Stereo"] = w[0]&1 == 1
		if w[0]>>4 == 10 {
			if len(w) < 2 || w[1] > 1 {
				return bad()
			}
			f["AAC Packet Type"] = w[1]
		}
	case 9:
		if len(w) < 1 {
			return bad()
		}
		f["Frame Type"] = w[0] >> 4
		f["Codec ID"] = w[0] & 15
		if w[0]&15 == 7 {
			if len(w) < 5 || w[1] > 2 {
				return bad()
			}
			f["AVC Packet Type"] = w[1]
			v := int32(rtmp24(w[2:]))
			if v&0x800000 != 0 {
				v |= ^int32(0xffffff)
			}
			f["Composition Time"] = v
		}
	case 22:
		var tags []any
		for off := 0; off < len(w); {
			if len(tags) >= max {
				return f, ErrObservedWireBudget
			}
			if len(w)-off < 15 {
				return bad()
			}
			n := int(rtmp24(w[off+1:]))
			if n > len(w)-off-15 || binary.BigEndian.Uint32(w[off+11+n:]) != uint32(n+11) {
				return bad()
			}
			if w[off] != 8 && w[off] != 9 && w[off] != 18 {
				return bad()
			}
			v, e := DecodeRTMPMessage(w[off+11:off+11+n], w[off], max-len(tags))
			if e != nil {
				return f, e
			}
			v["Timestamp"] = rtmp24(w[off+4:]) | uint32(w[off+7])<<24
			v["Stream ID"] = rtmp24(w[off+8:])
			tags = append(tags, v)
			off += 15 + n
		}
		f["Tags"] = tags
	case 16, 19:
		f["Shared Object Payload"] = append([]byte(nil), w...)
		f["Payload Interpretation"] = "shared object event codec not selected"
	default:
		f["Payload Interpretation"] = "unselected message type; bytes retained"
	}
	return f, nil
}

func DecodeRTMPInitialChunk(w []byte, max int) (map[string]any, error) {
	h, e := DecodeRTMPChunkHeader(w, RTMPChunkContext{}, 128, max)
	if e != nil {
		return nil, e
	}
	if h.WireLength == 0 || h.WireLength != len(w) {
		return nil, fmt.Errorf("RTMP initial chunk requires its exact complete wire boundary")
	}
	return map[string]any{"Packet Name": "Chunk", "Format": h.Format, "Chunk Stream ID": h.CSID, "Timestamp": h.Context.Timestamp, "Message Length": h.Context.Length, "Message Type": h.Context.Type, "Message Stream ID": h.Context.StreamID, "Payload Length": h.PayloadLength}, nil
}
