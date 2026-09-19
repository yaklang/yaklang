package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strings"
)

// This is the Chromium 57 Q035 wire profile, not IETF QUIC or TLS.
// quic_constants.h fixes this implementation's packet ceiling at 1452 bytes.
const gquic35MaxBytes = 1452

type gquic35Field struct {
	Name, Type string
	Endian     string // empty inherits Q035 little-endian; NONC time is big-endian
	Start, End int    // byte offsets in the unchanged packet
	Children   []gquic35Field
	Info       map[string]any
}

func gquic35Leaf(name, typ string, start, end int) gquic35Field {
	return gquic35Field{Name: name, Type: typ, Start: start, End: end}
}

// Q035 NullDecrypter compares low 96 bits of FNV-1a-128(header || plaintext).
// Q037's perspective suffix MUST NOT be included. No keys or authentication
// are involved; matching this checksum is not peer identity verification.
func gquic35NullHash(header, body []byte) [12]byte {
	hi, lo := uint64(7809847782465536322), uint64(7113472399480571277)
	for _, part := range [][]byte{header, body} {
		for _, b := range part {
			lo ^= uint64(b)
			carry, next := bits.Mul64(lo, 315)
			hi = hi*315 + lo*16777216 + carry
			lo = next
		}
	}
	var result [12]byte
	binary.LittleEndian.PutUint64(result[:8], lo)
	binary.LittleEndian.PutUint32(result[8:], uint32(hi))
	return result
}

func gquic35Uint(wire []byte) uint64 {
	var v uint64
	for i, b := range wire {
		v |= uint64(b) << (8 * i)
	}
	return v
}

