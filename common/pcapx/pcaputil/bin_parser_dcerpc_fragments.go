package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
)

const (
	dcerpcFragmentCharge      int64 = 128
	dcerpcFragmentPieceCharge int64 = 24
)

type dcerpcFragmentPiece struct {
	id         uint64
	start, end uint64
}

// The common header is followed by a request/response header in EVERY RPC
// fragment. Alloc Hint is advisory; it never controls allocation/completion.
func dcerpcMessageHeader(ptype, flags byte, body []byte) (int, error) {
	need := 8
	switch ptype {
	case 0:
		if flags&dcerpcObject != 0 {
			need += 16
		}
	case 2:
	case 3:
		need = 16
	default:
		return 0, protocolError(ErrUnsupportedFeature, "DCE/RPC fragmented PType %d is outside the call-message profile", ptype)
	}
	if len(body) < need {
		return 0, protocolError(ErrMalformedMessage, "DCE/RPC fragment lacks its complete per-PDU call header")
	}
	return need, nil
}

func dcerpcFragmentFailure(info map[string]any, err error) ([]byte, byte, bool, error) {
	// Only semantic failures at an already framed RPC boundary are isolated.
	// Memory failures remain fatal; this never skips a TCP byte hole.
	var typed *ProtocolError
	if errors.Is(err, errBinContext) || errors.As(err, &typed) && typed.Kind != ErrResourceExceeded {
		info["Error Scope"], info["Context Level"] = "message", "partial"
	}
	return nil, 0, false, err
}

func (d *binDCERPC) consumeMessageFragment(key dcerpcFragmentKey, ptype, flags byte, body []byte, info map[string]any, maxElements int, pduID uint64) ([]byte, byte, bool, error) {
	header, err := dcerpcMessageHeader(ptype, flags, body)
	if err != nil {
		d.discardFragment(key)
		return dcerpcFragmentFailure(info, err)
	}
	info["Fragment"] = true
	info["Fragment Stub Bytes"] = len(body) - header
	info["Context ID"] = binary.LittleEndian.Uint16(body[4:6])
	if ptype == 0 {
		if err := d.parseRequest(body, flags, info); err != nil {
			return dcerpcFragmentFailure(info, err)
		}
	} else if ptype == 3 {
		info["Status"] = binary.LittleEndian.Uint32(body[8:12])
	}
	piece := dcerpcFragmentPiece{id: pduID, start: uint64(16 + header), end: uint64(16 + len(body))}
	if flags&dcerpcFirst != 0 {
		if d.stateElements()+2 > sessionCollectionLimit(maxElements) || len(body)+16 > d.messageLimit() {
			return dcerpcFragmentFailure(info, protocolError(ErrResourceExceeded, "DCE/RPC first-fragment state exceeds budget"))
		}
		charge := dcerpcFragmentCharge + int64(len(body)) + dcerpcFragmentPieceCharge
		if err := d.reserveMemory(d.retainedBytes() + charge); err != nil {
			return dcerpcFragmentFailure(info, err)
		}
		if d.frags == nil {
			d.frags = make(map[dcerpcFragmentKey]dcerpcFragment)
		}
		ownedBody := make([]byte, len(body))
		copy(ownedBody, body)
		d.frags[key] = dcerpcFragment{ptype: ptype, firstFlags: flags, body: ownedBody, header: header, count: 1, pieces: []dcerpcFragmentPiece{piece}}
		info["Message ID"], info["Fragment Index"], info["Outstanding"] = pduID, 1, true
		return nil, flags, false, nil
	}
	previous, exists := d.frags[key]
	if !exists {
		return dcerpcFragmentFailure(info, sessionContext("DCE/RPC continuation has no first fragment in this direction"))
	}
	invalid := ""
	switch {
	case previous.ptype != ptype:
		invalid = "PType changed within call"
	case !bytes.Equal(previous.body[4:6], body[4:6]):
		invalid = "presentation context changed within call"
	case ptype == 0 && !bytes.Equal(previous.body[6:8], body[6:8]):
		invalid = "operation changed within request"
	case ptype == 0 && flags&dcerpcObject != 0 && (previous.firstFlags&dcerpcObject == 0 || !bytes.Equal(previous.body[8:24], body[8:24])):
		invalid = "object identity changed within request"
	case ptype == 3 && !bytes.Equal(previous.body[8:12], body[8:12]):
		invalid = "status changed within fault"
	}
	if invalid != "" {
		d.discardFragment(key)
		_ = d.reserveMemory(d.retainedBytes())
		return dcerpcFragmentFailure(info, protocolError(ErrDesynchronized, "DCE/RPC fragment %s", invalid))
	}
	need := len(previous.body) + len(body) - header
	if need+16 > d.messageLimit() || d.stateElements()+1 > sessionCollectionLimit(maxElements) {
		d.discardFragment(key)
		return dcerpcFragmentFailure(info, protocolError(ErrResourceExceeded, "DCE/RPC complete fragment message exceeds budget"))
	}
	bodyCapacity := max(cap(previous.body), need)
	pieceCapacity := len(previous.pieces) + 1
	extra := int64(bodyCapacity-cap(previous.body)) + int64(pieceCapacity-cap(previous.pieces))*dcerpcFragmentPieceCharge
	if err := d.reserveMemory(d.retainedBytes() + extra); err != nil {
		d.discardFragment(key)
		return dcerpcFragmentFailure(info, err)
	}
	assembled := make([]byte, need, bodyCapacity)
	copy(assembled, previous.body)
	copy(assembled[len(previous.body):], body[header:])
	pieces := make([]dcerpcFragmentPiece, pieceCapacity)
	copy(pieces, previous.pieces)
	pieces[len(previous.pieces)] = piece
	previous.body, previous.pieces, previous.count = assembled, pieces, previous.count+1
	info["Message ID"], info["Fragment Index"] = pieces[0].id, previous.count
	if flags&dcerpcLast == 0 {
		d.frags[key] = previous
		info["Outstanding"] = true
		return nil, previous.firstFlags, false, nil
	}
	stub := bytes.Clone(assembled[previous.header:])
	digest := sha256.Sum256(stub)
	parents := make([]uint64, 0, len(pieces))
	sources := make([]map[string]any, 0, len(pieces))
	for _, source := range pieces {
		if source.id != 0 {
			parents = append(parents, source.id)
		}
		sources = append(sources, map[string]any{"PDU ID": source.id, "Stub Range": []uint64{source.start, source.end}, "Stub Offset": source.start, "Stub Length": source.end - source.start})
	}
	info["Reassembled"] = true
	info["Reassembled Message"] = map[string]any{
		"Message ID": pieces[0].id, "Complete In PDU": pduID,
		"Header In PDU":  pieces[0].id,
		"Fragment Count": previous.count, "Stub Bytes": len(stub), "Stub": stub,
		"Stub Decoded": false, "Fragment Sources": sources,
		"Byte Source": map[string]any{"Kind": "reassembled", "SHA256": hex.EncodeToString(digest[:]), "ParentPDUs": parents, "Packet Projection": "not asserted; use the exact parent-PDU stub ranges"},
	}
	d.discardFragment(key)
	_ = d.reserveMemory(d.retainedBytes())
	return assembled, previous.firstFlags, true, nil
}
