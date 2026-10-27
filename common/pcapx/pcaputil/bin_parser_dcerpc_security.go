package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
)

func dcerpcHasSecurityTrailer(wire []byte) bool {
	return len(wire) >= 16 && (wire[10] != 0 || wire[11] != 0)
}

// Only the bounded outer envelope is interpreted. Neither a token nor
// plaintext-looking body bytes establish authentication or a presentation context.
func dcerpcSecurityEnvelope(wire []byte, info map[string]any) (map[string]any, error) {
	auth := int(binary.LittleEndian.Uint16(wire[10:12]))
	start := len(wire) - auth - 8
	if auth == 0 || start < 16 {
		return nil, protocolError(ErrMalformedMessage, "DCE/RPC authentication verifier length exceeds fragment payload")
	}
	trailer := wire[start : start+8]
	if trailer[0] == 0 || trailer[1] < 2 || trailer[1] > 6 {
		return nil, protocolError(ErrUnsupportedFeature, "DCE/RPC authentication type/level is outside the observed security-envelope profile")
	}
	padding := int(trailer[2])
	if padding > start-16 {
		return nil, protocolError(ErrMalformedMessage, "DCE/RPC authentication padding overlaps the common header")
	}
	info["RPC Vers"], info["RPC Vers Minor"] = wire[0], wire[1]
	info["Packed Drep"] = bytes.Clone(wire[4:8])
	info["Frag Length"], info["Auth Length"] = binary.LittleEndian.Uint16(wire[8:10]), uint16(auth)
	info["Profile"], info["Context Level"] = "dcerpc-security-envelope", "unverified-security-envelope"
	info["Authentication Verified"], info["Peer Identity Verified"], info["Stub Decoded"] = false, false, false
	// MS-RPCE alignment is relative to a PDU-specific body origin, not always
	// the common header. An opaque body provides no basis to certify it here.
	info["Trailer Alignment Validated"] = false
	info["Opaque PDU Body"] = bytes.Clone(wire[16 : start-padding])
	info["Authentication Padding"] = bytes.Clone(wire[start-padding : start])
	info["Security Trailer"] = map[string]any{"Auth Type": trailer[0], "Auth Level": trailer[1], "Auth Padding Length": trailer[2], "Auth Reserved": trailer[3], "Auth Context ID": binary.LittleEndian.Uint32(trailer[4:8])}
	info["Authentication Token"] = bytes.Clone(wire[start+8:])
	info["Wire Field Ranges"] = map[string]any{
		"Opaque PDU Body":        []uint64{16, uint64(start - padding)},
		"Authentication Padding": []uint64{uint64(start - padding), uint64(start)},
		"Security Trailer":       []uint64{uint64(start), uint64(start + 8)},
		"Authentication Token":   []uint64{uint64(start + 8), uint64(len(wire))},
	}
	return info, nil
}

var dcerpcBindFeaturePrefix = []byte{0x2c, 0x1c, 0xb7, 0x6c, 0x12, 0x98, 0x40, 0x45}

func dcerpcBindFeatureProposal(p dcerpcProposal) bool {
	return len(p.transferSyntax) == 16 && bytes.Equal(p.transferSyntax[:8], dcerpcBindFeaturePrefix) && p.transferVersion == 1
}

func (d *binDCERPC) duplicateCall(call uint32, dir int) error {
	if d.pending[call] == "Bind" || d.pending[call] == "AlterContext" || !d.peerMessages[1-dir] {
		return sessionContext("DCE/RPC call id reuse has no observed peer completion in this capture domain")
	}
	if d.pendingDir[call] != dir {
		return protocolError(ErrUnsupportedFeature, "DCE/RPC opposite-direction outstanding call id namespace is unsupported")
	}
	return protocolError(ErrDesynchronized, "DCE/RPC duplicate outstanding call id %d", call)
}

func dcerpcCallError(info map[string]any, err error) (map[string]any, error) {
	if errors.Is(err, errBinContext) {
		// The complete framed request is available. Keep the actual pending
		// owner and let a later captured completion restore usable context.
		info["Error Scope"] = "message"
		info["Context Missing"] = err.Error()
		return info, err
	}
	return nil, err
}