func decodeGQUIC35(wire []byte, fromServer, clientHello bool) (fields []gquic35Field, info map[string]any, err error) {
	defer func() {
		if err != nil {
			fields, info = nil, nil
		}
	}()
	fail := func(reason string) error { return fmt.Errorf("gquic35: %s", reason) }
	if len(wire) < 3 || len(wire) > gquic35MaxBytes {
		return nil, nil, fail("packet boundary must be 3..1452 bytes")
	}
	if fromServer && clientHello {
		return nil, nil, fail("client hello requires client-to-server direction")
	}
	flags := wire[0]
	if flags&0x80 != 0 {
		return nil, nil, fail("unsupported extended public flags")
	}
	if flags&0x02 != 0 {
		return nil, nil, fail("unsupported public reset packet")
	}
	if flags&0x40 != 0 {
		return nil, nil, fail("unsupported multipath packet")
	}
	if fromServer && flags&1 != 0 {
		return nil, nil, fail("unsupported server version negotiation packet")
	}
	p := 1
	header := gquic35Field{Name: "Public Header", Start: 0, Children: []gquic35Field{gquic35Leaf("Public Flags", "uint8", 0, 1)}}
	take := func(name, typ string, size int) error {
		if size > len(wire)-p {
			return fail("truncated " + name)
		}
		header.Children = append(header.Children, gquic35Leaf(name, typ, p, p+size))
		p += size
		return nil
	}
	if flags&8 != 0 {
		if err = take("Connection ID", "uint64", 8); err != nil {
			return nil, nil, err
		}
	}
	if flags&1 != 0 {
		if err = take("Version Tag", "string", 4); err != nil {
			return nil, nil, err
		}
		if string(wire[p-4:p]) != "Q035" {
			return nil, nil, fail("unsupported wire version; explicit Q035 profile")
		}
	}
	// Chromium ProcessPublicHeader: bit 2 on a CLIENT packet is the ignored
	// old eight-byte-CID compatibility bit, not a diversification nonce.
	nonce := fromServer && flags&4 != 0
	if nonce {
		if err = take("Diversification Nonce", "raw", 32); err != nil {
			return nil, nil, err
		}
	}
	pnBytes := []int{1, 2, 4, 6}[(flags>>4)&3]
	if err = take("Wire Packet Number", "uint64", pnBytes); err != nil {
		return nil, nil, err
	}
	if p == len(wire) {
		return nil, nil, fail("missing packet body")
	}
	header.End = p
	fields = append(fields, header)
	direction := "client-to-server"
	if fromServer {
		direction = "server-to-client"
	}
	info = map[string]any{
		"Profile": "Chromium 57 Q035 data packet public header", "Caller Supplied Version": "Q035",
		"Caller Supplied Direction": direction, "Wire Version Present": flags&1 != 0,
		"Connection ID Present": flags&8 != 0, "Diversification Nonce Present": nonce,
		"Client Legacy CID Bit": !fromServer && flags&4 != 0, "Wire Packet Number Bytes": pnBytes,
		"Packet Number Reconstructed": false, "Connection State Inferred": false,
		"Public Header Bytes": p, "Body Format Decoded": false, "Null Checksum Verified": false,
		"Payload Decrypted": false, "Peer Authenticated": false, "Handshake Parameters Complete": false,
		"Handshake Outcome Inferred": false, "Maximum Packet Bytes": gquic35MaxBytes,
	}
	if !clientHello {
		fields = append(fields, gquic35Leaf("Protected or Uninterpreted Body", "raw", p, len(wire)))
		return fields, info, nil
	}
	if len(wire)-p < 13 {
		return nil, nil, fail("truncated null checksum or frame body")
	}
	hash := gquic35NullHash(wire[:p], wire[p+12:])
	if !bytes.Equal(hash[:], wire[p:p+12]) {
		return nil, nil, fail("null-profile checksum mismatch")
	}
	fields = append(fields, gquic35Leaf("Null Checksum", "raw", p, p+12))
	p += 12
	streamStart := p
	ft := wire[p]
	p++
	if ft&0x80 == 0 || ft&0x40 != 0 {
		return nil, nil, fail("profile requires a single non-FIN crypto STREAM frame")
	}
	sidSize, offSize := int(ft&3)+1, int((ft>>2)&7)
	if offSize > 0 {
		offSize++
	}
	stream := gquic35Field{Name: "Crypto STREAM Frame", Start: streamStart, Children: []gquic35Field{gquic35Leaf("STREAM Type", "uint8", streamStart, p)}}
	if sidSize+offSize > len(wire)-p {
		return nil, nil, fail("truncated STREAM ID or offset")
	}
	stream.Children = append(stream.Children, gquic35Leaf("Stream ID", "uint32", p, p+sidSize))
	if gquic35Uint(wire[p:p+sidSize]) != 1 {
		return nil, nil, fail("profile requires crypto stream ID 1")
	}
	p += sidSize
	if offSize > 0 {
		stream.Children = append(stream.Children, gquic35Leaf("Stream Offset", "uint64", p, p+offSize))
		if gquic35Uint(wire[p:p+offSize]) != 0 {
			return nil, nil, fail("fragmented CHLO requires unavailable stream reassembly")
		}
		p += offSize
	}
	lengthPresent := ft&0x20 != 0
	dataLength := len(wire) - p
	if lengthPresent {
		if len(wire)-p < 2 {
			return nil, nil, fail("truncated STREAM data length")
		}
		dataLength = int(binary.LittleEndian.Uint16(wire[p:]))
		stream.Children = append(stream.Children, gquic35Leaf("Stream Data Length", "uint16", p, p+2))
		p += 2
	}
	if dataLength > len(wire)-p {
		return nil, nil, fail("STREAM data exceeds packet boundary")
	}
	if dataLength < 1024 {
		return nil, nil, fail("CHLO is below Chromium 1024-byte minimum")
	}
	chlo, err := gquic35CHLO(wire, p, p+dataLength)
	if err != nil {
		return nil, nil, err
	}
	stream.Children = append(stream.Children, chlo)
	p += dataLength
	stream.End = p
	stream.Info = map[string]any{"FIN": false, "Offset Present": offSize != 0, "Stream Offset": uint64(0), "Data Length Present": lengthPresent, "Reassembled": false}
	fields = append(fields, stream)
	if p < len(wire) {
		if wire[p] != 0 {
			return nil, nil, fail("unsupported additional frame after complete CHLO")
		}
		// Q035 ProcessFrameData returns at the first PADDING type and does not
		// interpret or constrain its remaining bytes. Preserve all of them.
		padding := gquic35Field{Name: "Padding Frame", Start: p, End: len(wire), Children: []gquic35Field{gquic35Leaf("Padding Type", "uint8", p, p+1)}}
		if p+1 < len(wire) {
			padding.Children = append(padding.Children, gquic35Leaf("Padding Remainder", "raw", p+1, len(wire)))
		}
		fields = append(fields, padding)
	}
	info["Profile"] = "Chromium 57 Q035 null-protected complete client CHLO field profile"
	info["Null Checksum Verified"], info["Body Format Decoded"] = true, true
	info["CHLO Entry Count"] = len(chlo.Children[3].Children)
	return fields, info, nil
}

func gquic35CHLO(wire []byte, start, end int) (gquic35Field, error) {
	fail := func(reason string) (gquic35Field, error) { return gquic35Field{}, fmt.Errorf("gquic35: %s", reason) }
	if end-start < 8 || string(wire[start:start+4]) != "CHLO" {
		return fail("expected complete CHLO message tag")
	}
	count := int(binary.LittleEndian.Uint16(wire[start+4:]))
	if count > 128 {
		return fail("CHLO exceeds 128-entry limit")
	}
	valueStart := start + 8 + count*8
	if valueStart > end {
		return fail("truncated CHLO tag table")
	}
	chlo := gquic35Field{Name: "Client Hello", Start: start, End: end, Children: []gquic35Field{
		gquic35Leaf("Handshake Tag", "string", start, start+4), gquic35Leaf("Tag Count", "uint16", start+4, start+6),
		gquic35Leaf("Handshake Padding", "raw", start+6, start+8),
	}}
	table := gquic35Field{Name: "Tag Table", Start: start + 8, End: valueStart}
	values := gquic35Field{Name: "Tag Values", Start: valueStart, End: end}
	var previousTag, previousEnd uint32
	for i := 0; i < count; i++ {
		p := start + 8 + i*8
		tag, offset := binary.LittleEndian.Uint32(wire[p:]), binary.LittleEndian.Uint32(wire[p+4:])
		if i > 0 && tag <= previousTag {
			return fail("CHLO tags must be strictly increasing little-endian integers")
		}
		if offset < previousEnd || uint64(offset) > uint64(end-valueStart) {
			return fail("CHLO cumulative end offset is descending or out of bounds")
		}
		table.Children = append(table.Children, gquic35Field{Name: fmt.Sprintf("Tag Entry %d", i), Start: p, End: p + 8, Children: []gquic35Field{
			gquic35Leaf("Tag", "string", p, p+4), gquic35Leaf("Cumulative End Offset", "uint32", p+4, p+8),
		}})
		f, err := gquic35TagValue(wire, string(wire[p:p+4]), valueStart+int(previousEnd), valueStart+int(offset), i)
		if err != nil {
			return fail(err.Error())
		}
		values.Children = append(values.Children, f)
		previousTag, previousEnd = tag, offset
	}
	if uint64(previousEnd) != uint64(end-valueStart) {
		return fail("CHLO has unconsumed bytes after final tag value")
	}
	chlo.Children = append(chlo.Children, table, values)
	return chlo, nil
}

func gquic35TagValue(wire []byte, tag string, start, end, index int) (gquic35Field, error) {
	name := strings.TrimRight(tag, "\x00")
	for _, b := range []byte(name) {
		if b < 0x20 || b > 0x7e {
			name = fmt.Sprintf("%08x", binary.LittleEndian.Uint32([]byte(tag)))
			break
		}
	}
	f := gquic35Leaf("CHLO "+name, "raw", start, end)
	f.Info = map[string]any{"Table Index": index, "Tag": tag, "Value Layout Decoded": false, "Value Semantics Validated": false}
	width := 0
	switch tag {
	case "ICSL", "CFCW", "SFCW", "IRTT", "MSPC", "SRBF", "SWND", "MIDS", "SCLS", "TCID", "SMHL":
		f.Type, width = "uint32", 4
	case "CTIM", "XLCT":
		f.Type, width = "uint64", 8
		if tag == "CTIM" {
			f.Info["Unit"] = "seconds since Unix epoch"
			f.Info["Clock Validated"] = false
		} else {
			f.Info["Certificate Hash Matched"] = false
		}
	case "CCS\x00", "CCRT":
		if (end-start)%8 != 0 {
			return gquic35Field{}, fmt.Errorf("CHLO %s hash list has partial uint64", name)
		}
		f.Info["Hash Count"] = (end - start) / 8
		f.Info["Certificate Hash Matched"] = false
		if end > start {
			f.Type = ""
		}
		for p := start; p < end; p += 8 {
			f.Children = append(f.Children, gquic35Leaf(fmt.Sprintf("%s Hash %d", name, (p-start)/8), "uint64", p, p+8))
		}
	case "NONC":
		if end-start != 32 {
			return gquic35Field{}, fmt.Errorf("CHLO NONC requires 32-byte value")
		}
		// Chromium 57 FillClientHello supplies an eight-byte SCFG orbit to
		// GenerateNonce. Its timestamp is big-endian, unlike CHLO scalars.
		f.Type = ""
		timestamp := gquic35Leaf("Nonce Timestamp Seconds", "uint32", start, start+4)
		timestamp.Endian = "big"
		f.Children = []gquic35Field{timestamp, gquic35Leaf("Nonce Orbit", "raw", start+4, start+12), gquic35Leaf("Nonce Random Bytes", "raw", start+12, end)}
		f.Info["Orbit Correlated"], f.Info["Clock Validated"], f.Info["Randomness Validated"] = false, false, false
	case "NONP":
		width = 32 // opaque random bytes, not GenerateNonce's timestamp layout
		f.Info["Randomness Validated"] = false
	case "CSCT":
		// FillInchoateClientHello emits the empty request marker, not an SCT
		// list. Nonempty unknown values remain completely uninterpreted.
		if end != start {
			return f, nil
		}
		f.Info["Empty SCT Request Marker"] = true
	case "SNI\x00", "UAID":
		f.Type = "string" // factual bytes, no hostname or encoding validation
	case "VER\x00", "AEAD", "KEXS", "COPT", "PDMD", "TBKP":
		if (end-start)%4 != 0 {
			return gquic35Field{}, fmt.Errorf("CHLO %s tag list has partial tag", name)
		}
		f.Info["Tag Count"] = (end - start) / 4
		// An empty tag vector still needs a physical zero-length leaf: an
		// empty structural node is omitted by the library's Result walker.
		if end > start {
			f.Type = ""
		}
		for p := start; p < end; p += 4 {
			f.Children = append(f.Children, gquic35Leaf(fmt.Sprintf("%s Tag %d", name, (p-start)/4), "string", p, p+4))
		}
	default:
		// PUBS is a single client value, NOT SCFG's u24-length vector.
		// PUBS, CETV, tokens, certificate-related bytes, PAD, extensions and
		// unimplemented option values remain opaque; no cryptographic claim.
		return f, nil
	}
	if width != 0 && end-start != width {
		return gquic35Field{}, fmt.Errorf("CHLO %s requires %d-byte value", name, width)
	}
	f.Info["Value Layout Decoded"] = true
	return f, nil
}
